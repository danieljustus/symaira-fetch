package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func TestMakeFetchURLHandler_InvalidFormat(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	_, err := handler(context.Background(), json.RawMessage(`{"url":"https://example.com","format":"xml"}`))
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestMakeFetchURLHandler_CharLimit(t *testing.T) {
	handler := makeFetchURLHandler(&mockClient{}, pipeline.StaticEngine{})
	_, err := handler(context.Background(), json.RawMessage(`{"url":"https://example.com","char_limit":5000}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMakeFetchBatchHandler_InvalidFormat(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, nil)
	_, err := handler(context.Background(), json.RawMessage(`{"urls":["https://example.com"],"format":"xml"}`))
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestMakeFetchBatchHandler_CharLimit(t *testing.T) {
	handler := makeFetchBatchHandler(&mockClient{}, pipeline.StaticEngine{}, nil)
	_, err := handler(context.Background(), json.RawMessage(`{"urls":["https://example.com"],"char_limit":5000}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
