package configs

import (
	"shortUrl/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDb() {
	database, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{
		// LogMode(true) equivalent: log all SQL statements, as before.
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		panic("Failed to connect to db")
	}

	if err := database.AutoMigrate(&models.ShortUrl{}); err != nil {
		panic("Failed to migrate db: " + err.Error())
	}
	DB = database
}
