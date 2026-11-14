package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"shorturl/internal/model"
	"shorturl/internal/store"
)

func setupTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.ShortURL{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return New(store.New(db), "http://localhost:8080/redirect/")
}

func TestGenerateHash(t *testing.T) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		h, err := generateHash(7)
		if err != nil {
			t.Fatalf("generateHash returned error: %v", err)
		}
		if len(h) != 7 {
			t.Fatalf("hash %q has length %d, want 7", h, len(h))
		}
		for _, c := range h {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("hash %q contains non-base58 rune %q", h, c)
			}
		}
		if seen[h] {
			t.Fatalf("duplicate hash %q in 1000 draws", h)
		}
		seen[h] = true
	}

	// The other lengths mint exactly that many characters.
	for _, length := range []int{8, 9} {
		h, err := generateHash(length)
		if err != nil {
			t.Fatalf("generateHash(%d) returned error: %v", length, err)
		}
		if len(h) != length {
			t.Fatalf("generateHash(%d) = %q, length %d", length, h, len(h))
		}
	}

	// Every alphabet character shows up over enough draws. With ~140k
	// draws the chance of a given character never appearing is
	// e^-2400, so this is a sampler-coverage check, not a flaky
	// statistical test. It also proves none of the ambiguous 0/O/I/l
	// ever appears: they aren't in the alphabet at all.
	seenRunes := make(map[rune]bool, len(alphabet))
	for i := 0; i < 20000; i++ {
		h, err := generateHash(7)
		if err != nil {
			t.Fatalf("generateHash returned error: %v", err)
		}
		for _, c := range h {
			seenRunes[c] = true
		}
	}
	for _, c := range alphabet {
		if !seenRunes[c] {
			t.Errorf("alphabet rune %q never drawn in 20000 hashes", c)
		}
	}
}

func TestServiceCreateRetriesHashConflict(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	taken := model.ShortURL{
		ShortURL:    "http://localhost:8080/redirect/1111111",
		OriginalURL: "https://example.com/taken",
		HashKey:     "1111111",
		CreatedDate: time.Now(),
		ExpiryDate:  time.Now().Add(24 * time.Hour),
	}
	if err := svc.store.Create(ctx, &taken); err != nil {
		t.Fatalf("store.Create returned error: %v", err)
	}

	// First two candidates collide with the seeded row; the third is free.
	calls := 0
	svc.mint = func(length int) (string, error) {
		calls++
		if calls <= 2 {
			return "1111111", nil
		}
		return "2222222", nil
	}
	link, err := svc.Create(ctx, "https://example.com/retry", "")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if link.HashKey != "2222222" {
		t.Errorf("hash_key = %q, want the first non-colliding candidate", link.HashKey)
	}
	if calls != 3 {
		t.Errorf("mint called %d times, want 3", calls)
	}
}

func TestServiceCreateGrowsHashLength(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	taken := model.ShortURL{
		ShortURL:    "http://localhost:8080/redirect/1111111",
		OriginalURL: "https://example.com/taken",
		HashKey:     "1111111",
		CreatedDate: time.Now(),
		ExpiryDate:  time.Now().Add(24 * time.Hour),
	}
	if err := svc.store.Create(ctx, &taken); err != nil {
		t.Fatalf("store.Create returned error: %v", err)
	}

	// Length 7 is exhausted (every candidate collides); length 8 mints
	// a fresh hash on its first attempt.
	calls := 0
	svc.mint = func(length int) (string, error) {
		calls++
		if length == 7 {
			return "1111111", nil
		}
		return "22222222", nil
	}
	link, err := svc.Create(ctx, "https://example.com/grow", "")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(link.HashKey) != 8 || link.HashKey != "22222222" {
		t.Errorf("hash_key = %q, want the 8-character candidate", link.HashKey)
	}
	// 5 failed attempts at length 7, then 1 at length 8.
	if calls != 6 {
		t.Errorf("mint called %d times, want 6", calls)
	}
}

