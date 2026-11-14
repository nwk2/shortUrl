package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app_config.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return dir
}

func TestLoadFromFile(t *testing.T) {
	dir := writeConfig(t, "APP_HOSTNAME: \"http://example.com/r/\"\nRATE_LIMIT_PER_MINUTE: 10\nAPI_KEY: \"secret\"\n")
	cfg, err := LoadFrom(dir)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if cfg.AppHostname != "http://example.com/r/" {
		t.Errorf("AppHostname = %q", cfg.AppHostname)
	}
	if cfg.RateLimitPerMinute != 10 {
		t.Errorf("RateLimitPerMinute = %d, want 10", cfg.RateLimitPerMinute)
	}
	if cfg.APIKey != "secret" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	// Keys absent from the file keep their defaults.
	if cfg.DBPath != "test.db" {
		t.Errorf("DBPath = %q, want the default", cfg.DBPath)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "*" {
		t.Errorf("CORSAllowedOrigins = %v, want the default [\"*\"]", cfg.CORSAllowedOrigins)
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	if _, err := LoadFrom(t.TempDir()); err == nil {
		t.Error("LoadFrom on a directory without app_config.yml returned no error")
	}
}

func TestLoadFromBadYAML(t *testing.T) {
	dir := writeConfig(t, "APP_HOSTNAME: [unclosed\n")
	if _, err := LoadFrom(dir); err == nil {
		t.Error("LoadFrom on invalid YAML returned no error")
	}
}

func TestLoadFromEnvOverride(t *testing.T) {
	dir := writeConfig(t, "RATE_LIMIT_PER_MINUTE: 10\nAPI_KEY: \"filekey\"\n")
	t.Setenv("API_KEY", "envkey")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "42")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example, https://b.example")

	cfg, err := LoadFrom(dir)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if cfg.APIKey != "envkey" {
		t.Errorf("APIKey = %q, want the env value", cfg.APIKey)
	}
	if cfg.RateLimitPerMinute != 42 {
		t.Errorf("RateLimitPerMinute = %d, want 42", cfg.RateLimitPerMinute)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[0] != "https://a.example" || cfg.CORSAllowedOrigins[1] != "https://b.example" {
		t.Errorf("CORSAllowedOrigins = %v, want the comma-split env value", cfg.CORSAllowedOrigins)
	}
}

func TestLoadFromBadEnvInt(t *testing.T) {
	dir := writeConfig(t, "")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "not-a-number")
	if _, err := LoadFrom(dir); err == nil {
		t.Error("LoadFrom with a non-numeric RATE_LIMIT_PER_MINUTE returned no error")
	}
}
