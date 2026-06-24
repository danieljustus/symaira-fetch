package mcp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/mcp"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func TestStartServerInvalidProxy(t *testing.T) {
	err := mcp.StartServer(fetch.ProfileHonest, "://invalid")
	if err == nil {
		t.Fatal("expected error for invalid proxy")
	}
	if !strings.Contains(err.Error(), "init fetch client") {
		t.Errorf("expected 'init fetch client' in error, got: %s", err.Error())
	}
}

func TestMCPToolsListSchema(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{"jsonrpc": "2.0", "id": 1, "method": "tools/list"},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	result := frames[0]["result"].(map[string]interface{})
	tools := result["tools"].([]interface{})

	for _, tool := range tools {
		m := tool.(map[string]interface{})
		name := m["name"].(string)
		schema := m["inputSchema"].(map[string]interface{})

		if schema["type"] != "object" {
			t.Errorf("tool %s: schema type should be 'object', got %v", name, schema["type"])
		}
		props, ok := schema["properties"].(map[string]interface{})
		if !ok || len(props) == 0 {
			t.Errorf("tool %s: should have properties", name)
		}
	}
}

func TestMCPFetchURLRequiresURL(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      "fetch_url",
				"arguments": map[string]interface{}{},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	if resp["error"] == nil && resp["result"].(map[string]interface{})["isError"] != true {
		t.Error("expected error for missing URL")
	}
}

func TestMCPFetchBatchRequiresURLs(t *testing.T) {
	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      "fetch_batch",
				"arguments": map[string]interface{}{},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	if resp["error"] == nil && resp["result"].(map[string]interface{})["isError"] != true {
		t.Error("expected error for missing URLs")
	}
}

func TestMCPFetchBatchMax20URLs(t *testing.T) {
	urls := make([]interface{}, 21)
	for i := range urls {
		urls[i] = "https://example.com"
	}

	frames := runRPC(t, []map[string]interface{}{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "fetch_batch",
				"arguments": map[string]interface{}{
					"urls": urls,
				},
			},
		},
	})
	if len(frames) == 0 {
		t.Fatal("expected response")
	}
	resp := frames[0]
	result := resp["result"].(map[string]interface{})
	if result["isError"] != true {
		t.Error("expected error for >20 URLs")
	}
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, "maximum 20") {
		t.Errorf("expected max 20 error, got: %s", text)
	}
}

func TestMCPNotificationNoResponse(t *testing.T) {
	var in bytes.Buffer
	writeFrame(&in, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})

	client, err := fetch.New(fetch.ProfileHonest)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var out bytes.Buffer
	if err := mcp.ServeIO(context.Background(), &in, &out, client, pipeline.StaticEngine{}); err != nil {
		t.Fatalf("ServeIO error: %v", err)
	}

	if out.Len() > 0 {
		t.Errorf("expected no response for notification, got: %s", out.String())
	}
}
