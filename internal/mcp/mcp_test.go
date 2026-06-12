package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/mcp"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func writeFrame(buf *bytes.Buffer, obj interface{}) {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(buf, "Content-Length: %d\r\n\r\n%s", len(data), data)
}

func readFrames(t *testing.T, data string) []map[string]interface{} {
	t.Helper()
	var frames []map[string]interface{}
	rest := data
	for {
		idx := strings.Index(rest, "Content-Length:")
		if idx == -1 {
			break
		}
		rest = rest[idx:]
		headerEnd := strings.Index(rest, "\r\n\r\n")
		if headerEnd == -1 {
			break
		}
		header := rest[:headerEnd]
		parts := strings.SplitN(header, ":", 2)
		if len(parts) != 2 {
			break
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &n); err != nil {
			break
		}
		bodyStart := headerEnd + 4
		if bodyStart+n > len(rest) {
			break
		}
		body := rest[bodyStart : bodyStart+n]
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(body), &obj); err == nil {
			frames = append(frames, obj)
		}
		rest = rest[bodyStart+n:]
	}
	return frames
}

func runRPC(t *testing.T, requests []map[string]interface{}) []map[string]interface{} {
	t.Helper()

	var in bytes.Buffer
	for _, req := range requests {
		writeFrame(&in, req)
	}

	client, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	eng := pipeline.StaticEngine{}

	var out bytes.Buffer
	if err := mcp.ServeIO(context.Background(), &in, &out, client, eng); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	return readFrames(t, out.String())
}

func TestMCPInitialize(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{}},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}
	if result["protocolVersion"] == nil {
		t.Error("expected protocolVersion in initialize response")
	}
}

func TestMCPToolsList(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "tools/list"},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("expected non-empty tools list")
	}

	names := map[string]bool{}
	for _, tool := range tools {
		if m, ok := tool.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				names[name] = true
			}
		}
	}
	for _, want := range []string{"fetch_url", "fetch_batch"} {
		if !names[want] {
			t.Errorf("expected tool %q in tools/list", want)
		}
	}
}

func TestMCPFetchURLBlocksPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `<html><body><p>Hello</p></body></html>`)
	}))
	defer srv.Close()

	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_url",
				"arguments": map[string]interface{}{
					"url":    srv.URL,
					"format": "markdown",
				},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for private address fetch")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "blocked_private") {
		t.Errorf("expected blocked_private in error text, got: %s", text)
	}
}

func TestMCPStdoutPurity(t *testing.T) {
	var in bytes.Buffer
	writeFrame(&in, map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]interface{}{}})
	writeFrame(&in, map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})

	client, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var out bytes.Buffer
	if err := mcp.ServeIO(context.Background(), &in, &out, client, pipeline.StaticEngine{}); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	frames := readFrames(t, out.String())
	for i, obj := range frames {
		if obj["jsonrpc"] == nil {
			t.Errorf("frame %d missing jsonrpc field: %v", i, obj)
		}
	}
	if len(frames) < 2 {
		t.Fatalf("expected at least 2 response frames, got %d", len(frames))
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 99, "method": "nonexistent/method"},
	})
	if len(frames) == 0 {
		t.Fatal("expected error response")
	}
	resp := frames[0]
	if resp["error"] == nil {
		t.Error("expected error for unknown method")
	}
}

func TestMCPParseError(t *testing.T) {
	var in bytes.Buffer
	fmt.Fprintf(&in, "Content-Length: 1\r\n\r\n{")

	client, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var out bytes.Buffer
	mcp.ServeIO(context.Background(), &in, &out, client, pipeline.StaticEngine{})

	frames := readFrames(t, out.String())
	if len(frames) == 0 {
		t.Fatal("expected parse error response")
	}
	resp := frames[0]
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object: %v", resp)
	}
	if code, ok := errObj["code"].(float64); !ok || code != -32700 {
		t.Errorf("expected parse error code -32700, got %v", errObj["code"])
	}
}

func TestMCPFetchBatchReturnsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `<html><body><p>Content</p></body></html>`)
	}))
	defer srv.Close()

	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_batch",
				"arguments": map[string]interface{}{
					"urls":   []interface{}{srv.URL, srv.URL},
					"format": "text",
				},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	if resp["result"] == nil {
		t.Fatalf("expected result, got: %v", resp)
	}
	result := resp["result"].(map[string]interface{})
	content := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("expected content")
	}
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "[") && !strings.Contains(text, "blocked_private") {
		t.Errorf("unexpected batch output: %s", truncate(text, 200))
	}
}

func TestMCPFetchURLRejectsFileScheme(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_url",
				"arguments": map[string]interface{}{
					"url": "file:///etc/passwd",
				},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for file:// URL")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", text)
	}
}

func TestMCPFetchURLRejectsGopherScheme(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_url",
				"arguments": map[string]interface{}{
					"url": "gopher://example.com",
				},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for gopher:// URL")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", text)
	}
}

func TestMCPFetchBatchRejectsFileScheme(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_batch",
				"arguments": map[string]interface{}{
					"urls": []interface{}{"file:///etc/passwd", "https://example.com"},
				},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["isError"] != true {
		t.Error("expected isError=true for batch with file:// URL")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %s", text)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
