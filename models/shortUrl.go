package models

import "time"

type ShortUrl struct {
	ID uint `json:"id" gorm:"primary_key"`
	ShortUrl string `json:"shortUrl"`
	OriginalUrl string `json:"originalUrl"`
	HashKey uint64 `json:"hashKey"`
	CreatedDate time.Time `json:"createdDate"`
	ExpiryDate time.Time `json:"expiryDate"`
}

type CreateShortUrlInput struct {
	OriginalUrl string `json:"originalUrl" binding:"required"`
	ExpiryDate string `json:"expiryDate"`
}