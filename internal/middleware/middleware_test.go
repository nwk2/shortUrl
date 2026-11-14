package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestCorsAllowAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorsMiddleware([]string{"*"}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://example.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want %q", got, "*")
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q, want empty (credentials are ignored with a * origin)", got)
	}
}

func TestCorsAllowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorsMiddleware([]string{"http://localhost:8000"}))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Matching origin is echoed back with credentials.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:8000")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8000" {
		t.Errorf("Allow-Origin = %q, want the request origin echoed", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want %q", got, "true")
	}

	// Non-listed origin gets no Allow-Origin header.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for a non-listed origin", got)
	}
}

func TestPruneStaleWindows(t *testing.T) {
	now := time.Now()
	windows := map[string]*window{
		"1.1.1.1": {count: 5, startedAt: now.Add(-2 * time.Minute)},  // stale
		"2.2.2.2": {count: 3, startedAt: now.Add(-30 * time.Second)}, // fresh
		"3.3.3.3": {count: 1, startedAt: now},                        // fresh
	}

	pruneStaleWindows(windows, now)

	if _, ok := windows["1.1.1.1"]; ok {
		t.Error("stale window (2m old) was not pruned")
	}
	if _, ok := windows["2.2.2.2"]; !ok {
		t.Error("fresh window (30s old) was pruned")
	}
	if _, ok := windows["3.3.3.3"]; !ok {
		t.Error("fresh window (just started) was pruned")
	}
}

func TestRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitMiddleware(2))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 1; i <= 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		if i <= 2 && w.Code != http.StatusOK {
			t.Fatalf("request %d returned %d, want %d", i, w.Code, http.StatusOK)
		}
		if i == 3 {
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("request 3 returned %d, want %d", w.Code, http.StatusTooManyRequests)
			}
			if w.Header().Get("Retry-After") == "" {
				t.Error("429 response is missing a Retry-After header")
			}
		}
	}
}

func TestRateLimitDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitMiddleware(0))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d returned %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}
}

func TestAPIKeyAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(APIKeyAuth("secret"))
	r.POST("/short-urls", func(c *gin.Context) { c.Status(http.StatusCreated) })

	// No header -> 401.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/short-urls", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no header returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Wrong key -> 401.
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/short-urls", nil)
	req.Header.Set("X-Api-Key", "wrong")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong key returned %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Correct key -> through.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/short-urls", nil)
	req.Header.Set("X-Api-Key", "secret")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("correct key returned %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestAPIKeyAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(APIKeyAuth(""))
	r.POST("/short-urls", func(c *gin.Context) { c.Status(http.StatusCreated) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/short-urls", nil))
	if w.Code != http.StatusCreated {
		t.Errorf("empty key returned %d, want %d (auth disabled)", w.Code, http.StatusCreated)
	}
}
