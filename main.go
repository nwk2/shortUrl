package main

import (
	"net/http"

	"shortUrl/api"
	"shortUrl/configs"
	"shortUrl/middlewares"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(middlewares.CorsMiddleware())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"res": "pong"})
	})

	configs.ConnectDb()

	r.GET("/shortUrls", api.GetAllShortUrls)
	r.GET("/shortUrls/:hashKey", api.GetShortUrlByHashKey)
	r.POST("/shortUrls", api.CreateShortUrl)

	r.GET("redirect/:hash", api.GetRedirect)

	r.Run(":8080")
}
