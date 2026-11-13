package api

import (
	"errors"
	"hash/fnv"
	"log"
	"net/http"
	"strconv"
	"time"
	"net/url"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"shortUrl/configs"
	"shortUrl/models"
)

// GET /short-urls
// List all short urls
func ListShortUrls(c *gin.Context) {
	var shortUrls []models.ShortUrl
	configs.DB.Find(&shortUrls)

	c.JSON(http.StatusOK, shortUrls)
}

// GET /short-urls/:hash_key
func GetShortUrlByHashKey(c *gin.Context) {
	var shortUrl models.ShortUrl
	err := configs.DB.Where("hash_key = ?", c.Param("hashKey")).First(&shortUrl).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ShortUrl not found"})
		return
	}
	if isExpired(shortUrl) {
		c.JSON(http.StatusGone, gin.H{"error": "ShortUrl expired"})
		return
	}
	log.Println(shortUrl)
	c.JSON(http.StatusOK, shortUrl)
}

// GET /redirect/:hash_key
func GetRedirect(c *gin.Context) {
	var shortUrl models.ShortUrl
	err := configs.DB.Where("hash_key = ?", c.Param("hashKey")).First(&shortUrl).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ShortUrl not found"})
		return
	}
	if isExpired(shortUrl) {
		c.JSON(http.StatusGone, gin.H{"error": "ShortUrl expired"})
		return
	}
	redirectUrl, _ := url.Parse(shortUrl.OriginalUrl)
	if redirectUrl.Scheme == "" {
		log.Println("Original url does not have scheme. Defaulting to http protocol...")
		redirectUrl.Scheme = "http"
	}
	log.Println("Getting redirected to: ", redirectUrl)
	c.Redirect(http.StatusFound, redirectUrl.String())
}

// POST /short-urls
// Create new short url
func CreateShortUrl(c *gin.Context) {
	var input models.CreateShortUrlInput

	// TODO: how to validate if invalid fields are provided in request body?
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validatedUrl, err := url.Parse(input.OriginalUrl)
	if err != nil || validatedUrl.String() == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid url provided..."})
		return
	}

	expiryTS, createdTS, err := formatTimestamps(input.ExpiryDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid expiry date, expected format YYYY-MM-DD HH:MM"})
		return
	}

	if expiryTS.Before(time.Now()) {
		log.Println("expiryTS is: ", expiryTS)
		log.Println("time.Now() is: ", time.Now())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Expiry date must be a future timestamp"})
		return
	}

	appConfig, err := configs.LoadAppConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load app config"})
		return
	}

	// Hash is time-based, so collisions are unlikely but possible if two
	// creates land in the same instant. Retry with a different input rather
	// than storing a duplicate hash_key (unique index on the column).
	var hashValueStr string
	for attempt := 0; attempt < 5; attempt++ {
		hashKey := time.Now().String() + strconv.Itoa(attempt)
		hashValueStr = strconv.FormatUint(hash(hashKey, validatedUrl.String()), 10)

		var existing models.ShortUrl
		err := configs.DB.Where("hash_key = ?", hashValueStr).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			break
		}
		hashValueStr = ""
	}
	if hashValueStr == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate a unique short url, please retry"})
		return
	}

	shortened := appConfig.APP_HOSTNAME + hashValueStr

	shortUrl := models.ShortUrl{
		ShortUrl:    shortened,
		OriginalUrl: validatedUrl.String(),
		HashKey:     hashValueStr,
		CreatedDate: createdTS,
		ExpiryDate:  expiryTS,
	}

	if err := configs.DB.Create(&shortUrl).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, shortUrl)
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
		log.Println("Expiry_date no provided. Default expiry date will be set to 30 days after creation data.")
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

func isExpired(shortUrl models.ShortUrl) bool {
	return time.Now().After(shortUrl.ExpiryDate)
}
