package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"shorturl/internal/config"
	"shorturl/internal/handler"
	"shorturl/internal/middleware"
	"shorturl/internal/service"
	"shorturl/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Println("Failed to load app config, using defaults:", err)
		cfg = config.Default()
	}

	st, err := store.Open(cfg.DBPath, cfg.LogLevel)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	svc := service.New(st, cfg.AppHostname)
	h := handler.New(svc)

	r := gin.Default()
	r.Use(middleware.CorsMiddleware(cfg.CORSAllowedOrigins))
	r.Use(middleware.RateLimitMiddleware(cfg.RateLimitPerMinute))

	if cfg.APIKey == "" {
		log.Println("WARNING: API_KEY is not set, create/list endpoints are unauthenticated")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"res": "pong"})
	})

	// Create and list require the API key; reading one link and following
	// a redirect stay public (that is what a short link is for).
	protected := r.Group("/", middleware.APIKeyAuth(cfg.APIKey))
	protected.GET("/short-urls", h.ListShortURLs)
	protected.POST("/short-urls", h.CreateShortURL)

	r.GET("/short-urls/:hashKey", h.GetShortURLByHashKey)

	r.GET("/redirect/:hashKey", h.GetRedirect)

	// Expired links are swept hourly so they don't accumulate forever.
	stopSweeper := make(chan struct{})
	go sweepExpiredLinks(st, time.Hour, stopSweeper)

	// Timeouts bound how long a slow client can hold a connection; the
	// shutdown below lets in-flight requests finish instead of being cut
	// off by SIGTERM.
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	// ListenAndServe runs in a goroutine so main can wait for a shutdown
	// signal; a startup failure is reported on serverErr instead of
	// log.Fatal, which would exit the process without running deferred
	// cleanup.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		log.Println("Shutting down...")
	case err := <-serverErr:
		log.Println("Server failed:", err)
		close(stopSweeper)
		os.Exit(1)
	}
	close(stopSweeper)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}
	log.Println("Server stopped")
}

// sweepExpiredLinks deletes expired links every interval until stop is
// closed (at shutdown). It only logs when it actually removed something.
func sweepExpiredLinks(st *store.Store, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			n, err := st.DeleteExpired(context.Background())
			if err != nil {
				log.Println("Expiry sweep failed:", err)
				continue
			}
			if n > 0 {
				log.Printf("Expiry sweep removed %d expired links", n)
			}
		}
	}
}
