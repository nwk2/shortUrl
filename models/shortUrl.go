package models

import "time"

type ShortUrl struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ShortUrl    string    `json:"short_url" gorm:"uniqueIndex"`
	OriginalUrl string    `json:"original_url"`
	HashKey     string    `json:"hash_key" gorm:"uniqueIndex"`
	CreatedDate time.Time `json:"created_date"`
	ExpiryDate  time.Time `json:"expiry_date"`
}

type CreateShortUrlInput struct {
	OriginalUrl string `json:"original_url" binding:"required"`
	ExpiryDate  string `json:"expiry_date"`
}
