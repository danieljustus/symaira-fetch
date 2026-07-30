package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/danieljustus/symaira-corekit/mcpserver"
	"github.com/danieljustus/symaira-fetch/internal/apicommon"
	"github.com/danieljustus/symaira-fetch/internal/batch"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

const (
	maxTimeoutSec = 120
	maxCharsLimit = 500_000
)

func registerTools(srv *mcpserver.Server, client fetch.Client, eng pipeline.Engine, adaptivePool *batch.AdaptivePool) {
	registerWaybackTools(srv)

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "fetch_url",
		Description: "Fetch a web page and return LLM-optimized content. Uses browser-impersonating TLS to bypass basic bot detection. Returns Markdown by default.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"url": {"type": "string", "description": "The URL to fetch (must be http or https)"},
				"format": {"type": "string", "description": "Output format: markdown (default), json, text", "enum": ["markdown", "json", "text"]},
				"max_chars": {"type": "integer", "description": "Maximum characters in output (default 20000)"},
				"include_links": {"type": "boolean", "description": "Append a Links section with all hrefs (default false)"},
				"raw": {"type": "boolean", "description": "Return raw decoded response body without semantic processing"},
				"timeout_seconds": {"type": "integer", "description": "Request timeout in seconds (default 30, max 120)", "maximum": 120},
				"css_selector": {"type": "string", "description": "CSS selector to extract specific elements (e.g., 'table.pricing', '.article-body')"},
				"frontmatter": {"type": "boolean", "description": "Prepend YAML frontmatter with metadata (title, url, fetched_at, lang, tokens)"},
				"schema_path": {"type": "string", "description": "JSON-LD query path. Typed selectors (e.g., '@Recipe:name', '@Product:aggregateRating.ratingValue') filter by @type then traverse a dot-path. Plain field paths (e.g., 'name', 'headline', '@type') search all JSON-LD islands including @graph nodes. Returns empty on miss."},
				"store_full_text": {"type": "boolean", "description": "Enable truncate-and-store: returns head+tail for long pages, stores full text in cache (default false)"},
				"char_limit": {"type": "integer", "description": "Per-page character limit for truncate-and-store (default 15000)"},
				"wayback_timestamp": {"type": "string", "description": "Fetch from Wayback Machine archive at this timestamp (YYYYMMDDHHmmss). When set, automatically uses Wayback fallback for 404/thin-content."},
				"wayback_fallback": {"type": "boolean", "description": "Enable Wayback Machine as automatic fallback on 404/thin-content (default false)"},
				"query": {"type": "string", "description": "BM25 query for relevance filtering. Returns only sections matching the query, preserving headings and structure."},
				"top_k": {"type": "integer", "description": "Number of top sections to return for relevance filtering (0 = all matching sections)"}
			},
			"required": ["url"]
		}`),
		Handler: makeFetchURLHandler(client, eng),
	})

	srv.RegisterTool(&mcpserver.Tool{
		Name:        "fetch_batch",
		Description: "Fetch multiple URLs concurrently and return results in input order. Each URL is processed independently; one failure does not abort the batch.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"urls": {"type": "array", "items": {"type": "string"}, "description": "URLs to fetch (max 20)"},
				"format": {"type": "string", "description": "Output format for each result: markdown, json, text"},
				"max_chars": {"type": "integer", "description": "Per-page character budget (default 20000)"},
				"concurrency": {"type": "integer", "description": "Maximum parallel fetches (default 4, max 8)"},
				"store_full_text": {"type": "boolean", "description": "Enable truncate-and-store for each page (default false)"},
				"char_limit": {"type": "integer", "description": "Per-page character limit for truncate-and-store (default 15000)"}
			},
			"required": ["urls"]
		}`),
		Handler: makeFetchBatchHandler(client, eng, adaptivePool),
	})
}

