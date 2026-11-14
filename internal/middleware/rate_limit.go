package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// window is one IP's fixed one-minute counter.
type window struct {
	count     int
	startedAt time.Time
}

// pruneStaleWindows drops windows whose minute has ended. It is pure so
// tests can call it directly; the middleware calls it lazily, at most
// once a minute, on an incoming request.
func pruneStaleWindows(windows map[string]*window, now time.Time) {
	for ip, w := range windows {
		if now.Sub(w.startedAt) >= time.Minute {
			delete(windows, ip)
		}
	}
}

// RateLimitMiddleware caps requests per client IP to requestsPerMinute,
// using a fixed one-minute window per IP kept in memory. requestsPerMinute
// <= 0 disables limiting. Exceeding the cap returns 429 with a Retry-After
// header (seconds until the current window ends). Stale windows are pruned
// lazily, at most once a minute, on an incoming request — there is no
// background goroutine, so nothing leaks when the engine is discarded.
func RateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	if requestsPerMinute <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	var (
		mu        sync.Mutex
		windows   = make(map[string]*window)
		lastPrune = time.Now()
	)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		mu.Lock()
		if now.Sub(lastPrune) >= time.Minute {
			pruneStaleWindows(windows, now)
			lastPrune = now
		}
		w, ok := windows[ip]
		if !ok || now.Sub(w.startedAt) >= time.Minute {
			w = &window{count: 0, startedAt: now}
			windows[ip] = w
		}
		w.count++
		count := w.count
		resetIn := time.Minute - now.Sub(w.startedAt)
		mu.Unlock()

		if count > requestsPerMinute {
			c.Header("Retry-After", strconv.Itoa(int(resetIn/time.Second)+1))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			return
		}
		c.Next()
	}
}
