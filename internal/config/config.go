package config

import (
	"time"

	"github.com/danieljustus/symaira-corekit/configkit"
)

type Config struct {
	HTTP     HTTPConfig     `json:"http" toml:"http"`
	Cache    CacheConfig    `json:"cache" toml:"cache"`
	Security SecurityConfig `json:"security" toml:"security"`
}

type HTTPConfig struct {
	Proxy          string        `json:"proxy" toml:"proxy"`
	TimeoutSeconds int           `json:"timeout_seconds" toml:"timeout_seconds"`
	MaxBodyMB      int           `json:"max_body_mb" toml:"max_body_mb"`
	Profile        string        `json:"profile" toml:"profile"`
	DefaultFormat  string        `json:"default_format" toml:"default_format"`
	MaxChars       int           `json:"max_chars" toml:"max_chars"`
	Concurrency    int           `json:"concurrency" toml:"concurrency"`
}

type CacheConfig struct {
	Enabled bool          `json:"enabled" toml:"enabled"`
	TTL     time.Duration `json:"ttl" toml:"ttl"`
	Dir     string        `json:"dir" toml:"dir"`
}

type SecurityConfig struct {
	AllowPrivate bool `json:"allow_private" toml:"allow_private"`
}

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

var loader = configkit.NewLoader[Config](configkit.Options{
	AppName:  "symfetch",
	EnvPrefix: "SYMFETCH",
}, Defaults)

func Load() (*Config, error) {
	return loader.Load()
}

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
