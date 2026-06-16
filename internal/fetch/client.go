package fetch

import (
	"context"
	"time"
)

// Profile controls TLS/HTTP fingerprint impersonation behavior.
type Profile string

const (
	ProfileChrome  Profile = "chrome"
	ProfileFirefox Profile = "firefox"
	ProfileHonest  Profile = "honest"
)

// ParseProfile parses a string into a Profile. Returns ProfileChrome as default.
func ParseProfile(s string) Profile {
	switch s {
	case "firefox":
		return ProfileFirefox
	case "honest":
		return ProfileHonest
	default:
		return ProfileChrome
	}
}

// Request describes a single HTTP fetch operation.
type Request struct {
	URL     string
	Method  string            // defaults to GET
	Headers map[string]string // additional/override headers
	Body    []byte

	Timeout      time.Duration // 0 = use client default (30s)
	Proxy        string        // http(s)://, socks5://; overrides client-level proxy
	Session      string        // named cookie jar; "" = ephemeral
	MaxBody      int64         // max body bytes; 0 = use client default (10 MiB)
	AllowPrivate bool          // allow RFC1918/loopback targets (SSRF override)
}

// Response is the result of a fetch.
type Response struct {
	FinalURL    string
	StatusCode  int
	Headers     map[string][]string
	Body        []byte // decoded (gzip/br/zstd), UTF-8 normalised
	Protocol    string // "HTTP/1.1", "HTTP/2.0", "HTTP/3.0"
	ContentType string
	Elapsed     time.Duration
	FromCache   bool
}

// Client is the core fetch abstraction. Implementations may use
// browser-impersonating transports (AzureTLS) or plain net/http (honest).
type Client interface {
	Fetch(ctx context.Context, req Request) (*Response, error)
	Close() error
}

// Option configures a Client at construction time.
type Option func(*clientOptions)

type clientOptions struct {
	proxy          string
	timeoutSeconds int
	maxBodyMB      int
	sessionsDir    string
	enableRetry    bool
	backoffConfig  BackoffConfig
	rateLimiter    *HostRateLimiter
}

// WithProxy sets a default proxy for all requests.
func WithProxy(proxy string) Option {
	return func(o *clientOptions) { o.proxy = proxy }
}

// WithTimeout sets the default request timeout in seconds.
func WithTimeout(seconds int) Option {
	return func(o *clientOptions) { o.timeoutSeconds = seconds }
}

// WithMaxBody sets the default maximum response body size in MB.
func WithMaxBody(mb int) Option {
	return func(o *clientOptions) { o.maxBodyMB = mb }
}

// WithSessionsDir sets the directory for persistent named cookie jars.
func WithSessionsDir(dir string) Option {
	return func(o *clientOptions) { o.sessionsDir = dir }
}

// WithRetry enables automatic retry with exponential backoff for transient errors.
func WithRetry(enable bool) Option {
	return func(o *clientOptions) { o.enableRetry = enable }
}

// WithBackoffConfig sets custom backoff configuration.
func WithBackoffConfig(config BackoffConfig) Option {
	return func(o *clientOptions) { o.backoffConfig = config }
}

// WithRateLimiter sets a per-host rate limiter with circuit breaker.
func WithRateLimiter(limiter *HostRateLimiter) Option {
	return func(o *clientOptions) { o.rateLimiter = limiter }
}

// New creates a Client for the given Profile.
func New(p Profile, opts ...Option) (Client, error) {
	o := &clientOptions{
		timeoutSeconds: 30,
		maxBodyMB:      10,
		backoffConfig:  DefaultBackoffConfig(),
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.enableRetry && o.rateLimiter == nil {
		o.rateLimiter = NewHostRateLimiter(DefaultCircuitBreakerConfig())
	}
	switch p {
	case ProfileHonest:
		return newHonestClient(o)
	default:
		return newAzureClient(p, o)
	}
}
