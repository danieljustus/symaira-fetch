// Package robots implements robots.txt parsing and URL allowance checking
// per the Robots Exclusion Protocol (RFC 9309).
package robots

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultTTL is the cache lifetime for a domain's robots.txt rules.
const DefaultTTL = 1 * time.Hour

type rule struct {
	path    string
	allowed bool
}

type group struct {
	agents []string
	rules  []rule
}

type cacheEntry struct {
	groups    []group
	expiresAt time.Time
}

// Checker fetches and caches robots.txt rules per domain, then checks
// whether a given URL is allowed for a specific user-agent.
type Checker struct {
	mu     sync.RWMutex
	cache  map[string]*cacheEntry
	ttl    time.Duration
	client *http.Client
}

func NewChecker() *Checker {
	return &Checker{
		cache: make(map[string]*cacheEntry),
		ttl:   DefaultTTL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Check returns true if the given URL is allowed for userAgent according
// to the site's robots.txt. If robots.txt cannot be fetched or parsed,
// the URL is allowed (fail-open for polite crawling).
func (c *Checker) Check(ctx context.Context, userAgent, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true, fmt.Errorf("robots: parse url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return true, nil
	}

	domain := u.Scheme + "://" + u.Host
	groups, err := c.groupsForDomain(ctx, domain)
	if err != nil {
		return true, nil
	}

	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	return isAllowed(groups, userAgent, path), nil
}

func (c *Checker) groupsForDomain(ctx context.Context, domain string) ([]group, error) {
	c.mu.RLock()
	entry, ok := c.cache[domain]
	c.mu.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.groups, nil
	}

	robotsURL := domain + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.mu.Lock()
		c.cache[domain] = &cacheEntry{
			groups:    nil,
			expiresAt: time.Now().Add(c.ttl),
		}
		c.mu.Unlock()
		return nil, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	groups := parse(string(body))

	c.mu.Lock()
	c.cache[domain] = &cacheEntry{
		groups:    groups,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	return groups, nil
}

func parse(content string) []group {
	var groups []group
	var current *group

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		colonIdx := strings.IndexByte(line, ':')
		if colonIdx < 0 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(line[:colonIdx]))
		value := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "user-agent":
			if current == nil || len(current.rules) > 0 {
				g := group{}
				groups = append(groups, g)
				current = &groups[len(groups)-1]
			}
			current.agents = append(current.agents, strings.ToLower(value))

		case "disallow":
			if current != nil && value != "" {
				current.rules = append(current.rules, rule{path: value, allowed: false})
			}

		case "allow":
			if current != nil && value != "" {
				current.rules = append(current.rules, rule{path: value, allowed: true})
			}
		}
	}

	return groups
}

func isAllowed(groups []group, userAgent, path string) bool {
	ua := strings.ToLower(userAgent)

	var bestGroup *group
	bestSpecificity := -1

	for i := range groups {
		g := &groups[i]
		for _, agent := range g.agents {
			if agent == "*" {
				if bestSpecificity < 0 {
					bestGroup = g
					bestSpecificity = 0
				}
			} else if strings.Contains(ua, agent) {
				bestGroup = g
				bestSpecificity = 1
				break
			}
		}
		if bestSpecificity == 1 {
			break
		}
	}

	if bestGroup == nil {
		for i := range groups {
			g := &groups[i]
			for _, agent := range g.agents {
				if agent == "*" {
					bestGroup = g
					break
				}
			}
			if bestGroup != nil {
				break
			}
		}
	}

	if bestGroup == nil {
		return true
	}

	bestLen := -1
	allowed := true

	for _, r := range bestGroup.rules {
		if matchPath(r.path, path) {
			ruleLen := len(r.path)
			if ruleLen > bestLen {
				bestLen = ruleLen
				allowed = r.allowed
			} else if ruleLen == bestLen {
				if r.allowed {
					allowed = true
				}
			}
		}
	}

	return allowed
}

func matchPath(pattern, path string) bool {
	if pattern == "" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, pattern[:len(pattern)-1])
	}
	return strings.HasPrefix(path, pattern)
}
