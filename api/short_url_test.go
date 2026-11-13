package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"shortUrl/configs"
	"shortUrl/models"
)

// CreateShortUrl calls configs.LoadAppConfig, which reads ./configs relative
// to the working directory. go test runs with CWD set to this package's
// directory, so move up to the repo root first.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	if err := os.Chdir(".."); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// setupTestDB gives each test a fresh in-memory database. A single open
// connection is required: :memory: databases are per-connection, so a pool
// larger than one would see an empty database on the second connection.
func setupTestDB(t *testing.T) {
	t.Helper()
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
	if err := db.AutoMigrate(&models.ShortUrl{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	configs.DB = db
	t.Cleanup(func() { sqlDB.Close() })
}

func setupRouter() *gin.Engine {
	r := gin.New()
	r.GET("/short-urls", ListShortUrls)
	r.GET("/short-urls/:hashKey", GetShortUrlByHashKey)
	r.POST("/short-urls", CreateShortUrl)
	r.GET("/redirect/:hashKey", GetRedirect)
	return r
}

func doRequest(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createShortUrl(t *testing.T, r *gin.Engine, body string) models.ShortUrl {
	t.Helper()
	w := doRequest(t, r, http.MethodPost, "/short-urls", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create returned %d, want %d: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var created models.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("create response is not a ShortUrl: %v", err)
	}
	return created
}

func TestCreateShortUrl(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

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
	setupTestDB(t)
	r := setupRouter()

	created := createShortUrl(t, r, `{"original_url":"https://example.com","expiry_date":"2030-06-15 10:30"}`)

	want, _ := time.Parse("2006-01-02 15:04", "2030-06-15 10:30")
	if !created.ExpiryDate.Equal(want) {
		t.Errorf("expiry_date = %v, want %v", created.ExpiryDate, want)
	}
}

func TestCreateShortUrlEmptyBody(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty body returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateShortUrlInvalidURL(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"%zz"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid url returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateShortUrlBadExpiryFormat(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"https://example.com","expiry_date":"not-a-date"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad expiry format returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreateShortUrlPastExpiry(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	w := doRequest(t, r, http.MethodPost, "/short-urls", `{"original_url":"https://example.com","expiry_date":"2020-01-01 00:00"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("past expiry returned %d, want %d: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetShortUrlByHashKey(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

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
	var got models.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("get response is not a ShortUrl: %v", err)
	}
	if got.HashKey != created.HashKey {
		t.Errorf("hash_key = %q, want %q", got.HashKey, created.HashKey)
	}
}

func TestGetShortUrlByHashKeyNotFound(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	w := doRequest(t, r, http.MethodGet, "/short-urls/doesnotexist", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown hash returned %d, want %d: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func expireLink(t *testing.T, hashKey string) {
	t.Helper()
	if err := configs.DB.Model(&models.ShortUrl{}).Where("hash_key = ?", hashKey).Update("expiry_date", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("failed to expire link: %v", err)
	}
}

func TestGetShortUrlExpired(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)
	expireLink(t, created.HashKey)

	w := doRequest(t, r, http.MethodGet, "/short-urls/"+created.HashKey, "")
	if w.Code != http.StatusGone {
		t.Errorf("expired link returned %d, want %d: %s", w.Code, http.StatusGone, w.Body.String())
	}
}

func TestRedirect(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

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
	setupTestDB(t)
	r := setupRouter()

	w := doRequest(t, r, http.MethodGet, "/redirect/doesnotexist", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown hash redirect returned %d, want %d: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestRedirectExpired(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	created := createShortUrl(t, r, `{"original_url":"https://example.com/some/path"}`)
	expireLink(t, created.HashKey)

	w := doRequest(t, r, http.MethodGet, "/redirect/"+created.HashKey, "")
	if w.Code != http.StatusGone {
		t.Errorf("expired redirect returned %d, want %d: %s", w.Code, http.StatusGone, w.Body.String())
	}
}

func TestListShortUrls(t *testing.T) {
	setupTestDB(t)
	r := setupRouter()

	createShortUrl(t, r, `{"original_url":"https://example.com/one"}`)
	createShortUrl(t, r, `{"original_url":"https://example.com/two"}`)

	w := doRequest(t, r, http.MethodGet, "/short-urls", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list returned %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var list []models.ShortUrl
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list response is not a ShortUrl array: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list returned %d entries, want 2", len(list))
	}
}
