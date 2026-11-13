package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"shortUrl/internal/config"
	"shortUrl/internal/handler"
	"shortUrl/internal/middleware"
	"shortUrl/internal/service"
	"shortUrl/internal/store"
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
	protected := r.Group("/", middleware.ApiKeyAuth(cfg.APIKey))
	protected.GET("/short-urls", h.ListShortUrls)
	protected.POST("/short-urls", h.CreateShortUrl)

	r.GET("/short-urls/:hashKey", h.GetShortUrlByHashKey)

	r.GET("/redirect/:hashKey", h.GetRedirect)

	r.Run()
}
