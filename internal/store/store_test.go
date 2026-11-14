package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"shortUrl/internal/model"
)

func setupTestStore(t *testing.T) *Store {
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
	t.Cleanup(func() { _ = sqlDB.Close() })
	return New(db)
}

func insertLink(t *testing.T, st *Store, hashKey string, expiry time.Time) {
	t.Helper()
	link := model.ShortUrl{
		ShortUrl:    "http://localhost:8080/redirect/" + hashKey,
		OriginalUrl: "https://example.com/" + hashKey,
		HashKey:     hashKey,
		CreatedDate: time.Now(),
		ExpiryDate:  expiry,
	}
	if err := st.Create(context.Background(), &link); err != nil {
		t.Fatalf("failed to insert link %q: %v", hashKey, err)
	}
}

func TestCreateDuplicateHashIsConflict(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()

	insertLink(t, st, "abc1234", time.Now().Add(24*time.Hour))

	// Same hash (and, derived from it, the same short URL) again: the
	// insert must report a conflict, not a generic database error, so
	// the service can retry with a fresh hash.
	dup := model.ShortUrl{
		ShortUrl:    "http://localhost:8080/redirect/abc1234",
		OriginalUrl: "https://example.com/other",
		HashKey:     "abc1234",
		CreatedDate: time.Now(),
		ExpiryDate:  time.Now().Add(24 * time.Hour),
	}
	if err := st.Create(ctx, &dup); !errors.Is(err, ErrHashConflict) {
		t.Fatalf("duplicate Create returned %v, want ErrHashConflict", err)
	}
}

func TestStoreListPagination(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()
	future := time.Now().Add(24 * time.Hour)

	insertLink(t, st, "aaa", future)
	insertLink(t, st, "bbb", future)
	insertLink(t, st, "ccc", future)

	// First page: two rows, in creation order.
	page, err := st.List(ctx, 2, 0)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page) != 2 || page[0].HashKey != "aaa" || page[1].HashKey != "bbb" {
		t.Errorf("page 1 = %v, want [aaa bbb]", hashKeys(page))
	}

	// Second page: the remaining row.
	page, err = st.List(ctx, 2, 2)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page) != 1 || page[0].HashKey != "ccc" {
		t.Errorf("page 2 = %v, want [ccc]", hashKeys(page))
	}
}

func TestStoreListExcludesExpired(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()

	insertLink(t, st, "live", time.Now().Add(24*time.Hour))
	insertLink(t, st, "dead", time.Now().Add(-time.Hour))

	links, err := st.List(ctx, 50, 0)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(links) != 1 || links[0].HashKey != "live" {
		t.Errorf("List = %v, want only [live]", hashKeys(links))
	}
}

func TestStoreDeleteExpired(t *testing.T) {
	st := setupTestStore(t)
	ctx := context.Background()

	insertLink(t, st, "live", time.Now().Add(24*time.Hour))
	insertLink(t, st, "dead1", time.Now().Add(-time.Hour))
	insertLink(t, st, "dead2", time.Now().Add(-2*time.Hour))

	n, err := st.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired returned error: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteExpired removed %d rows, want 2", n)
	}

	// Expired rows are gone; the live one is untouched.
	if _, err := st.GetByHash(ctx, "dead1"); err != ErrNotFound {
		t.Errorf("GetByHash(dead1) error = %v, want ErrNotFound", err)
	}
	if _, err := st.GetByHash(ctx, "live"); err != nil {
		t.Errorf("GetByHash(live) error = %v, want nil", err)
	}

	// Second sweep is a no-op.
	n, err = st.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("second DeleteExpired removed %d rows, want 0", n)
	}
}

func hashKeys(links []model.ShortUrl) []string {
	keys := make([]string, len(links))
	for i, l := range links {
		keys[i] = l.HashKey
	}
	return keys
}
