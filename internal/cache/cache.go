package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const defaultMaxSize = 100 * 1024 * 1024

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
	dir         string
	ttl         time.Duration
	maxSize     int64
	mu          sync.RWMutex
	currentSize int64
	lastScan    time.Time
}

// New creates a Cache rooted at dir with the given TTL.
// It ensures the directory exists with 0700 permissions and fixes
// overly permissive directories on shared systems.
func New(dir string, ttl time.Duration) *Cache {
	ensureCacheDir(dir)
	return &Cache{dir: dir, ttl: ttl, maxSize: defaultMaxSize}
}

// ensureCacheDir creates the cache directory with 0700 permissions
// and tightens existing directories that are overly permissive.
func ensureCacheDir(dir string) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	info, err := os.Stat(dir)
	if err != nil {
		return
	}
	if info.Mode().Perm() != 0700 {
		os.Chmod(dir, 0700)
	}
}

// DefaultDir returns ~/.cache/symfetch.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "symfetch")
	}
	return filepath.Join(home, ".cache", "symfetch")
}

func (c *Cache) key(url, profile, format string) string {
	h := sha256.New()
	h.Write([]byte(url))
	h.Write([]byte("|"))
	h.Write([]byte(profile))
	h.Write([]byte("|"))
	h.Write([]byte(format))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (c *Cache) bodyPath(k string) string {
	return filepath.Join(c.dir, k[:2], k+".body")
}

func (c *Cache) metaPath(k string) string {
	return filepath.Join(c.dir, k[:2], k+".meta.json")
}

// Get returns cached body+meta if present and not expired.
func (c *Cache) Get(url, profile, format string) ([]byte, *Meta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	k := c.key(url, profile, format)
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
func (c *Cache) Put(url, profile, format string, body []byte, meta Meta) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := c.key(url, profile, format)
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

	bodyTmp := c.bodyPath(k) + ".tmp"
	if err := os.WriteFile(bodyTmp, body, 0600); err != nil {
		return err
	}
	if err := os.Rename(bodyTmp, c.bodyPath(k)); err != nil {
		return err
	}

	metaTmp := c.metaPath(k) + ".tmp"
	if err := os.WriteFile(metaTmp, metaData, 0600); err != nil {
		return err
	}
	if err := os.Rename(metaTmp, c.metaPath(k)); err != nil {
		return err
	}

	entrySize := int64(len(body)) + int64(len(metaData))
	c.currentSize += entrySize
	c.evictIfOverSize()
	return nil
}

type cacheEntryInfo struct {
	key      string
	storedAt time.Time
	size     int64
}

func (c *Cache) evictIfOverSize() {
	if time.Since(c.lastScan) < time.Hour && c.currentSize <= c.maxSize {
		return
	}

	totalSize, entries := c.scanCache()
	c.currentSize = totalSize
	c.lastScan = time.Now()

	if totalSize <= c.maxSize {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].storedAt.Before(entries[j].storedAt)
	})

	for _, entry := range entries {
		if totalSize <= c.maxSize*8/10 {
			break
		}
		bodyPath := c.bodyPath(entry.key)
		metaPath := c.metaPath(entry.key)
		os.Remove(bodyPath)
		os.Remove(metaPath)
		totalSize -= entry.size
		c.currentSize -= entry.size
		slog.Debug("evicted cache entry", "key", entry.key)
	}
}

func (c *Cache) scanCache() (int64, []cacheEntryInfo) {
	var totalSize int64
	var entries []cacheEntryInfo

	filepath.WalkDir(c.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var m Meta
		if err := json.Unmarshal(data, &m); err != nil {
			return nil
		}

		key := filepath.Base(path)
		key = key[:len(key)-len(".meta.json")]

		info, err := d.Info()
		if err != nil {
			return nil
		}
		metaSize := info.Size()

		bodyPath := c.bodyPath(key)
		bodyInfo, err := os.Stat(bodyPath)
		bodySize := int64(0)
		if err == nil {
			bodySize = bodyInfo.Size()
		}

		totalSize += metaSize + bodySize
		entries = append(entries, cacheEntryInfo{
			key:      key,
			storedAt: m.StoredAt,
			size:     metaSize + bodySize,
		})
		return nil
	})

	return totalSize, entries
}
