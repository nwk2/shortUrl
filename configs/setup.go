package configs

import (
	// "fmt"
	"shortUrl/models"
	// "log"

	"github.com/jinzhu/gorm"
	// _ "github.com/jinzhu/gorm/dialects/postgres"
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

// placeholder for Postgres DB connection

// func ConnectPGDatabase() {
// 	config, err := LoadConfig("./util")
// 	if err != nil {
// 		log.Fatal("Failed to load config file:", err)
// 	}
// 	log.Println("config from YAML file: ", config)

// 	// db_host := config.DB_HOST
// 	// db_port := config.DB_PORT
// 	// db_name := config.DB_NAME
// 	// db_user := config.DB_USER
// 	// db_password := config.DB_PASSWORD

// 	db_user := config.DB_USER
// 	db_password := config.DB_PASSWORD
// 	db_name := config.DB_NAME
// 	db_host := config.DB_HOST
// 	db_port := 24321

// 	pg_conn_name := fmt.Sprintf("host=%v port=%v user=%v dbname=%v password=%v sslmode=disable", db_host, db_port, db_user, db_name, db_password)
// 	log.Println("pg conn details -> ", pg_conn_name)
// 	db, err := gorm.Open("postgres", pg_conn_name)
// 	if err != nil {
// 		panic("Failed to connect to db")
// 	}
// 	db.LogMode(true)
// 	DB = db
// }
