package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"shortUrl/internal/model"
	"shortUrl/internal/service"
	"shortUrl/internal/store"
)

// setupTestHandler builds a Handler backed by a fresh in-memory store.
// No config file or global state is involved: the service gets its
// hostname directly, and each test gets its own database. A single open
// connection is required: :memory: databases are per-connection, so a pool
// larger than one would see an empty database on the second connection.
func setupTestHandler(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	// gorm v2: DB() returns (*sql.DB, error) instead of *sql.DB.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	// gorm v2: AutoMigrate returns an error instead of *gorm.DB.
	if err := db.AutoMigrate(&model.ShortUrl{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	st := store.New(db)
	h := New(service.New(st, "http://localhost:8080/redirect/"))

	r := gin.New()
	r.GET("/short-urls", h.ListShortUrls)
	r.GET("/short-urls/:hashKey", h.GetShortUrlByHashKey)
	r.POST("/short-urls", h.CreateShortUrl)
	r.GET("/redirect/:hashKey", h.GetRedirect)
	return r, st
}

func doRequest(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createShortUrl(t *testing.T, r *gin.Engine, body string) model.ShortUrl {
	t.Helper()
	w := doRequest(t, r, http.MethodPost, "/short-urls", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create returned %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created model.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("create response is not a ShortUrl: %v", err)
	}
	return created
}

func TestCreateShortUrlBodyTooLarge(t *testing.T) {
	r, _ := setupTestHandler(t)

	// 1MB of padding inside the URL: over the cap before it even parses.
	body := `{"original_url":"https://example.com/` + strings.Repeat("a", 1<<20) + `"}`
	w := doRequest(t, r, http.MethodPost, "/short-urls", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("oversized body returned %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlerDBErrorIsGeneric(t *testing.T) {
	// A store whose table was never migrated: every query fails with
	// "no such table". The API must answer 500 with a generic message,
	// not the SQLite error.
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer func() { _ = sqlDB.Close() }()

	h := New(service.New(store.New(db), "http://localhost:8080/redirect/"))
	r := gin.New()
	r.GET("/short-urls", h.ListShortUrls)
	r.POST("/short-urls", h.CreateShortUrl)

	w := doRequest(t, r, http.MethodGet, "/short-urls", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("list on broken db returned %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if strings.Contains(w.Body.String(), "no such table") {
		t.Errorf("list leaked the database error: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Internal server error") {
		t.Errorf("list body = %s, want the generic message", w.Body.String())
	}

	w = doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"https://example.com"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("create on broken db returned %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if strings.Contains(w.Body.String(), "no such table") {
		t.Errorf("create leaked the database error: %s", w.Body.String())
	}
}

func TestCreateShortUrl(t *testing.T) {
	r, _ := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)

	if created.HashKey == "" {
		t.Error("hash_key is empty")
	}
	if !strings.HasSuffix(created.ShortUrl, created.HashKey) {
		t.Errorf("short_url %q does not end with hash_key %q", created.ShortUrl, created.HashKey)
	}
	if created.OriginalUrl != "https://example.com/some/path" {
		t.Errorf("original_url = %q", created.OriginalUrl)
	}
	// No expiry given: defaults to exactly 30 days after creation.
	if got := created.ExpiryDate.Sub(created.CreatedDate); got != 30*24*time.Hour {
		t.Errorf("expiry - created = %v, want 720h", got)
	}
}

func TestCreateShortUrlCustomExpiry(t *testing.T) {
	r, _ := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com","expiry_date":"2030-06-15 10:30"}`)

	// Parsed in the server's local timezone, like the implementation does.
	want, _ := time.ParseInLocation("2006-01-02 15:04", "2030-06-15 10:30", time.Local)
	if !created.ExpiryDate.Equal(want) {
		t.Errorf("expiry_date = %v, want %v", created.ExpiryDate, want)
	}
}

func TestCreateShortUrlEmptyBody(t *testing.T) {
	r, _ := setupTestHandler(t)

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty body returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateShortUrlInvalidURL(t *testing.T) {
	r, _ := setupTestHandler(t)

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"%zz"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid url returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateShortUrlBadExpiryFormat(t *testing.T) {
	r, _ := setupTestHandler(t)

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"https://example.com","expiry_date":"not-a-date"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad expiry format returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateShortUrlPastExpiry(t *testing.T) {
	r, _ := setupTestHandler(t)

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"https://example.com","expiry_date":"2020-01-01 00:00"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("past expiry returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetShortUrlByHashKey(t *testing.T) {
	r, _ := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)

	w := doRequest(t, r, http.MethodGet, "/short-urls/"+created.HashKey, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get returned %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Canonical response shape is the bare object, same as create.
	// (It used to be wrapped in {"data": ...}.)
	if strings.Contains(w.Body.String(), `"data":`) {
		t.Errorf("get response is wrapped in a data envelope: %s", w.Body.String())
	}
	var got model.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("get response is not a ShortUrl: %v", err)
	}
	if got.HashKey != created.HashKey {
		t.Errorf("hash_key = %q, want %q", got.HashKey, created.HashKey)
	}
}

func TestGetShortUrlByHashKeyNotFound(t *testing.T) {
	r, _ := setupTestHandler(t)

	w := doRequest(t, r, http.MethodGet, "/short-urls/doesnotexist", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown hash returned %d, want %d: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func expireLink(t *testing.T, st *store.Store, hashKey string) {
	t.Helper()
	if err := st.UpdateExpiry(context.Background(), hashKey, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("failed to expire link: %v", err)
	}
}

func TestGetShortUrlExpired(t *testing.T) {
	r, st := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)
	expireLink(t, st, created.HashKey)

	w := doRequest(t, r, http.MethodGet, "/short-urls/"+created.HashKey, "")
	if w.Code != http.StatusGone {
		t.Errorf("expired link returned %d, want %d: %s", w.Code, http.StatusGone, w.Body.String())
	}
}

func TestRedirect(t *testing.T) {
	r, _ := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)

	w := doRequest(t, r, http.MethodGet, "/redirect/"+created.HashKey, "")
	if w.Code != http.StatusFound {
		t.Fatalf("redirect returned %d, want %d: %s", w.Code, http.StatusFound, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/some/path" {
		t.Errorf("Location = %q, want original url", loc)
	}
}

func TestRedirectNotFound(t *testing.T) {
	r, _ := setupTestHandler(t)

	w := doRequest(t, r, http.MethodGet, "/redirect/doesnotexist", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown hash redirect returned %d, want %d: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestRedirectExpired(t *testing.T) {
	r, st := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)
	expireLink(t, st, created.HashKey)

	w := doRequest(t, r, http.MethodGet, "/redirect/"+created.HashKey, "")
	if w.Code != http.StatusGone {
		t.Errorf("expired redirect returned %d, want %d: %s", w.Code, http.StatusGone, w.Body.String())
	}
}

func TestListShortUrlsPagination(t *testing.T) {
	r, _ := setupTestHandler(t)

	createShortUrl(t, r, `{"original_url":"https://example.com/one"}`)
	createShortUrl(t, r, `{"original_url":"https://example.com/two"}`)
	createShortUrl(t, r, `{"original_url":"https://example.com/three"}`)

	// First page: two entries.
	w := doRequest(t, r, http.MethodGet, "/short-urls?limit=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list returned %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var page []model.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("list response is not a ShortUrl array: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("page 1 returned %d entries, want 2", len(page))
	}

	// Second page: the remaining entry.
	w = doRequest(t, r, http.MethodGet, "/short-urls?limit=2&offset=2", "")
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("list response is not a ShortUrl array: %v", err)
	}
	if len(page) != 1 {
		t.Errorf("page 2 returned %d entries, want 1", len(page))
	}

	// No params: the default page holds all three.
	w = doRequest(t, r, http.MethodGet, "/short-urls", "")
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("list response is not a ShortUrl array: %v", err)
	}
	if len(page) != 3 {
		t.Errorf("default list returned %d entries, want 3", len(page))
	}
}

func TestListShortUrlsInvalidPagination(t *testing.T) {
	r, _ := setupTestHandler(t)

	for _, path := range []string{
		"/short-urls?limit=abc",
		"/short-urls?limit=0",
		"/short-urls?limit=-1",
		"/short-urls?offset=-1",
		"/short-urls?offset=abc",
	} {
		w := doRequest(t, r, http.MethodGet, path, "")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s returned %d, want %d", path, w.Code, http.StatusBadRequest)
		}
	}
}

func TestListShortUrlsExcludesExpired(t *testing.T) {
	r, st := setupTestHandler(t)

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)
	expireLink(t, st, created.HashKey)

	w := doRequest(t, r, http.MethodGet, "/short-urls", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list returned %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var list []model.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list response is not a ShortUrl array: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("list returned %d entries, want 0 (expired links are excluded)", len(list))
	}
}

func TestListShortUrls(t *testing.T) {
	r, _ := setupTestHandler(t)

	createShortUrl(t, r, `{"original_url":"https://example.com/one"}`)
	createShortUrl(t, r, `{"original_url":"https://example.com/two"}`)

	w := doRequest(t, r, http.MethodGet, "/short-urls", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list returned %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var list []model.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list response is not a ShortUrl array: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list returned %d entries, want 2", len(list))
	}
}
