package configs

import (
	"log"

	"github.com/spf13/viper"
)

type DbConfig struct {
	DB_HOST     string `mapstructure:"POSTGRES_HOST"`
	DB_PORT     string `mapstructure:"POSTGRES_PASSWORD"`
	DB_NAME     string `mapstructure:"POSTGRES_DB"`
	DB_USER     string `mapstructure:"POSTGRES_USER"`
	DB_PASSWORD string `mapstructure:"POSTGRES_PASSWORD"`
}

type AppConfig struct {
	APP_HOSTNAME string `mapstructure:"APP_HOSTNAME"`
}

var configPath = "./configs"

func LoadDbConfig(path string) (config DbConfig, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("dbconfig")
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

func LoadAppConfig() (appConfig AppConfig, err error) {
	viper.AddConfigPath(configPath)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		log.Fatal(err)
		return
	}

	err = viper.Unmarshal(&appConfig)
	return
}
