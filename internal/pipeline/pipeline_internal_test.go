package pipeline

import (
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
)

func TestRawHTMLFallback(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "empty",
			body: []byte{},
			want: "",
		},
		{
			name: "ascii",
			body: []byte("Hello World"),
			want: "Hello World",
		},
		{
			name: "html",
			body: []byte("<html><body>Content</body></html>"),
			want: "<html><body>Content</body></html>",
		},
		{
			name: "binary-like",
			body: []byte{0x00, 0x01, 0x02},
			want: "\x00\x01\x02",
		},
		{
			name: "unicode",
			body: []byte("Hello \u00e9\u00e8\u00ea"),
			want: "Hello \u00e9\u00e8\u00ea",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rawHTMLFallback(tt.body)
			if got != tt.want {
				t.Errorf("rawHTMLFallback() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIslandSummary(t *testing.T) {
	tests := []struct {
		name     string
		islands  []agentdom.DataIsland
		contains []string
		empty    bool
	}{
		{
			name:    "empty islands",
			islands: []agentdom.DataIsland{},
			empty:   true,
		},
		{
			name: "single island with object",
			islands: []agentdom.DataIsland{
				{
					Source: "__NEXT_DATA__",
					JSON:   []byte(`{"page":"home","props":{"id":1}}`),
				},
			},
			contains: []string{"__NEXT_DATA__", "keys="},
		},
		{
			name: "multiple islands",
			islands: []agentdom.DataIsland{
				{
					Source: "__NEXT_DATA__",
					JSON:   []byte(`{"page":"home"}`),
				},
				{
					Source: "application/ld+json",
					JSON:   []byte(`{"@type":"Article","headline":"Test"}`),
				},
			},
			contains: []string{"__NEXT_DATA__", "application/ld+json"},
		},
		{
			name: "island with array JSON",
			islands: []agentdom.DataIsland{
				{
					Source: "data-list",
					JSON:   []byte(`[1,2,3]`),
				},
			},
			contains: []string{"data-list", "raw JSON"},
		},
		{
			name: "island with malformed JSON",
			islands: []agentdom.DataIsland{
				{
					Source: "bad-json",
					JSON:   []byte(`not valid json`),
				},
			},
			contains: []string{"bad-json", "raw JSON"},
		},
		{
			name: "island with primitive JSON",
			islands: []agentdom.DataIsland{
				{
					Source: "string-value",
					JSON:   []byte(`"just a string"`),
				},
			},
			contains: []string{"string-value", "raw JSON"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IslandSummary(tt.islands)
			if tt.empty {
				if got != "" {
					t.Errorf("IslandSummary() = %q, want empty", got)
				}
				return
			}
			for _, s := range tt.contains {
				if !containsString(got, s) {
					t.Errorf("IslandSummary() = %q, should contain %q", got, s)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
