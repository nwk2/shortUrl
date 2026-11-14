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

// Hash lengths: start at 7 and grow to 9 if a length is exhausted.
// 7 base58 chars is ~2.2 trillion codes, so growth is insurance for a
// table dense enough that 5 straight collisions happen, not a routine
// path.
const (
	minHashLength = 7
	maxHashLength = 9
	hashAttempts  = 5
)

// Service is the domain layer: validating input, minting hashes, applying
// expiry rules. It knows nothing about HTTP; the handler translates its
// errors to statuses.
type Service struct {
	store    *store.Store
	hostname string
	// mint generates candidate hashes. It is a field (defaulting to
	// generateHash) so tests can inject a deterministic generator and
	// force collisions, retries and length growth.
	mint func(length int) (string, error)
}

func New(s *store.Store, hostname string) *Service {
	return &Service{store: s, hostname: hostname, mint: generateHash}
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

	// The insert is the uniqueness check: mint a candidate and try to
	// store it. On a hash collision the store reports ErrHashConflict
	// and we retry with a fresh candidate — no pre-check read, and no
	// race between two concurrent creates for the same candidate. The
	// unique index is the arbiter, which is the only thing that is
	// actually correct under concurrency. Length grows if a length is
	// exhausted, so a dense table degrades gracefully instead of failing.
	for length := minHashLength; length <= maxHashLength; length++ {
		for attempt := 0; attempt < hashAttempts; attempt++ {
			candidate, err := s.mint(length)
			if err != nil {
				return model.ShortUrl{}, err
			}
			link := model.ShortUrl{
				ShortUrl:    s.hostname + candidate,
				OriginalUrl: parsedURL.String(),
				HashKey:     candidate,
				CreatedDate: createdTS,
				ExpiryDate:  expiryTS,
			}
			if err := s.store.Create(ctx, &link); err == nil {
				return link, nil
			} else if !errors.Is(err, store.ErrHashConflict) {
				return model.ShortUrl{}, err
			}
		}
	}
	return model.ShortUrl{}, ErrHashExhausted
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

// generateHash mints a length-character base58 token from crypto/rand.
// Base58 is the Bitcoin alphabet: no 0, O, I or l, so codes survive
// being read aloud or retyped. Sampling is rejection-based, not
// byte%58: 58 doesn't divide 256, so a plain modulo would make some
// characters likelier than others.
func generateHash(length int) (string, error) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	// Largest multiple of 58 below 256; bytes at or above it are
	// discarded so every character is equally likely.
	const maxByte = 58 * 4 // 232
	out := make([]byte, 0, length)
	buf := make([]byte, length)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b < maxByte {
				out = append(out, alphabet[int(b)%len(alphabet)])
				if len(out) == length {
					break
				}
			}
		}
	}
	return string(out), nil
}
