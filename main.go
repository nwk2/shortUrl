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

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"res": "pong"})
	})

	configs.ConnectDb()

	r.GET("/short-urls", api.ListShortUrls)
	r.GET("/short-urls/:hashKey", api.GetShortUrlByHashKey)
	r.POST("/short-urls", api.CreateShortUrl)

	r.GET("/redirect/:hashKey", api.GetRedirect)

	r.Run()
}
