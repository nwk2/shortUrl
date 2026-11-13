package configs

import (
	"shortUrl/models"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

var DB *gorm.DB

func ConnectDb() {
	database, err := gorm.Open("sqlite3", "test.db")

	if err != nil {
		panic("Failed to connect to db")
	}

	database.LogMode(true)
	database.AutoMigrate(&models.ShortUrl{})
	DB = database
}
