package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Meta is stored alongside each cached body.
type Meta struct {
	URL         string            `json:"url"`
	FinalURL    string            `json:"final_url"`
	StatusCode  int               `json:"status_code"`
	ContentType string            `json:"content_type"`
	Protocol    string            `json:"protocol"`
	Headers     map[string][]string `json:"headers"`
	StoredAt    time.Time         `json:"stored_at"`
	TTL         time.Duration     `json:"ttl"`
}

// Cache is a flat-file, content-addressed response cache.
type Cache struct {
	dir string
	ttl time.Duration
}

// New creates a Cache rooted at dir with the given TTL.
func New(dir string, ttl time.Duration) *Cache {
	return &Cache{dir: dir, ttl: ttl}
}

// DefaultDir returns ~/.cache/symfetch.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "symfetch")
	}
	return filepath.Join(home, ".cache", "symfetch")
}

func (c *Cache) key(url, profile string) string {
	h := sha256.New()
	h.Write([]byte(url))
	h.Write([]byte("|"))
	h.Write([]byte(profile))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (c *Cache) bodyPath(k string) string {
	return filepath.Join(c.dir, k[:2], k+".body")
}

func (c *Cache) metaPath(k string) string {
	return filepath.Join(c.dir, k[:2], k+".meta.json")
}

// Get returns cached body+meta if present and not expired.
func (c *Cache) Get(url, profile string) ([]byte, *Meta, bool) {
	k := c.key(url, profile)
	metaData, err := os.ReadFile(c.metaPath(k))
	if err != nil {
		return nil, nil, false
	}
	var m Meta
	if err := json.Unmarshal(metaData, &m); err != nil {
		return nil, nil, false
	}
	ttl := m.TTL
	if ttl <= 0 {
		ttl = c.ttl
	}
	if time.Since(m.StoredAt) > ttl {
		return nil, nil, false
	}
	body, err := os.ReadFile(c.bodyPath(k))
	if err != nil {
		return nil, nil, false
	}
	return body, &m, true
}

// Put stores the body and meta in the cache.
func (c *Cache) Put(url, profile string, body []byte, meta Meta) error {
	k := c.key(url, profile)
	dir := filepath.Join(c.dir, k[:2])
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	meta.StoredAt = time.Now()
	meta.TTL = c.ttl

	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	// Write body
	bodyTmp := c.bodyPath(k) + ".tmp"
	if err := os.WriteFile(bodyTmp, body, 0600); err != nil {
		return err
	}
	if err := os.Rename(bodyTmp, c.bodyPath(k)); err != nil {
		return err
	}

	// Write meta
	metaTmp := c.metaPath(k) + ".tmp"
	if err := os.WriteFile(metaTmp, metaData, 0600); err != nil {
		return err
	}
	return os.Rename(metaTmp, c.metaPath(k))
}
