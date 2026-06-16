package fetch

import (
	"math"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BackoffConfig configures exponential backoff behavior.
type BackoffConfig struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxRetries   int
}

// DefaultBackoffConfig returns sensible defaults for HTTP retry backoff.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		MaxRetries:   3,
	}
}

// BackoffDelay calculates the delay for a given retry attempt with jitter.
func (c BackoffConfig) BackoffDelay(attempt int) time.Duration {
	delay := float64(c.InitialDelay) * math.Pow(c.Multiplier, float64(attempt))
	if delay > float64(c.MaxDelay) {
		delay = float64(c.MaxDelay)
	}
	// Add jitter: 50% to 100% of calculated delay
	jitter := delay * (0.5 + rand.Float64()*0.5)
	return time.Duration(jitter)
}

// ParseRetryAfter parses the Retry-After header value (seconds or HTTP-date).
func ParseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	// Try parsing as integer seconds
	if secs, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
		return time.Duration(secs) * time.Second
	}
	// Try parsing as HTTP-date (RFC 7231)
	if t, err := time.Parse(time.RFC1123, val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing, reject requests
	CircuitHalfOpen                     // Testing if service recovered
)

// CircuitBreaker implements a per-host circuit breaker pattern.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	consecutiveFails int
	lastFailure      time.Time
	openedAt         time.Time
	config           CircuitBreakerConfig
}

// CircuitBreakerConfig configures circuit breaker behavior.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Consecutive failures to open circuit
	RecoveryTimeout  time.Duration // Time before half-open
	SuccessThreshold int           // Successes in half-open to close circuit
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		RecoveryTimeout:  60 * time.Second,
		SuccessThreshold: 2,
	}
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  CircuitClosed,
		config: config,
	}
}

// Allow checks if a request should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.openedAt) > cb.config.RecoveryTimeout {
			cb.state = CircuitHalfOpen
			cb.consecutiveFails = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.consecutiveFails++
		if cb.consecutiveFails >= cb.config.SuccessThreshold {
			cb.state = CircuitClosed
			cb.consecutiveFails = 0
		}
	} else {
		cb.consecutiveFails = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		cb.openedAt = time.Now()
	} else if cb.consecutiveFails >= cb.config.FailureThreshold {
		cb.state = CircuitOpen
		cb.openedAt = time.Now()
	}
}

// HostRateLimiter tracks rate limiting state per host.
type HostRateLimiter struct {
	mu         sync.Mutex
	breakers   map[string]*CircuitBreaker
	config     CircuitBreakerConfig
}

// NewHostRateLimiter creates a new per-host rate limiter.
func NewHostRateLimiter(config CircuitBreakerConfig) *HostRateLimiter {
	return &HostRateLimiter{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// extractHost extracts the host from a URL string.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Hostname()
}

// Allow checks if a request to the given URL should be allowed.
func (hrl *HostRateLimiter) Allow(rawURL string) bool {
	host := extractHost(rawURL)
	hrl.mu.Lock()
	breaker, ok := hrl.breakers[host]
	if !ok {
		breaker = NewCircuitBreaker(hrl.config)
		hrl.breakers[host] = breaker
	}
	hrl.mu.Unlock()
	return breaker.Allow()
}

// RecordSuccess records a successful request to the given URL.
func (hrl *HostRateLimiter) RecordSuccess(rawURL string) {
	host := extractHost(rawURL)
	hrl.mu.Lock()
	if breaker, ok := hrl.breakers[host]; ok {
		breaker.RecordSuccess()
	}
	hrl.mu.Unlock()
}

// RecordFailure records a failed request to the given URL.
func (hrl *HostRateLimiter) RecordFailure(rawURL string) {
	host := extractHost(rawURL)
	hrl.mu.Lock()
	if breaker, ok := hrl.breakers[host]; ok {
		breaker.RecordFailure()
	}
	hrl.mu.Unlock()
}

// IsTransientError checks if an error is transient and should be retried.
func IsTransientError(statusCode int, err error) bool {
	if err != nil {
		// Network errors are generally transient
		return true
	}
	switch statusCode {
	case 429, 502, 503, 504:
		return true
	}
	return false
}
