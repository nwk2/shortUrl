package configs

import (
	"github.com/spf13/viper"
)

type AppConfig struct {
	APP_HOSTNAME string `mapstructure:"APP_HOSTNAME"`
	// Origins allowed to call the API from a browser. ["*"] allows any
	// origin (no credentials); a list of explicit origins echoes the
	// matching one back with credentials allowed.
	CORS_ALLOWED_ORIGINS []string `mapstructure:"CORS_ALLOWED_ORIGINS"`
	// Max requests per minute per client IP, across all routes.
	// 0 or negative disables rate limiting.
	RATE_LIMIT_PER_MINUTE int `mapstructure:"RATE_LIMIT_PER_MINUTE"`
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
