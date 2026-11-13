package service

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"time"

	"shortUrl/internal/model"
	"shortUrl/internal/store"
)

// Sentinel errors the handler maps to HTTP statuses. Messages stay the
// same as the pre-refactor handlers returned, so clients see no change.
var (
	ErrNotFound      = store.ErrNotFound
	ErrExpired       = errors.New("short url expired")
	ErrInvalidURL    = errors.New("invalid url provided")
	ErrInvalidExpiry = errors.New("invalid expiry date")
	ErrExpiryPast    = errors.New("expiry date must be in the future")
	ErrHashExhausted = errors.New("failed to generate a unique short url")
)

// Service is the domain layer: validating input, minting hashes, applying
// expiry rules. It knows nothing about HTTP; the handler translates its
// errors to statuses.
type Service struct {
	store    *store.Store
	hostname string
}

func New(s *store.Store, hostname string) *Service {
	return &Service{store: s, hostname: hostname}
}

// Create validates the URL and expiry, mints a unique hash and stores the link.
func (s *Service) Create(ctx context.Context, originalURL, expiry string) (model.ShortUrl, error) {
	parsedURL, err := url.Parse(originalURL)
	if err != nil || parsedURL.String() == "" {
		return model.ShortUrl{}, ErrInvalidURL
	}
	// Only http(s) links can be shortened. Anything else (javascript:,
	// data:, ftp:, ...) would turn the short domain into a redirector
	// to an arbitrary scheme. A missing scheme is fine — the redirect
	// defaults it to http, as before.
	if !allowedScheme(parsedURL.Scheme) {
		return model.ShortUrl{}, ErrInvalidURL
	}

	expiryTS, createdTS, err := parseTimestamps(expiry)
	if err != nil {
		return model.ShortUrl{}, ErrInvalidExpiry
	}
	if expiryTS.Before(time.Now()) {
		return model.ShortUrl{}, ErrExpiryPast
	}

	// Hashes are random, so a collision is near-impossible; the unique
	// index is the backstop and the loop below is the safety net, kept
	// bounded so a pathological run of collisions fails fast.
	var hash string
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := generateHash()
		if err != nil {
			return model.ShortUrl{}, err
		}
		if _, err := s.store.GetByHash(ctx, candidate); errors.Is(err, store.ErrNotFound) {
			hash = candidate
			break
		}
		// Any other outcome (collision, or a lookup error) means this
		// candidate is unusable; try another.
	}
	if hash == "" {
		return model.ShortUrl{}, ErrHashExhausted
	}

	link := model.ShortUrl{
		ShortUrl:    s.hostname + hash,
		OriginalUrl: parsedURL.String(),
		HashKey:     hash,
		CreatedDate: createdTS,
		ExpiryDate:  expiryTS,
	}
	if err := s.store.Create(ctx, &link); err != nil {
		return model.ShortUrl{}, err
	}
	return link, nil
}

// GetByHash returns the link for a hash, or ErrNotFound / ErrExpired.
func (s *Service) GetByHash(ctx context.Context, hashKey string) (model.ShortUrl, error) {
	link, err := s.store.GetByHash(ctx, hashKey)
	if err != nil {
		return model.ShortUrl{}, err
	}
	if isExpired(link) {
		return model.ShortUrl{}, ErrExpired
	}
	return link, nil
}

// List returns live (unexpired) links, pageSize at a time, in creation
// order. Expired rows are excluded here; they are removed by the hourly
// sweep, and GetByHash still reports 410 for them until then.
func (s *Service) List(ctx context.Context, limit, offset int) ([]model.ShortUrl, error) {
	return s.store.List(ctx, limit, offset)
}

// Resolve returns the original URL a hash redirects to, defaulting a
// missing scheme to http (as the old redirect handler did). A stored URL
// with any other scheme is refused, so a row written before the scheme
// allowlist can't be used as a redirector either.
func (s *Service) Resolve(ctx context.Context, hashKey string) (string, error) {
	link, err := s.GetByHash(ctx, hashKey)
	if err != nil {
		return "", err
	}
	redirectURL, err := url.Parse(link.OriginalUrl)
	if err != nil {
		return "", err
	}
	if !allowedScheme(redirectURL.Scheme) {
		return "", ErrInvalidURL
	}
	if redirectURL.Scheme == "" {
		redirectURL.Scheme = "http"
	}
	return redirectURL.String(), nil
}

// allowedScheme is the redirect allowlist: http, https, or empty
// (normalised to http at redirect time). Everything else is rejected.
func allowedScheme(scheme string) bool {
	switch scheme {
	case "", "http", "https":
		return true
	default:
		return false
	}
}

func isExpired(link model.ShortUrl) bool {
	return time.Now().After(link.ExpiryDate)
}

// parseTimestamps parses an optional "YYYY-MM-DD HH:MM" expiry; empty
// means 30 days after creation. Creation is truncated to the minute so
// created and expiry stay on the same grid. Times are interpreted in the
// server's local timezone, so "10:30" means 10:30 where the server runs
// — parsing in UTC instead silently moved every expiry by the local
// offset.
func parseTimestamps(expiry string) (expiryTS, createdTS time.Time, err error) {
	const layout = "2006-01-02 15:04"
	now := time.Now().Format(layout)

	if expiry != "" {
		expiryTS, err = time.ParseInLocation(layout, expiry, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		expiryTS, err = time.ParseInLocation(layout, now, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		expiryTS = expiryTS.AddDate(0, 0, 30)
	}

	createdTS, err = time.ParseInLocation(layout, now, time.Local)
	return expiryTS, createdTS, err
}

// generateHash mints an 11-character base62 token from crypto/rand:
// ~65 bits of entropy, the same order as the old 64-bit FNV hash.
func generateHash() (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var buf [11]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}
