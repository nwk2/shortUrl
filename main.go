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

	r.GET("/short-urls", api.ListShortUrls)
	r.GET("/short-urls/:hashKey", api.GetShortUrlByHashKey)
	r.POST("/short-urls", api.CreateShortUrl)

	r.GET("/redirect/:hashKey", api.GetRedirect)

	r.Run()
}
