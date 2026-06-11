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
	resetCache()
	t.Setenv("SYMFETCH_PROXY", "http://proxy:8080")
	t.Setenv("SYMFETCH_PROFILE", "firefox")

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
	resetCache()
}

func TestMergeFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[http]
profile = "honest"
timeout_seconds = 60
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	if err := mergeFile(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Profile != "honest" {
		t.Errorf("expected profile honest, got %s", cfg.HTTP.Profile)
	}
	if cfg.HTTP.TimeoutSeconds != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.HTTP.TimeoutSeconds)
	}
	// Defaults should be preserved
	if cfg.HTTP.MaxBodyMB != 10 {
		t.Errorf("expected max_body_mb default 10, got %d", cfg.HTTP.MaxBodyMB)
	}
}

func TestMergeFileMissing(t *testing.T) {
	cfg := Defaults()
	if err := mergeFile(cfg, "/nonexistent/path/config.toml"); err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}
}
