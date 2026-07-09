package mcp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/mcp"
)

func TestMCPWaybackSnapshots(t *testing.T) {
	// Start mock CDX server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		targetURL := q.Get("url")
		if targetURL == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if targetURL == "https://empty.com" {
			// Return empty response (only header row)
			json.NewEncoder(w).Encode([][]string{
				{"timestamp", "original", "mimetype", "statuscode", "digest", "length"},
			})
			return
		}

		// Return sample CDX rows
		data := [][]string{
			{"timestamp", "original", "mimetype", "statuscode", "digest", "length"},
			{"20240101120000", targetURL, "text/html", "200", "digest1", "1024"},
		}
		json.NewEncoder(w).Encode(data)
	}))
	defer server.Close()

	// Override CdxBaseURL
	oldURL := mcp.CdxBaseURL
	mcp.CdxBaseURL = server.URL
	defer func() { mcp.CdxBaseURL = oldURL }()

	t.Run("basic lookup", func(t *testing.T) {
		frames := runRPC(t, []map[string]interface{}{
			{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "wayback_snapshots",
					"arguments": map[string]interface{}{
						"url": "https://example.com",
					},
				},
			},
		})
		if len(frames) == 0 {
			t.Fatal("expected response")
		}
		res := frames[0]["result"].(map[string]interface{})
		if res["isError"] == true {
			t.Fatalf("unexpected error response: %v", res)
		}
		content := res["content"].([]interface{})[0].(map[string]interface{})["text"].(string)

		if !strings.Contains(content, "20240101120000") {
			t.Errorf("expected timestamp in output, got: %s", content)
		}
	})

	t.Run("empty lookup", func(t *testing.T) {
		frames := runRPC(t, []map[string]interface{}{
			{
				"jsonrpc": "2.0",
				"id":      2,
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "wayback_snapshots",
					"arguments": map[string]interface{}{
						"url": "https://empty.com",
					},
				},
			},
		})
		if len(frames) == 0 {
			t.Fatal("expected response")
		}
		res := frames[0]["result"].(map[string]interface{})
		content := res["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
		if !strings.Contains(content, "No snapshots found") {
			t.Errorf("expected 'No snapshots found', got: %s", content)
		}
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		frames := runRPC(t, []map[string]interface{}{
			{
				"jsonrpc": "2.0",
				"id":      3,
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "wayback_snapshots",
					"arguments": map[string]interface{}{
						"url": "ftp://example.com",
					},
				},
			},
		})
		if len(frames) == 0 {
			t.Fatal("expected response")
		}
		res := frames[0]["result"].(map[string]interface{})
		if res["isError"] != true {
			t.Error("expected error for ftp scheme")
		}
	})
}
