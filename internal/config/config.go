package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

var (
	cachedCfg  *Config
	cachedOnce sync.Once
	cachedErr  error
)

// Config holds all runtime configuration loaded from TOML files.
type Config struct {
	HTTP     HTTPConfig     `toml:"http"`
	Cache    CacheConfig    `toml:"cache"`
	Security SecurityConfig `toml:"security"`
}

type HTTPConfig struct {
	Proxy          string        `toml:"proxy"`
	TimeoutSeconds int           `toml:"timeout_seconds"`
	MaxBodyMB      int           `toml:"max_body_mb"`
	Profile        string        `toml:"profile"`
	DefaultFormat  string        `toml:"default_format"`
	MaxChars       int           `toml:"max_chars"`
	Concurrency    int           `toml:"concurrency"`
}

type CacheConfig struct {
	Enabled bool          `toml:"enabled"`
	TTL     time.Duration `toml:"ttl"`
	Dir     string        `toml:"dir"`
}

type SecurityConfig struct {
	AllowPrivate bool `toml:"allow_private"`
}

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	return &Config{
		HTTP: HTTPConfig{
			TimeoutSeconds: 30,
			MaxBodyMB:      10,
			Profile:        "chrome",
			DefaultFormat:  "markdown",
			MaxChars:       20000,
			Concurrency:    4,
		},
		Cache: CacheConfig{
			Enabled: true,
			TTL:     15 * time.Minute,
		},
		Security: SecurityConfig{
			AllowPrivate: false,
		},
	}
}

// Load reads the global config from ~/.config/symfetch/config.toml,
// then merges a project-level .symfetch.toml override if present.
// The config is loaded once and cached for subsequent calls.
func Load() (*Config, error) {
	cachedOnce.Do(func() {
		cachedCfg, cachedErr = loadOnce()
	})
	return cachedCfg, cachedErr
}

// resetCache clears the cached config so the next Load() call reads from disk again.
// Used only by tests.
func resetCache() {
	cachedCfg = nil
	cachedErr = nil
	cachedOnce = sync.Once{}
}

func loadOnce() (*Config, error) {
	cfg := Defaults()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, fmt.Errorf("cannot determine home directory: %w", err)
	}

	globalPath := filepath.Join(home, ".config", "symfetch", "config.toml")
	if err := mergeFile(cfg, globalPath); err != nil {
		return cfg, fmt.Errorf("global config error: %w", err)
	}

	cwd, err := os.Getwd()
	if err == nil {
		projectPath := filepath.Join(cwd, ".symfetch.toml")
		if err := mergeFile(cfg, projectPath); err != nil {
			return cfg, fmt.Errorf("project config error: %w", err)
		}
	}

	// Environment variable overrides
	if v := os.Getenv("SYMFETCH_PROXY"); v != "" {
		cfg.HTTP.Proxy = v
	}
	if v := os.Getenv("SYMFETCH_PROFILE"); v != "" {
		cfg.HTTP.Profile = v
	}
	if v := os.Getenv("SYMFETCH_FORMAT"); v != "" {
		cfg.HTTP.DefaultFormat = v
	}

	return cfg, nil
}

// DefaultConfigTOML returns the default TOML content for config init.
func DefaultConfigTOML() string {
	return `# Symaira Fetch configuration
# See https://github.com/danieljustus/symaira-fetch

[http]
# Browser impersonation profile: chrome, firefox, honest
profile = "chrome"
# Default output format: markdown, json, text
default_format = "markdown"
# Request timeout in seconds
timeout_seconds = 30
# Maximum response body size in MB
max_body_mb = 10
# Default maximum characters in semantic output
max_chars = 20000
# Default concurrency for batch fetches
concurrency = 4
# Optional proxy URL (e.g. http://proxy:8080 or socks5://...)
# proxy = ""

[cache]
# Enable response caching
enabled = true
# Cache TTL (e.g. 15m, 1h, 24h)
ttl = "15m"
# Cache directory (default: ~/.cache/symfetch)
# dir = ""

[security]
# Allow fetching private/loopback addresses (dangerous, CLI-only recommended)
allow_private = false
`
}

func mergeFile(cfg *Config, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	var overlay Config
	if _, err := toml.DecodeFile(path, &overlay); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if overlay.HTTP.Proxy != "" {
		cfg.HTTP.Proxy = overlay.HTTP.Proxy
	}
	if overlay.HTTP.TimeoutSeconds != 0 {
		cfg.HTTP.TimeoutSeconds = overlay.HTTP.TimeoutSeconds
	}
	if overlay.HTTP.MaxBodyMB != 0 {
		cfg.HTTP.MaxBodyMB = overlay.HTTP.MaxBodyMB
	}
	if overlay.HTTP.Profile != "" {
		cfg.HTTP.Profile = overlay.HTTP.Profile
	}
	if overlay.HTTP.DefaultFormat != "" {
		cfg.HTTP.DefaultFormat = overlay.HTTP.DefaultFormat
	}
	if overlay.HTTP.MaxChars != 0 {
		cfg.HTTP.MaxChars = overlay.HTTP.MaxChars
	}
	if overlay.HTTP.Concurrency != 0 {
		cfg.HTTP.Concurrency = overlay.HTTP.Concurrency
	}
	if overlay.Cache.TTL != 0 {
		cfg.Cache.TTL = overlay.Cache.TTL
	}
	if overlay.Cache.Dir != "" {
		cfg.Cache.Dir = overlay.Cache.Dir
	}
	// Explicit false is still valid — only override if the toml key was present:
	// We use a simple approach: always apply security from overlay if file exists.
	cfg.Security.AllowPrivate = overlay.Security.AllowPrivate
	cfg.Cache.Enabled = overlay.Cache.Enabled

	return nil
}
