package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is everything the server needs at startup. Values come from
// configs/app_config.yml, with environment variables overriding them,
// and the defaults in Default() when neither is set. It is loaded once
// in main and passed down — handlers never re-read it.
type Config struct {
	AppHostname string `yaml:"APP_HOSTNAME"`
	// Origins allowed to call the API from a browser. ["*"] allows any
	// origin (no credentials); a list of explicit origins echoes the
	// matching one back with credentials allowed.
	CORSAllowedOrigins []string `yaml:"CORS_ALLOWED_ORIGINS"`
	// Max requests per minute per client IP, across all routes.
	// 0 or negative disables rate limiting.
	RateLimitPerMinute int `yaml:"RATE_LIMIT_PER_MINUTE"`
	// Shared secret for the create/list endpoints, checked against the
	// X-Api-Key header. Empty disables auth (endpoints are open) — the
	// server logs a warning at startup in that case.
	APIKey string `yaml:"API_KEY"`
	// SQLite file for the link table. Relative paths resolve against the
	// working directory the server starts in.
	DBPath string `yaml:"DB_PATH"`
	// GORM SQL log level: info, warn, error or silent.
	LogLevel string `yaml:"LOG_LEVEL"`
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

// Load reads configs/app_config.yml from the working directory.
func Load() (Config, error) {
	return LoadFrom("./configs")
}

// LoadFrom reads app_config.yml from dir, starting from Default() so
// keys absent from the file keep their defaults, then applies
// environment overrides. A missing or unreadable file is an error; the
// caller (main) falls back to Default() in that case.
func LoadFrom(dir string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(filepath.Join(dir, "app_config.yml"))
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// applyEnv overrides config values from environment variables of the
// same name. Unset or empty variables leave the value alone; a malformed
// RATE_LIMIT_PER_MINUTE is an error. The list key takes a
// comma-separated value ("https://a.example, https://b.example").
func applyEnv(cfg *Config) error {
	if v, ok := os.LookupEnv("APP_HOSTNAME"); ok && v != "" {
		cfg.AppHostname = v
	}
	if v, ok := os.LookupEnv("CORS_ALLOWED_ORIGINS"); ok && v != "" {
		cfg.CORSAllowedOrigins = splitList(v)
	}
	if v, ok := os.LookupEnv("RATE_LIMIT_PER_MINUTE"); ok && v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return err
		}
		cfg.RateLimitPerMinute = n
	}
	if v, ok := os.LookupEnv("API_KEY"); ok && v != "" {
		cfg.APIKey = v
	}
	if v, ok := os.LookupEnv("DB_PATH"); ok && v != "" {
		cfg.DBPath = v
	}
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok && v != "" {
		cfg.LogLevel = v
	}
	return nil
}

// splitList parses a comma-separated env list, trimming spaces and
// dropping empty entries.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
