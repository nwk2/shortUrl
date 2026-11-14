package model

import "time"

// ShortURL is one shortened link: the row in SQLite and the JSON body
// the API returns.
type ShortURL struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ShortURL    string    `json:"short_url" gorm:"uniqueIndex"`
	OriginalURL string    `json:"original_url"`
	HashKey     string    `json:"hash_key" gorm:"uniqueIndex"`
	CreatedDate time.Time `json:"created_date"`
	ExpiryDate  time.Time `json:"expiry_date"`
}

// CreateShortURLInput is the POST /short-urls request body.
type CreateShortURLInput struct {
	OriginalURL string `json:"original_url" binding:"required"`
	ExpiryDate  string `json:"expiry_date"`
}
