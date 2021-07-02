package api

import (
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"shortUrl/configs"
	"shortUrl/models"
)

// GET /shortUrls
// Get all short urls
func GetAllShortUrls(c *gin.Context) {
	var shortUrls []models.ShortUrl
	configs.DB.Find(&shortUrls)

	c.JSON(http.StatusOK, shortUrls)
}

// GET /shortUrls/:hash_key
func GetShortUrlByHashKey(c *gin.Context) {
	var shortUrl models.ShortUrl
	fmt.Println(c.Param("hashKey"))
	err := configs.DB.Where("hash_key = ?", c.Param("hashKey")).First(&shortUrl).Error
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ShortUrl not found"})
		return
	}
	fmt.Println(shortUrl)
	c.JSON(http.StatusOK, gin.H{"data": shortUrl})
}

// GET /redirect/:hash_key
func GetRedirect(c *gin.Context) {
	var shortUrl models.ShortUrl
	err := configs.DB.Where("hash_key = ?", c.Param("hash")).First(&shortUrl).Error
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ShortUrl not found"})
		return
	}
	log.Println("Getting redirected to: ", shortUrl)
	c.Redirect(http.StatusMovedPermanently, shortUrl.OriginalUrl)
}

// POST /shortUrls
// Create new short url
func CreateShortUrl(c *gin.Context) {
	var input models.CreateShortUrlInput

	// TODO: how to validate if invalid fields are provided in request body?
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expiryTS, createdTS, _ := formatTimestamps(input.ExpiryDate)

	if expiryTS.Before(time.Now()) {
		log.Println("expiryTS is: ", expiryTS)
		log.Println("time.Now() is: ", time.Now())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Expiry date must be a future timestamp"})
		return
	}

	var hashValue uint64

	hashKey := time.Now().String()
	hashValue = hash(hashKey, input.OriginalUrl)

	hashValueStr := strconv.FormatUint(hashValue, 10)

	appConfig, _ := configs.LoadAppConfig()

	shortened := appConfig.APP_HOSTNAME + hashValueStr

	shortUrl := models.ShortUrl{
		ShortUrl:    shortened,
		OriginalUrl: input.OriginalUrl,
		HashKey:     hashValueStr,
		CreatedDate: createdTS,
		ExpiryDate:  expiryTS,
	}

	configs.DB.Create(&shortUrl)
	c.JSON(http.StatusCreated, shortUrl)

	// after creation we have ID and original_url
	// https://stackoverflow.com/questions/742013/how-do-i-create-a-url-shortener
}

// TODO: How to handle multiple errors?
func formatTimestamps(expiryTS string) (parsedExpiryTS time.Time, parsedCreatedTS time.Time, err error) {
	formatString := "2006-01-02 15:04"
	currentTS := time.Now().Format(formatString)

	if len(expiryTS) > 0 {
		parsedExpiryTS, err = time.Parse(formatString, expiryTS)
		log.Println("Provided expiry date is: ", parsedExpiryTS)
		if err != nil {
			log.Println("error parsing expiry date:", err)
			return time.Time{}, time.Time{}, err
		}
	} else {
		log.Println("Expiry_date no provided. Default expiry date will be set to 5 days after creation data.")
		parsedExpiryTS, err = time.Parse(formatString, currentTS)
		if err != nil {
			log.Println("error parsing expiry date:", err)
			return time.Time{}, time.Time{}, err
		}
		parsedExpiryTS = parsedExpiryTS.AddDate(0, 0, 30)
	}

	parsedCreatedTS, err = time.Parse(formatString, currentTS)

	if err != nil {
		log.Println("error parsing created date:", err)
		return time.Time{}, time.Time{}, err
	}

	return parsedExpiryTS, parsedCreatedTS, nil
}

func hash(hashKey string, originalUrl string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(hashKey + originalUrl))
	return h.Sum64()
}