func makeFetchURLHandler(client fetch.Client, eng pipeline.Engine) func(ctx context.Context, input json.RawMessage) (any, error) {
	return func(ctx context.Context, input json.RawMessage) (any, error) {
		var args map[string]interface{}
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("invalid input: %w", err)
		}

		rawURL, ok := args["url"].(string)
		if !ok || rawURL == "" {
			return nil, fmt.Errorf("missing required argument 'url'")
		}

		if err := apicommon.ValidateURLScheme(rawURL); err != nil {
			return nil, err
		}

		format := pipeline.FormatMarkdown
		if f, ok, err := stringArg(args, "format"); err != nil {
			return nil, err
		} else if ok && f != "" {
			parsed, err := pipeline.ParseFormat(f)
			if err != nil {
				return nil, fmt.Errorf("invalid format: %w", err)
			}
			format = parsed
		}

		maxChars := 20000
		if v, ok, err := intArg(args, "max_chars"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			maxChars = v
		}
		if maxChars > maxCharsLimit {
			slog.Debug("max_chars capped", "requested", maxChars, "limit", maxCharsLimit)
			maxChars = maxCharsLimit
		}

		includeLinks, _, err := boolArg(args, "include_links")
		if err != nil {
			return nil, err
		}
		raw, _, err := boolArg(args, "raw")
		if err != nil {
			return nil, err
		}
		frontmatter, _, err := boolArg(args, "frontmatter")
		if err != nil {
			return nil, err
		}
		storeFullText, _, err := boolArg(args, "store_full_text")
		if err != nil {
			return nil, err
		}

		cssSelector, _, err := stringArg(args, "css_selector")
		if err != nil {
			return nil, err
		}
		schemaPath, _, err := stringArg(args, "schema_path")
		if err != nil {
			return nil, err
		}

		waybackFallback, _, err := boolArg(args, "wayback_fallback")
		if err != nil {
			return nil, err
		}
		waybackTimestamp, _, err := stringArg(args, "wayback_timestamp")
		if err != nil {
			return nil, err
		}
		if waybackTimestamp != "" {
			waybackFallback = true
		}

		query, _, err := stringArg(args, "query")
		if err != nil {
			return nil, err
		}
		topK := 0
		if v, ok, err := intArg(args, "top_k"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			topK = v
		}

		charLimit := pipeline.DefaultCharLimit
		if v, ok, err := intArg(args, "char_limit"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			charLimit = v
		}

		timeoutSec := 30
		if v, ok, err := intArg(args, "timeout_seconds"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			timeoutSec = v
		}
		if timeoutSec > maxTimeoutSec {
			slog.Debug("timeout_seconds capped", "requested", timeoutSec, "limit", maxTimeoutSec)
			timeoutSec = maxTimeoutSec
		}

		fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()

		if raw {
			resp, err := client.Fetch(fetchCtx, fetch.Request{URL: rawURL, AllowPrivate: false})
			if err != nil {
				return nil, apicommon.CategoriseError(err)
			}
			return string(resp.Body), nil
		}

		res, err := pipeline.Run(fetchCtx, client, eng, rawURL, pipeline.Options{
			Format: format,
			Content: pipeline.ContentOptions{
				MaxChars:     maxChars,
				IncludeLinks: includeLinks,
			},
			CSSSelector:      cssSelector,
			Frontmatter:      frontmatter,
			SchemaPath:       schemaPath,
			StoreFullText:    storeFullText,
			CharLimit:        charLimit,
			WaybackFallback:  waybackFallback,
			WaybackTimestamp: waybackTimestamp,
			Query:            query,
			TopK:             topK,
		})
		if err != nil {
			return nil, apicommon.CategoriseError(err)
		}

		return apicommon.FormatWithMeta(res, format, frontmatter), nil
	}
}

func makeFetchBatchHandler(client fetch.Client, eng pipeline.Engine, adaptivePool *batch.AdaptivePool) func(ctx context.Context, input json.RawMessage) (any, error) {
	return func(ctx context.Context, input json.RawMessage) (any, error) {
		var args map[string]interface{}
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("invalid input: %w", err)
		}

		rawURLs, ok := args["urls"].([]interface{})
		if !ok || len(rawURLs) == 0 {
			return nil, fmt.Errorf("missing required argument 'urls'")
		}
		if len(rawURLs) > 20 {
			return nil, fmt.Errorf("maximum 20 URLs per batch")
		}

		items := make([]batch.Item, 0, len(rawURLs))
		for _, u := range rawURLs {
			s, ok := u.(string)
			if !ok || s == "" {
				continue
			}
			if err := apicommon.ValidateURLScheme(s); err != nil {
				return nil, fmt.Errorf("invalid URL %q: %w", s, err)
			}
			items = append(items, batch.Item{URL: s})
		}

		format := pipeline.FormatMarkdown
		if f, ok, err := stringArg(args, "format"); err != nil {
			return nil, err
		} else if ok && f != "" {
			parsed, err := pipeline.ParseFormat(f)
			if err != nil {
				return nil, fmt.Errorf("invalid format: %w", err)
			}
			format = parsed
		}

		maxChars := 20000
		if v, ok, err := intArg(args, "max_chars"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			maxChars = v
		}
		if maxChars > maxCharsLimit {
			slog.Debug("max_chars capped", "requested", maxChars, "limit", maxCharsLimit)
			maxChars = maxCharsLimit
		}

		concurrency := 4
		if v, ok, err := intArg(args, "concurrency"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			if v > 8 {
				v = 8
			}
			concurrency = v
		}

		storeFullText, _, err := boolArg(args, "store_full_text")
		if err != nil {
			return nil, err
		}
		charLimit := pipeline.DefaultCharLimit
		if v, ok, err := intArg(args, "char_limit"); err != nil {
			return nil, err
		} else if ok && v > 0 {
			charLimit = v
		}

		pool := batch.Pool{Workers: concurrency, PerHost: 2, Adaptive: true, AdaptivePool: adaptivePool}
		results := pool.RunBatch(ctx, client, eng, items, pipeline.Options{
			Format: format,
			Content: pipeline.ContentOptions{
				MaxChars: maxChars,
			},
			StoreFullText: storeFullText,
			CharLimit:     charLimit,
		})

		type jsonResult struct {
			URL     string `json:"url"`
			OK      bool   `json:"ok"`
			Content string `json:"content,omitempty"`
			Error   string `json:"error,omitempty"`
		}
		out := make([]jsonResult, len(results))
		for i, r := range results {
			out[i] = jsonResult{URL: r.URL, OK: r.OK, Content: r.Output, Error: r.Error}
		}

		data, _ := json.MarshalIndent(out, "", "  ")
		return string(data), nil
	}
}
