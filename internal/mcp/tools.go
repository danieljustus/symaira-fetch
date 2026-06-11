package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/danieljustus/symaira-fetch/internal/batch"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func toolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "fetch_url",
			"description": "Fetch a web page and return LLM-optimized content. Uses browser-impersonating TLS to bypass basic bot detection. Returns Markdown by default.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to fetch (must be http or https)",
					},
					"format": map[string]interface{}{
						"type":        "string",
						"description": "Output format: markdown (default), json, text",
						"enum":        []string{"markdown", "json", "text"},
					},
					"max_chars": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum characters in output (default 20000)",
					},
					"include_links": map[string]interface{}{
						"type":        "boolean",
						"description": "Append a Links section with all hrefs (default false)",
					},
					"raw": map[string]interface{}{
						"type":        "boolean",
						"description": "Return raw decoded response body without semantic processing",
					},
					"timeout_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Request timeout in seconds (default 30)",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "fetch_batch",
			"description": "Fetch multiple URLs concurrently and return results in input order. Each URL is processed independently; one failure does not abort the batch.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"urls": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "URLs to fetch (max 20)",
					},
					"format": map[string]interface{}{
						"type":        "string",
						"description": "Output format for each result: markdown, json, text",
					},
					"max_chars": map[string]interface{}{
						"type":        "integer",
						"description": "Per-page character budget (default 20000)",
					},
					"concurrency": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum parallel fetches (default 4, max 8)",
					},
				},
				"required": []string{"urls"},
			},
		},
	}
}

func handleToolCall(reqID interface{}, name string, args map[string]interface{}, client fetch.Client, eng pipeline.Engine) {
	switch name {
	case "fetch_url":
		handleFetchURL(reqID, args, client, eng)
	case "fetch_batch":
		handleFetchBatch(reqID, args, client, eng)
	default:
		sendError(reqID, -32601, "Unknown tool: "+name)
	}
}

func handleFetchURL(reqID interface{}, args map[string]interface{}, client fetch.Client, eng pipeline.Engine) {
	rawURL, ok := args["url"].(string)
	if !ok || rawURL == "" {
		sendToolError(reqID, "error: missing required argument 'url'")
		return
	}

	format := pipeline.FormatMarkdown
	if f, ok := args["format"].(string); ok && f != "" {
		format = pipeline.ParseFormat(f)
	}

	maxChars := 20000
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
	}

	includeLinks, _ := args["include_links"].(bool)
	raw, _ := args["raw"].(bool)

	timeoutSec := 30
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSec = int(v)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if raw {
		resp, err := client.Fetch(ctx, fetch.Request{URL: rawURL, AllowPrivate: false})
		if err != nil {
			sendToolError(reqID, categoriseError(err))
			return
		}
		sendToolResponse(reqID, string(resp.Body))
		return
	}

	res, err := pipeline.Run(ctx, client, eng, rawURL, pipeline.Options{
		Format:       format,
		MaxChars:     maxChars,
		IncludeLinks: includeLinks,
	})
	if err != nil {
		sendToolError(reqID, categoriseError(err))
		return
	}

	output := formatWithMeta(res, format)
	sendToolResponse(reqID, output)
}

func handleFetchBatch(reqID interface{}, args map[string]interface{}, client fetch.Client, eng pipeline.Engine) {
	rawURLs, ok := args["urls"].([]interface{})
	if !ok || len(rawURLs) == 0 {
		sendToolError(reqID, "error: missing required argument 'urls'")
		return
	}
	if len(rawURLs) > 20 {
		sendToolError(reqID, "error: maximum 20 URLs per batch")
		return
	}

	items := make([]batch.Item, 0, len(rawURLs))
	for _, u := range rawURLs {
		if s, ok := u.(string); ok && s != "" {
			items = append(items, batch.Item{URL: s})
		}
	}

	format := pipeline.FormatMarkdown
	if f, ok := args["format"].(string); ok && f != "" {
		format = pipeline.ParseFormat(f)
	}

	maxChars := 20000
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
	}

	concurrency := 4
	if v, ok := args["concurrency"].(float64); ok && v > 0 {
		c := int(v)
		if c > 8 {
			c = 8
		}
		concurrency = c
	}

	pool := batch.Pool{Workers: concurrency, PerHost: 2}
	results := pool.RunBatch(context.Background(), client, eng, items, pipeline.Options{
		Format:   format,
		MaxChars: maxChars,
	})

	type jsonResult struct {
		URL     string      `json:"url"`
		OK      bool        `json:"ok"`
		Content string      `json:"content,omitempty"`
		Error   string      `json:"error,omitempty"`
	}
	out := make([]jsonResult, len(results))
	for i, r := range results {
		out[i] = jsonResult{URL: r.URL, OK: r.OK, Content: r.Output, Error: r.Error}
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	sendToolResponse(reqID, string(data))
}

func formatWithMeta(res *pipeline.Result, format pipeline.Format) string {
	if format == pipeline.FormatMarkdown {
		var sb strings.Builder
		m := res.Meta
		sb.WriteString(fmt.Sprintf("> **%s** · %d · ~%d tokens",
			m.Title, m.StatusCode, m.EstTokens))
		if m.Truncated {
			sb.WriteString(" · ⚠ truncated")
		}
		sb.WriteString("\n> ")
		sb.WriteString(m.FinalURL)
		sb.WriteString("\n\n")
		sb.WriteString(res.Output)
		return sb.String()
	}
	return res.Output
}

func categoriseError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "blocked_private"):
		return "error [blocked_private]: " + msg
	case strings.Contains(msg, "too_large"):
		return "error [too_large]: " + msg
	case strings.Contains(msg, "http_4"):
		return "error [http_4xx]: " + msg
	case strings.Contains(msg, "http_5"):
		return "error [http_5xx]: " + msg
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "error [timeout]: " + msg
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return "error [dns]: " + msg
	default:
		return "error: " + msg
	}
}
