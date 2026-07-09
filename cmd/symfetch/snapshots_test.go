package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSnapshotsCommand(t *testing.T) {
	// Start a mock server for CDX lookup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "cdx/search/cdx") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		
		q := r.URL.Query()
		targetURL := q.Get("url")
		if targetURL == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		
		// Return sample CDX rows (header row first)
		data := [][]string{
			{"timestamp", "original", "mimetype", "statuscode", "digest", "length"},
			{"20240101120000", targetURL, "text/html", "200", "digest1", "1024"},
			{"20240102120000", targetURL, "text/html", "404", "digest2", "512"},
		}
		json.NewEncoder(w).Encode(data)
	}))
	defer server.Close()

	// Override cdxBaseURL with mock server endpoint
	oldURL := cdxBaseURL
	cdxBaseURL = server.URL + "/cdx/search/cdx"
	defer func() { cdxBaseURL = oldURL }()

	t.Run("table output", func(t *testing.T) {
		stdout, stderr, err := executeCmd(t, "snapshots", "https://example.com")
		if err != nil {
			t.Fatalf("execute failed: %v, stderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "TIMESTAMP") {
			t.Errorf("expected header row, got stdout: %s", stdout)
		}
		if !strings.Contains(stdout, "20240101120000") || !strings.Contains(stdout, "20240102120000") {
			t.Errorf("expected timestamps in output, got: %s", stdout)
		}
		if !strings.Contains(stdout, "2 snapshot(s) found.") {
			t.Errorf("expected snapshot count footer, got: %s", stdout)
		}
	})

	t.Run("json output", func(t *testing.T) {
		stdout, stderr, err := executeCmd(t, "snapshots", "https://example.com", "--json")
		if err != nil {
			t.Fatalf("execute failed: %v, stderr: %s", err, stderr)
		}
		
		var results []map[string]string
		if err := json.Unmarshal([]byte(stdout), &results); err != nil {
			t.Fatalf("failed to decode JSON output: %v, stdout: %s", err, stdout)
		}
		
		if len(results) != 2 {
			t.Errorf("expected 2 snapshots, got %d", len(results))
		}
		if results[0]["timestamp"] != "20240101120000" || results[0]["status"] != "200" {
			t.Errorf("unexpected first snapshot content: %v", results[0])
		}
	})

	t.Run("invalid url scheme", func(t *testing.T) {
		_, _, err := executeCmd(t, "snapshots", "ftp://example.com")
		if err == nil {
			t.Error("expected error for unsupported scheme")
		} else if !strings.Contains(err.Error(), "unsupported scheme") {
			t.Errorf("expected 'unsupported scheme' error, got: %v", err)
		}
	})
}
