package middlewares

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitMiddleware caps requests per client IP to requestsPerMinute,
// using a fixed one-minute window per IP kept in memory. requestsPerMinute
// <= 0 disables limiting. Exceeding the cap returns 429 with a Retry-After
// header (seconds until the current window ends).
func RateLimitMiddleware(requestsPerMinute int) gin.HandlerFunc {
	if requestsPerMinute <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	type window struct {
		count     int
		startedAt time.Time
	}
	var (
		mu      sync.Mutex
		windows = make(map[string]*window)
	)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		mu.Lock()
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
			c.Header("Retry-After", itoa(int(resetIn/time.Second)+1))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			return
		}
		c.Next()
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
