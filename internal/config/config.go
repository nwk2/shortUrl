package config

import (
	"github.com/spf13/viper"
)

// Config is everything the server needs at startup. Values come from
// configs/app_config.yml, with environment variables overriding them
// (viper AutomaticEnv), and the defaults in Default() when neither is set.
// It is loaded once in main and passed down — handlers never re-read it.
type Config struct {
	AppHostname string `mapstructure:"APP_HOSTNAME"`
	// Origins allowed to call the API from a browser. ["*"] allows any
	// origin (no credentials); a list of explicit origins echoes the
	// matching one back with credentials allowed.
	CORSAllowedOrigins []string `mapstructure:"CORS_ALLOWED_ORIGINS"`
	// Max requests per minute per client IP, across all routes.
	// 0 or negative disables rate limiting.
	RateLimitPerMinute int `mapstructure:"RATE_LIMIT_PER_MINUTE"`
	// Shared secret for the create/list endpoints, checked against the
	// X-Api-Key header. Empty disables auth (endpoints are open) — the
	// server logs a warning at startup in that case.
	APIKey string `mapstructure:"API_KEY"`
	// SQLite file for the link table. Relative paths resolve against the
	// working directory the server starts in.
	DBPath string `mapstructure:"DB_PATH"`
	// GORM SQL log level: info, warn, error or silent.
	LogLevel string `mapstructure:"LOG_LEVEL"`
}

func Default() Config {
	return Config{
		AppHostname:        "http://localhost:8080/redirect/",
		CORSAllowedOrigins: []string{"*"},
		RateLimitPerMinute: 100,
		APIKey:             "",
		DBPath:             "test.db",
		LogLevel:           "info",
	}
}

func Load() (Config, error) {
	v := viper.New()
	v.AddConfigPath("./configs")
	v.SetConfigName("app_config")
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	d := Default()
	v.SetDefault("APP_HOSTNAME", d.AppHostname)
	v.SetDefault("CORS_ALLOWED_ORIGINS", d.CORSAllowedOrigins)
	v.SetDefault("RATE_LIMIT_PER_MINUTE", d.RateLimitPerMinute)
	v.SetDefault("API_KEY", d.APIKey)
	v.SetDefault("DB_PATH", d.DBPath)
	v.SetDefault("LOG_LEVEL", d.LogLevel)

	if err := v.ReadInConfig(); err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
