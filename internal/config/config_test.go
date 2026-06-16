package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.HTTP.TimeoutSeconds != 30 {
		t.Errorf("expected timeout 30, got %d", cfg.HTTP.TimeoutSeconds)
	}
	if cfg.HTTP.Profile != "chrome" {
		t.Errorf("expected profile chrome, got %s", cfg.HTTP.Profile)
	}
	if !cfg.Cache.Enabled {
		t.Error("expected cache enabled by default")
	}
	if cfg.Security.AllowPrivate {
		t.Error("expected allow_private false by default")
	}
}

func TestDefaultsAllFields(t *testing.T) {
	cfg := Defaults()

	if cfg.HTTP.MaxBodyMB != 10 {
		t.Errorf("expected MaxBodyMB 10, got %d", cfg.HTTP.MaxBodyMB)
	}
	if cfg.HTTP.DefaultFormat != "markdown" {
		t.Errorf("expected DefaultFormat 'markdown', got %q", cfg.HTTP.DefaultFormat)
	}
	if cfg.HTTP.MaxChars != 20000 {
		t.Errorf("expected MaxChars 20000, got %d", cfg.HTTP.MaxChars)
	}
	if cfg.HTTP.Concurrency != 4 {
		t.Errorf("expected Concurrency 4, got %d", cfg.HTTP.Concurrency)
	}
	if cfg.HTTP.Proxy != "" {
		t.Errorf("expected empty Proxy, got %q", cfg.HTTP.Proxy)
	}
	if cfg.Cache.TTL != 15*time.Minute {
		t.Errorf("expected Cache.TTL 15m, got %v", cfg.Cache.TTL)
	}
	if cfg.Cache.Dir != "" {
		t.Errorf("expected empty Cache.Dir, got %q", cfg.Cache.Dir)
	}
}

func TestDefaultConfigTOML(t *testing.T) {
	toml := DefaultConfigTOML()

	if len(toml) == 0 {
		t.Fatal("DefaultConfigTOML() returned empty string")
	}

	if !strings.Contains(toml, "[http]") {
		t.Error("TOML should contain [http] section")
	}
	if !strings.Contains(toml, "[cache]") {
		t.Error("TOML should contain [cache] section")
	}
	if !strings.Contains(toml, "[security]") {
		t.Error("TOML should contain [security] section")
	}
	if !strings.Contains(toml, `profile = "chrome"`) {
		t.Error("TOML should contain default profile")
	}
	if !strings.Contains(toml, `timeout_seconds = 30`) {
		t.Error("TOML should contain default timeout")
	}
	if !strings.Contains(toml, `enabled = true`) {
		t.Error("TOML should contain cache enabled")
	}
	if !strings.Contains(toml, `allow_private = false`) {
		t.Error("TOML should contain allow_private false")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	loader.ResetCache()
	t.Setenv("SYMFETCH_HTTP_PROXY", "http://proxy:8080")
	t.Setenv("SYMFETCH_HTTP_PROFILE", "firefox")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Proxy != "http://proxy:8080" {
		t.Errorf("expected proxy from env, got %q", cfg.HTTP.Proxy)
	}
	if cfg.HTTP.Profile != "firefox" {
		t.Errorf("expected profile firefox from env, got %q", cfg.HTTP.Profile)
	}
	loader.ResetCache()
}

func TestLoadEnvOverrideAllFields(t *testing.T) {
	loader.ResetCache()
	t.Setenv("SYMFETCH_HTTP_TIMEOUT_SECONDS", "60")
	t.Setenv("SYMFETCH_HTTP_MAX_BODY_MB", "20")
	t.Setenv("SYMFETCH_HTTP_DEFAULT_FORMAT", "json")
	t.Setenv("SYMFETCH_HTTP_MAX_CHARS", "10000")
	t.Setenv("SYMFETCH_HTTP_CONCURRENCY", "8")
	t.Setenv("SYMFETCH_CACHE_ENABLED", "false")
	t.Setenv("SYMFETCH_SECURITY_ALLOW_PRIVATE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTTP.TimeoutSeconds != 60 {
		t.Errorf("expected timeout 60 from env, got %d", cfg.HTTP.TimeoutSeconds)
	}
	if cfg.HTTP.MaxBodyMB != 20 {
		t.Errorf("expected MaxBodyMB 20 from env, got %d", cfg.HTTP.MaxBodyMB)
	}
	if cfg.HTTP.DefaultFormat != "json" {
		t.Errorf("expected DefaultFormat 'json' from env, got %q", cfg.HTTP.DefaultFormat)
	}
	if cfg.HTTP.MaxChars != 10000 {
		t.Errorf("expected MaxChars 10000 from env, got %d", cfg.HTTP.MaxChars)
	}
	if cfg.HTTP.Concurrency != 8 {
		t.Errorf("expected Concurrency 8 from env, got %d", cfg.HTTP.Concurrency)
	}
	if cfg.Cache.Enabled != false {
		t.Error("expected Cache.Enabled false from env")
	}
	if cfg.Security.AllowPrivate != true {
		t.Error("expected AllowPrivate true from env")
	}

	loader.ResetCache()
}

func TestMergeFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".symfetch.toml")
	content := `
[http]
profile = "honest"
timeout_seconds = 60
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader.ResetCache()

	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Profile != "honest" {
		t.Errorf("expected profile honest, got %s", cfg.HTTP.Profile)
	}
	if cfg.HTTP.TimeoutSeconds != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.HTTP.TimeoutSeconds)
	}
	if cfg.HTTP.MaxBodyMB != 10 {
		t.Errorf("expected max_body_mb default 10, got %d", cfg.HTTP.MaxBodyMB)
	}
	loader.ResetCache()
}

func TestMergeFileMissing(t *testing.T) {
	loader.ResetCache()
	orig, _ := os.Getwd()
	os.Chdir("/nonexistent/path")
	defer os.Chdir(orig)

	cfg, err := Load()
	if err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	loader.ResetCache()
}

func TestMergeFilePartial(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".symfetch.toml")
	content := `
[http]
profile = "firefox"
timeout_seconds = 45
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader.ResetCache()

	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTTP.Profile != "firefox" {
		t.Errorf("expected profile firefox, got %s", cfg.HTTP.Profile)
	}
	if cfg.HTTP.TimeoutSeconds != 45 {
		t.Errorf("expected timeout 45 from file, got %d", cfg.HTTP.TimeoutSeconds)
	}
	if cfg.HTTP.MaxBodyMB != 10 {
		t.Errorf("expected MaxBodyMB 10 (default), got %d", cfg.HTTP.MaxBodyMB)
	}
	if cfg.Cache.Enabled != true {
		t.Error("expected cache enabled (default)")
	}
	if cfg.Security.AllowPrivate != false {
		t.Error("expected allow_private false (default)")
	}

	loader.ResetCache()
}
