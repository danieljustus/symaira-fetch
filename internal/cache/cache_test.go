package cache_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/cache"
)

func newTempCache(t *testing.T, ttl time.Duration) *cache.Cache {
	t.Helper()
	dir := t.TempDir()
	return cache.New(dir, ttl)
}

func TestCacheGetMiss(t *testing.T) {
	c := newTempCache(t, 15*time.Minute)
	_, _, ok := c.Get("https://example.com", "chrome")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCachePutAndGet(t *testing.T) {
	c := newTempCache(t, 15*time.Minute)
	body := []byte("hello world")
	meta := cache.Meta{StatusCode: 200, FinalURL: "https://example.com", ContentType: "text/html"}

	if err := c.Put("https://example.com", "chrome", body, meta); err != nil {
		t.Fatal(err)
	}

	gotBody, gotMeta, ok := c.Get("https://example.com", "chrome")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(gotBody) != "hello world" {
		t.Errorf("expected body %q, got %q", "hello world", gotBody)
	}
	if gotMeta.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", gotMeta.StatusCode)
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	ttl := 50 * time.Millisecond
	c := newTempCache(t, ttl)

	if err := c.Put("https://example.com", "chrome", []byte("data"), cache.Meta{StatusCode: 200}); err != nil {
		t.Fatal(err)
	}

	if _, _, ok := c.Get("https://example.com", "chrome"); !ok {
		t.Fatal("expected immediate hit")
	}

	time.Sleep(ttl + 20*time.Millisecond)

	if _, _, ok := c.Get("https://example.com", "chrome"); ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestCacheDifferentProfilesDontCollide(t *testing.T) {
	c := newTempCache(t, 15*time.Minute)
	c.Put("https://example.com", "chrome", []byte("chrome-body"), cache.Meta{StatusCode: 200})
	c.Put("https://example.com", "firefox", []byte("firefox-body"), cache.Meta{StatusCode: 200})

	b1, _, ok1 := c.Get("https://example.com", "chrome")
	b2, _, ok2 := c.Get("https://example.com", "firefox")

	if !ok1 || !ok2 {
		t.Fatal("expected both hits")
	}
	if string(b1) != "chrome-body" || string(b2) != "firefox-body" {
		t.Errorf("cache collision: %q %q", b1, b2)
	}
}

func TestCacheAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	c := cache.New(dir, 15*time.Minute)
	c.Put("https://example.com", "chrome", []byte("data"), cache.Meta{StatusCode: 200})

	// No .tmp files should remain after a successful Put
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			name := d.Name()
			if len(name) > 4 && name[len(name)-4:] == ".tmp" {
				t.Errorf("stale .tmp file found: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
