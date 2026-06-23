package agentdom

import (
	"encoding/json"

	"github.com/danieljustus/symaira-fetch/internal/semantic"
)

// Element is a semantic DOM element, optionally tagged with an agent ID.
type Element struct {
	AgentID  string            `json:"id,omitempty"` // "@e1" for interactive elements
	Category semantic.Category `json:"category"`
	Tag      string            `json:"tag,omitempty"`
	Text     string            `json:"text,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Children []Element         `json:"children,omitempty"`
}

// DataIsland proxies semantic.DataIsland.
type DataIsland struct {
	Source string          `json:"source"`
	JSON   json.RawMessage `json:"json"`
}

// Document is the agent-optimised view of a fetched page.
type Document struct {
	URL         string       `json:"url"`
	FinalURL    string       `json:"final_url,omitempty"`
	Title       string       `json:"title,omitempty"`
	Lang        string       `json:"lang,omitempty"`
	Content     []Element    `json:"content"`     // scored main content, document order
	Interactive []Element    `json:"interactive"` // flat list of @eN-tagged elements
	Islands     []DataIsland `json:"islands,omitempty"`
}

// Meta holds response metadata returned alongside the rendered output.
type Meta struct {
	FinalURL   string `json:"final_url"`
	StatusCode int    `json:"status_code"`
	Title      string `json:"title,omitempty"`
	Lang       string `json:"lang,omitempty"`
	CharCount  int    `json:"char_count"`
	EstTokens  int    `json:"est_tokens"` // chars / 4
	Truncated  bool   `json:"truncated"`
	Protocol   string `json:"protocol,omitempty"`
}
