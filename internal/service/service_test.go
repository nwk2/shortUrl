package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"shortUrl/internal/model"
	"shortUrl/internal/store"
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
	if err := db.AutoMigrate(&model.ShortUrl{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return New(store.New(db), "http://localhost:8080/redirect/")
}

func TestGenerateHash(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		h, err := generateHash()
		if err != nil {
			t.Fatalf("generateHash returned error: %v", err)
		}
		if len(h) != 11 {
			t.Fatalf("hash %q has length %d, want 11", h, len(h))
		}
		for _, c := range h {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("hash %q contains non-base62 rune %q", h, c)
			}
		}
		if seen[h] {
			t.Fatalf("duplicate hash %q in 1000 draws", h)
		}
		seen[h] = true
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
	want, _ := time.Parse("2006-01-02 15:04", "2030-06-15 10:30")
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
	stored := model.ShortUrl{
		ShortUrl:    "http://localhost:8080/redirect/oldbad1",
		OriginalUrl: "javascript:alert(1)",
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
	if !strings.HasSuffix(link.ShortUrl, link.HashKey) {
		t.Errorf("short_url %q does not end with hash_key %q", link.ShortUrl, link.HashKey)
	}
	if link.OriginalUrl != "https://example.com/some/path" {
		t.Errorf("original_url = %q", link.OriginalUrl)
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
