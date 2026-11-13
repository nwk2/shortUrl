package main

import (
	"log"
	"net/http"

	"shortUrl/api"
	"shortUrl/configs"
	"shortUrl/middlewares"

	"github.com/gin-gonic/gin"
)

func main() {
	appConfig, err := configs.LoadAppConfig()
	if err != nil {
		log.Println("Failed to load app config, using defaults:", err)
		appConfig = configs.AppConfig{
			CORS_ALLOWED_ORIGINS:  []string{"*"},
			RATE_LIMIT_PER_MINUTE: 100,
		}
	}

	r := gin.Default()
	r.Use(middlewares.CorsMiddleware(appConfig.CORS_ALLOWED_ORIGINS))
	r.Use(middlewares.RateLimitMiddleware(appConfig.RATE_LIMIT_PER_MINUTE))

	if appConfig.API_KEY == "" {
		log.Println("WARNING: API_KEY is not set, create/list endpoints are unauthenticated")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"res": "pong"})
	})

	configs.ConnectDb()

	// Create and list require the API key; reading one link and following
	// a redirect stay public (that is what a short link is for).
	protected := r.Group("/", middlewares.ApiKeyAuth(appConfig.API_KEY))
	protected.GET("/short-urls", api.ListShortUrls)
	protected.POST("/short-urls", api.CreateShortUrl)

	r.GET("/short-urls/:hashKey", api.GetShortUrlByHashKey)

	r.GET("/redirect/:hashKey", api.GetRedirect)

	r.Run()
}
