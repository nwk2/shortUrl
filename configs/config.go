package configs

import (
	"github.com/spf13/viper"
)

type AppConfig struct {
	APP_HOSTNAME string `mapstructure:"APP_HOSTNAME"`
}

var configPath = "./configs"

func LoadAppConfig() (appConfig AppConfig, err error) {
	viper.AddConfigPath(configPath)
	viper.SetConfigName("app_config")
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&appConfig)
	return
}
