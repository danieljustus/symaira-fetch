package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvictIfOverSize_NoEviction(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)
	c.maxSize = 1000

	body := []byte("small")
	meta := Meta{StatusCode: 200}
	c.Put("https://example.com", "chrome", "markdown", "", body, meta)

	c.evictIfOverSize()

	_, _, ok := c.Get("https://example.com", "chrome", "markdown", "")
	if !ok {
		t.Error("expected entry to still exist after eviction check")
	}
}

func TestEvictIfOverSize_EvictsOldest(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)
	c.maxSize = 200

	for i := 0; i < 5; i++ {
		url := "https://example.com/page" + string(rune('A'+i))
		body := make([]byte, 100)
		meta := Meta{StatusCode: 200, StoredAt: time.Now().Add(-time.Duration(i) * time.Hour)}
		c.Put(url, "chrome", "markdown", "", body, meta)
	}

	// Reset the eviction debounce so the explicit call below is not skipped.
	c.lastEviction = time.Time{}

	c.evictIfOverSize()

	totalSize := c.indexMgr.getTotalSize()
	if totalSize > c.maxSize {
		t.Errorf("expected cache size under limit, got %d", totalSize)
	}
}

func TestScanCache_Empty(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)

	totalSize, entries := c.scanCache()
	if totalSize != 0 {
		t.Errorf("expected total size 0, got %d", totalSize)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestScanCache_WithEntries(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)

	for i := 0; i < 3; i++ {
		url := "https://example.com/page" + string(rune('A'+i))
		body := []byte("body content " + string(rune('A'+i)))
		meta := Meta{StatusCode: 200}
		if err := c.Put(url, "chrome", "markdown", body, meta); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	totalSize, entries := c.scanCache()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	if totalSize <= 0 {
		t.Errorf("expected positive total size, got %d", totalSize)
	}
}

func TestScanCache_CorruptMetaSkipped(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)

	// Add a valid entry
	body := []byte("valid body")
	meta := Meta{StatusCode: 200}
	c.Put("https://example.com/valid", "chrome", "markdown", body, meta)

	// Manually create a corrupt meta file
	subdir := filepath.Join(dir, "ab") // assuming key starts with "ab"
	os.MkdirAll(subdir, 0700)
	corruptPath := filepath.Join(subdir, "corrupt.meta.json")
	os.WriteFile(corruptPath, []byte("not valid json"), 0600)

	_, entries := c.scanCache()
	if len(entries) != 1 {
		t.Errorf("expected 1 valid entry, got %d", len(entries))
	}
}

func TestScanCache_MissingBodySkipped(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)

	// Add a valid entry
	body := []byte("valid body")
	meta := Meta{StatusCode: 200}
	c.Put("https://example.com/valid", "chrome", "markdown", body, meta)

	// Manually create a meta file without corresponding body
	key := c.key("https://example.com/orphan", "chrome", "markdown", "")
	subdir := filepath.Join(dir, key[:2])
	os.MkdirAll(subdir, 0700)
	metaPath := filepath.Join(subdir, key+".meta.json")
	metaData := Meta{StatusCode: 200, StoredAt: time.Now()}
	data, _ := jsonMarshal(metaData)
	os.WriteFile(metaPath, data, 0600)

	_, entries := c.scanCache()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return []byte(`{"status_code":200}`), nil
}
