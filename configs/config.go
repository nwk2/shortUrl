package configs

import (
	"github.com/spf13/viper"
	"log"
)

type Config struct {
	DB_HOST     string `mapstructure:"POSTGRES_HOST"`
	DB_PORT     string `mapstructure:"POSTGRES_PASSWORD"`
	DB_NAME     string `mapstructure:"POSTGRES_DB"`
	DB_USER     string `mapstructure:"POSTGRES_USER"`
	DB_PASSWORD string `mapstructure:"POSTGRES_PASSWORD"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
		return
	}

	err = viper.Unmarshal(&config)
	return
}
