package cache

import (
	"encoding/json"
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
	c.Put("https://example.com", "chrome", "markdown", "", "", body, meta)

	c.evictIfOverSize()

	_, _, ok := c.Get("https://example.com", "chrome", "markdown", "", "")
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
		c.Put(url, "chrome", "markdown", "", "", body, meta)
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
		if err := c.Put(url, "chrome", "markdown", "", "", body, meta); err != nil {
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
	c.Put("https://example.com/valid", "chrome", "markdown", "", "", body, meta)

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
	c.Put("https://example.com/valid", "chrome", "markdown", "", "", body, meta)

	// Manually create a meta file without corresponding body
	key := c.key("https://example.com/orphan", "chrome", "markdown", "", "")
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

func TestReconcileIndex_CrossProcessStartup(t *testing.T) {
	dir := t.TempDir()

	// Simulate another process writing cache files directly, with no index.
	c1 := New(dir, 15*time.Minute)
	body := []byte("remote process body")
	meta := Meta{StatusCode: 200, StoredAt: time.Now()}
	metaData, _ := json.Marshal(meta)
	k := c1.key("https://example.com", "chrome", "markdown", "", "")
	subdir := filepath.Join(dir, k[:2])
	os.MkdirAll(subdir, 0700)
	os.WriteFile(filepath.Join(subdir, k+".body"), body, 0600)
	os.WriteFile(filepath.Join(subdir, k+".meta.json"), metaData, 0600)

	// Delete the index so the next New() must reconcile from disk.
	os.Remove(filepath.Join(dir, indexFileName))

	// Create a fresh cache process against the same directory.
	c2 := New(dir, 15*time.Minute)

	gotBody, _, ok := c2.Get("https://example.com", "chrome", "markdown", "", "")
	if !ok {
		t.Fatal("expected cache hit after cross-process reconcile")
	}
	if string(gotBody) != "remote process body" {
		t.Errorf("expected body %q, got %q", "remote process body", gotBody)
	}

	totalSize := c2.indexMgr.getTotalSize()
	if totalSize <= 0 {
		t.Errorf("expected positive total size after reconcile, got %d", totalSize)
	}
}

func TestReconcileIndex_StaleIndexPruned(t *testing.T) {
	dir := t.TempDir()

	// Write one valid entry.
	c1 := New(dir, 15*time.Minute)
	c1.Put("https://example.com/keep", "chrome", "markdown", "", "",
		[]byte("keep body"), Meta{StatusCode: 200})

	// Now corrupt the index: add a phantom entry that has no files on disk.
	phantomKey := "phantom-key-TEST-NOT-A-SECRET"
	c1.indexMgr.addEntry(phantomKey, 9999, time.Now())
	c1.indexMgr.save()

	// Start a new process. reconcileIndex should remove the phantom.
	c2 := New(dir, 15*time.Minute)

	totalSize := c2.indexMgr.getTotalSize()
	entries := c2.indexMgr.getEntries()
	for _, e := range entries {
		if e.Key == phantomKey {
			t.Error("phantom entry should have been pruned during reconcile")
		}
	}

	// Only the real entry should remain.
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after reconcile, got %d", len(entries))
	}

	gotBody, _, ok := c2.Get("https://example.com/keep", "chrome", "markdown", "", "")
	if !ok || string(gotBody) != "keep body" {
		t.Errorf("expected valid entry to survive reconcile, got %q (ok=%v)", gotBody, ok)
	}

	if totalSize <= 0 {
		t.Errorf("expected positive total size, got %d", totalSize)
	}
}

func TestAddEntry_UpsertNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)

	c.Put("https://example.com/page", "chrome", "markdown", "", "",
		[]byte("first version"), Meta{StatusCode: 200})

	entriesAfterFirst := c.indexMgr.getEntries()

	c.Put("https://example.com/page", "chrome", "markdown", "", "",
		[]byte("second version, longer body"), Meta{StatusCode: 200})

	entriesAfterSecond := c.indexMgr.getEntries()

	if len(entriesAfterSecond) != len(entriesAfterFirst) {
		t.Errorf("upsert should not add duplicate: got %d entries, want %d",
			len(entriesAfterSecond), len(entriesAfterFirst))
	}

	k := c.key("https://example.com/page", "chrome", "markdown", "", "")
	count := 0
	for _, e := range entriesAfterSecond {
		if e.Key == k {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 index entry for key, got %d", count)
	}
}

func TestEvictIfOverSize_BelowWatermark(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)
	c.maxSize = 500

	for i := 0; i < 10; i++ {
		url := "https://example.com/page" + string(rune('A'+i))
		body := make([]byte, 100)
		meta := Meta{StatusCode: 200, StoredAt: time.Now().Add(-time.Duration(i) * time.Hour)}
		c.Put(url, "chrome", "markdown", "", "", body, meta)
	}

	c.lastEviction = time.Time{}
	c.evictIfOverSize()

	totalSize := c.indexMgr.getTotalSize()
	watermark := c.maxSize * 8 / 10
	if totalSize > watermark {
		t.Errorf("expected cache size %d <= watermark %d (80%% of %d)",
			totalSize, watermark, c.maxSize)
	}
}

func TestIndexPersistedAfterPut(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 15*time.Minute)

	c.Put("https://example.com", "chrome", "markdown", "", "",
		[]byte("persist-me"), Meta{StatusCode: 200})

	data, err := os.ReadFile(filepath.Join(dir, indexFileName))
	if err != nil {
		t.Fatalf("index file not persisted: %v", err)
	}

	var idx cacheIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("corrupt index file: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Errorf("expected 1 persisted entry, got %d", len(idx.Entries))
	}
	if idx.TotalSize <= 0 {
		t.Errorf("expected positive persisted total size, got %d", idx.TotalSize)
	}
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return []byte(`{"status_code":200}`), nil
}