func TestServiceCreateHashExhausted(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	taken := model.ShortURL{
		ShortURL:    "http://localhost:8080/redirect/1111111",
		OriginalURL: "https://example.com/taken",
		HashKey:     "1111111",
		CreatedDate: time.Now(),
		ExpiryDate:  time.Now().Add(24 * time.Hour),
	}
	if err := svc.store.Create(ctx, &taken); err != nil {
		t.Fatalf("store.Create returned error: %v", err)
	}

	calls := 0
	svc.mint = func(length int) (string, error) {
		calls++
		return "1111111", nil
	}
	if _, err := svc.Create(ctx, "https://example.com/exhaust", ""); !errors.Is(err, ErrHashExhausted) {
		t.Errorf("Create error = %v, want ErrHashExhausted", err)
	}
	// 5 attempts at each of lengths 7, 8 and 9, then give up.
	if calls != 15 {
		t.Errorf("mint called %d times, want 15", calls)
	}
}

func TestServiceCreateConcurrent(t *testing.T) {
	// The pre-check design raced: two concurrent creates for the same
	// candidate could both pass the lookup, and one insert would fail
	// on the unique index with a 500. Insert-retry makes that
	// impossible: every create either lands a unique hash or retries.
	// A file-backed database in WAL mode with several connections
	// exercises it for real; an in-memory database serialises on a
	// single connection and would hide the race.
	dbPath := filepath.Join(t.TempDir(), "race.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(8)
	if err := db.AutoMigrate(&model.ShortURL{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	svc := New(store.New(db), "http://localhost:8080/redirect/")

	const n = 50
	hashes := make(chan string, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			link, err := svc.Create(context.Background(), fmt.Sprintf("https://example.com/%d", i), "")
			if err != nil {
				errs <- err
				return
			}
			hashes <- link.HashKey
		}(i)
	}
	wg.Wait()
	close(hashes)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Create returned error: %v", err)
	}
	seen := make(map[string]bool, n)
	for h := range hashes {
		if seen[h] {
			t.Errorf("duplicate hash %q across concurrent creates", h)
		}
		seen[h] = true
	}
	if len(seen) != n {
		t.Errorf("got %d unique hashes, want %d", len(seen), n)
	}
}

func TestServiceGetByHashOldLength(t *testing.T) {
	// An 11-character hash minted before the base58 change still
	// resolves: lookup is by hash value, not by length.
	svc := setupTestService(t)
	ctx := context.Background()

	old := model.ShortURL{
		ShortURL:    "http://localhost:8080/redirect/9I3BXIwDa44",
		OriginalURL: "https://example.com/old",
		HashKey:     "9I3BXIwDa44",
		CreatedDate: time.Now(),
		ExpiryDate:  time.Now().Add(24 * time.Hour),
	}
	if err := svc.store.Create(ctx, &old); err != nil {
		t.Fatalf("store.Create returned error: %v", err)
	}
	got, err := svc.GetByHash(ctx, "9I3BXIwDa44")
	if err != nil {
		t.Fatalf("GetByHash returned error: %v", err)
	}
	if got.OriginalURL != "https://example.com/old" {
		t.Errorf("original_url = %q", got.OriginalURL)
	}
}

func TestParseTimestampsUsesLocalTime(t *testing.T) {
	// time.Local is process-global; swap it for this test and restore it.
	orig := time.Local
	time.Local = time.FixedZone("SGT", 8*3600)
	defer func() { time.Local = orig }()

	expiry, _, err := parseTimestamps("2030-06-15 10:30")
	if err != nil {
		t.Fatalf("parseTimestamps returned error: %v", err)
	}
	// 10:30 in +08:00 is 02:30 UTC. Under the old UTC parsing it would
	// have been 10:30 UTC — eight hours later than the user meant.
	if got := expiry.Format("2006-01-02 15:04 -07:00"); got != "2030-06-15 10:30 +08:00" {
		t.Errorf("expiry = %q, want 10:30 in +08:00", got)
	}
	if got := expiry.UTC().Format("15:04"); got != "02:30" {
		t.Errorf("expiry in UTC = %q, want 02:30", got)
	}
}

func TestParseTimestampsDefault(t *testing.T) {
	expiry, created, err := parseTimestamps("")
	if err != nil {
		t.Fatalf("parseTimestamps(\"\") returned error: %v", err)
	}
	if got := expiry.Sub(created); got != 30*24*time.Hour {
		t.Errorf("expiry - created = %v, want 720h", got)
	}
}

func TestParseTimestampsCustom(t *testing.T) {
	expiry, _, err := parseTimestamps("2030-06-15 10:30")
	if err != nil {
		t.Fatalf("parseTimestamps returned error: %v", err)
	}
	// Parsed in local time, like the implementation does.
	want, _ := time.ParseInLocation("2006-01-02 15:04", "2030-06-15 10:30", time.Local)
	if !expiry.Equal(want) {
		t.Errorf("expiry = %v, want %v", expiry, want)
	}
}

func TestParseTimestampsBadFormat(t *testing.T) {
	if _, _, err := parseTimestamps("not-a-date"); err == nil {
		t.Error("parseTimestamps(\"not-a-date\") returned no error")
	}
}

func TestServiceCreateRejectsScheme(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for _, raw := range []string{
		"javascript:alert(1)",
		"data:text/html,<h1>hi</h1>",
		"ftp://example.com/file",
	} {
		if _, err := svc.Create(ctx, raw, ""); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Create(%q) error = %v, want ErrInvalidURL", raw, err)
		}
	}
}

