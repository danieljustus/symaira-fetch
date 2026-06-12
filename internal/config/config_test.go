package config

import (
	"os"
	"path/filepath"
	"testing"
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