func TestServiceResolveRejectsStoredScheme(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// A row written before the allowlist (or around it) must not be
	// usable as a redirector either.
	stored := model.ShortURL{
		ShortURL:    "http://localhost:8080/redirect/oldbad1",
		OriginalURL: "javascript:alert(1)",
		HashKey:     "oldbad1",
		CreatedDate: time.Now(),
		ExpiryDate:  time.Now().Add(24 * time.Hour),
	}
	if err := svc.store.Create(ctx, &stored); err != nil {
		t.Fatalf("store.Create returned error: %v", err)
	}
	if _, err := svc.Resolve(ctx, "oldbad1"); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("Resolve on a javascript: link error = %v, want ErrInvalidURL", err)
	}
}

func TestServiceCreate(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	link, err := svc.Create(ctx, "https://example.com/some/path", "")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if link.HashKey == "" {
		t.Error("hash_key is empty")
	}
	if !strings.HasSuffix(link.ShortURL, link.HashKey) {
		t.Errorf("short_url %q does not end with hash_key %q", link.ShortURL, link.HashKey)
	}
	if link.OriginalURL != "https://example.com/some/path" {
		t.Errorf("original_url = %q", link.OriginalURL)
	}

	got, err := svc.GetByHash(ctx, link.HashKey)
	if err != nil {
		t.Fatalf("GetByHash returned error: %v", err)
	}
	if got.ID != link.ID {
		t.Errorf("GetByHash id = %d, want %d", got.ID, link.ID)
	}
}

func TestServiceCreateInvalidURL(t *testing.T) {
	svc := setupTestService(t)

	if _, err := svc.Create(context.Background(), "%zz", ""); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("Create(\"%%zz\") error = %v, want ErrInvalidURL", err)
	}
}

func TestServiceCreatePastExpiry(t *testing.T) {
	svc := setupTestService(t)

	if _, err := svc.Create(context.Background(), "https://example.com", "2020-01-01 00:00"); !errors.Is(err, ErrExpiryPast) {
		t.Errorf("Create with past expiry error = %v, want ErrExpiryPast", err)
	}
}

func TestServiceResolveNoScheme(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	link, err := svc.Create(ctx, "example.com/no-scheme", "")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	target, err := svc.Resolve(ctx, link.HashKey)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if target != "http://example.com/no-scheme" {
		t.Errorf("Resolve = %q, want scheme defaulted to http", target)
	}
}
