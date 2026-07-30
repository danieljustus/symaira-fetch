---
name: symfetch
description: "Use Symaira Fetch (symfetch) when an AI agent needs to fetch, extract, and structure web content — Markdown/JSON, CSS selector extraction, JSON-LD schema queries, BM25 relevance filtering, and Wayback Machine fallback. Covers CLI, MCP stdio, and HTTP POST /fetch surfaces."
version: 1.0.0
author: Symaira Fetch
license: Apache-2.0
platforms: [linux, macos, windows]
---

# Symaira Fetch — Agent Skill

> **AI-native web fetch engine for LLM agents.** Fetches web pages using browser-impersonating TLS/HTTP2, transforms HTML into LLM-optimized Markdown or JSON via a semantic DOM pipeline — without JavaScript execution overhead.

## Surface Selection Guide

| Surface | When to Choose | Setup |
|---------|---------------|-------|
| **CLI** (`symfetch <url>`) | Ad-hoc fetches, debugging, scripting in a shell. Best for one-off exploration or when you need exact control over flags. | `brew install danieljustus/tap/symfetch` or download a release binary |
| **MCP stdio** (`symfetch mcp`) | An AI agent (Claude Code, Cursor, Hermes) that speaks MCP/JSON-RPC. The agent calls `fetch_url` / `fetch_batch` as tool calls. No shelling out required. | Add to `~/.claude/claude_desktop_config.json` as an MCP server (see [README §MCP Integration](#mcp-integration)) |
| **HTTP POST /fetch** (`symfetch mcp --http`) | Non-MCP integrations — curl, webhooks, serverless functions, or any HTTP client. Bearer-authenticated, JSON body. | `symfetch mcp --http --addr :8787 --token "$TOKEN"` |

### Quick Decision

- **You are an agent with MCP tool access** → use `fetch_url` / `fetch_batch` via MCP (no shell needed).
- **You are running in a terminal** → use the CLI directly.
- **You receive webhook requests or speak HTTP** → use the `POST /fetch` endpoint.

## Agent-Relevant CLI Flags

These flags are the primary way agents tune output from the CLI. All flags are documented against `cmd/symfetch/main.go`.

| Flag | Type | Default | Purpose |
|------|------|---------|---------|
| `--format` / `-f` | `markdown`, `json`, `text`, `html` | `markdown` | Output format. Use `json` for programmatic consumption. |
| `--raw` | bool | `false` | Bypass semantic processing — return the raw decoded response body. Useful when you need the untouched HTML or non-HTML payload. |
| `--selector` | string | `""` | CSS selector to extract specific elements (e.g. `"table.pricing"`, `".article-body"`). Bypasses the semantic BestBlock heuristic entirely. |
| `--query` | string | `""` | BM25 relevance query — returns only sections matching the search term, preserving headings and structure. Use to answer specific questions about a page. |
| `--top-k` | int | `0` | Number of top BM25-matching sections to return (0 = all matching). Pair with `--query`. |
| `--frontmatter` | bool | `false` | Prepend YAML frontmatter with `title`, `url`, `fetched_at`, `lang`, `tokens_est`, and optional `final_url` / `schema_type`. |
| `--schema-path` | string | `""` | JSON-LD query path. Typed selectors (`@Recipe:name`, `@Product:aggregateRating.ratingValue`) filter by `@type` then traverse a dot-path. Plain field paths (`name`, `headline`, `@type`) search all JSON-LD islands including `@graph` nodes. Returns empty on miss. |
| `--links` | bool | `false` | Append a Links section with every href found on the page. |
| `--store-full-text` | bool | `false` | Enable truncate-and-store (Hermes-style): return a head+tail window, cache the full text for later retrieval. |
| `--char-limit` | int | `15000` | Per-page character limit for truncate-and-store. |
| `--at` | string | `""` | Fetch from Wayback Machine archive at timestamp (`YYYYMMDDHHmmss`). |
| `--wayback-fallback` | bool | `false` | Enable Wayback Machine as automatic fallback on 404 or thin-content detection. |
| `--profile` | string | `chrome` | Browser fingerprint profile: `chrome`, `firefox`, `honest`. |
| `--no-cache` | bool | `false` | Disable response caching. |
| `--timeout` | string | `30s` | Request timeout (e.g. `30s`, `1m`). MCP caps at 120s. |
| `--max-chars` | int | `20000` | Maximum characters in semantic output. |
| `--session` | string | `""` | Named persistent cookie jar (for stateful browsing). |
| `--header` / `-H` | stringArray | — | Extra request header (`"Key: Value"`). |
| `--request` / `-X` | string | `GET` | HTTP method. |
| `--data` | string | `""` | Request body data (for POST/PUT). |
| `--robots` | bool | `false` | Check `robots.txt` before fetching. |
| `--allow-private` | bool | `false` | Allow fetching private/loopback addresses (dangerous). |

### Multi-URL Input

The CLI accepts multiple URLs as positional arguments. When `--format json` is used with multiple URLs, output is a JSON array with per-URL `ok`, `output`, and `error` fields. When `--concurrency` > 1, URLs are fetched in parallel with an adaptive per-host rate limiter.

```bash
# Sequential, Markdown (separated by ---)
symfetch https://example.com https://iana.org

# Batch JSON
symfetch https://example.com https://iana.org --format json

# Parallel
symfetch https://example.com https://iana.org --concurrency 4
```

## MCP Tool Reference

When connected via MCP stdio, two tools are available.

### `fetch_url`

Fetch a single web page and return LLM-optimized content.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `url` | string | *(required)* | The URL to fetch (must be http or https). |
| `format` | string | `"markdown"` | Output format: `markdown`, `json`, `text`. |
| `max_chars` | integer | `20000` | Maximum characters in output. |
| `include_links` | boolean | `false` | Append a Links section with all hrefs. |
| `raw` | boolean | `false` | Return raw decoded response body without semantic processing. |
| `timeout_seconds` | integer | `30` | Request timeout in seconds (max 120). |
| `css_selector` | string | `""` | CSS selector to extract specific elements (e.g. `"table.pricing"`, `".article-body"`). Bypasses semantic BestBlock heuristic. |
| `frontmatter` | boolean | `false` | Prepend YAML frontmatter with metadata (title, url, fetched_at, lang, tokens). |
| `schema_path` | string | `""` | JSON-LD query path. Typed selectors (`@Recipe:name`) filter by `@type` then traverse a dot-path. Plain field paths (`name`) search all JSON-LD islands. Returns empty on miss. |
| `store_full_text` | boolean | `false` | Enable truncate-and-store: returns head+tail for long pages, stores full text in cache. |
| `char_limit` | integer | `15000` | Per-page character limit for truncate-and-store. |
| `wayback_timestamp` | string | `""` | Fetch from Wayback Machine at this timestamp (`YYYYMMDDHHmmss`). |
| `wayback_fallback` | boolean | `false` | Enable Wayback Machine as automatic fallback on 404/thin-content. |
| `query` | string | `""` | BM25 query for relevance filtering — returns only matching sections. |
| `top_k` | integer | `0` | Number of top BM25 sections to return (0 = all). |

### `fetch_batch`

Fetch multiple URLs concurrently and return results in input order.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `urls` | string[] | *(required)* | URLs to fetch (max 20). |
| `format` | string | `"markdown"` | Per-page output format: `markdown`, `json`, `text`. |
| `max_chars` | integer | `20000` | Per-page character budget. |
| `concurrency` | integer | `4` | Maximum parallel fetches (max 8). |
| `store_full_text` | boolean | `false` | Enable truncate-and-store for each page. |
| `char_limit` | integer | `15000` | Per-page character limit for truncate-and-store. |

## HTTP POST /fetch

When using the HTTP REST server mode, send a POST request with a JSON body to `http://<addr>:8787/fetch`:

```bash
curl -X POST http://localhost:8787/fetch \
  -H "Authorization: Bearer $SYMFETCH_HTTP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "format": "markdown",
    "css_selector": "div.content",
    "frontmatter": true
  }'
```

The request body accepts the same parameters as `fetch_url` (documented above). Authentication is required when the server binds to a non-loopback address.

## Failure Modes

### 1. Thin-Content Auto-Fallback

When a page returns a navigation shell, SPA skeleton, or otherwise thin content (detected by text density heuristics), the engine automatically retries the URL with these strategies:

1. **`.md` twin** — If `https://example.com/page` exists, it attempts `https://example.com/page.md`.
2. **`llms.txt`** — The engine checks the site root for `llms.txt` and searches for the URL's path as a heading anchor.
3. **Wayback Machine** — When `--wayback-fallback` or `--at` is set, falls back to the nearest archived snapshot.

**What the agent sees:** The output is the fallback content (`.md`, `llms.txt`, or Wayback snapshot) rather than the thin HTML shell. If all fallbacks fail, a descriptive error is returned.

### 2. 4xx Recovery Hints

On HTTP 4xx errors (especially 404, 403, 410), the engine probes:

- **Ancestor paths** — e.g. for `https://example.com/blog/2024/post`, it tries `/blog/2024/`, `/blog/`, and `/` to find a reachable index.
- **Sitemap discovery** — It checks `/sitemap.xml` and `/robots.txt` for alternative URLs.

**What the agent sees:** The error message includes hints like `"page not found; nearest reachable ancestor: https://example.com/blog/"`. Use these hints to adjust the URL and retry.

### 3. Truncate-and-Store Footers

When `--store-full-text` is enabled and a page exceeds `--char-limit` (default 15000), the engine returns a head+tail window bounded to `--char-limit` characters and appends a structured footer:

```
...
[Truncated: full text stored in cache at ~/.cache/symfetch/full/<hash>.md]
```

**What the agent sees:** The response includes the opening content (title, headings, first N chars), the closing content (last N chars), and a footer with the cache path. The full text is retrievable from disk for later processing. This keeps LLM context bounded while preserving access to the complete content.

## Installation

```bash
# Homebrew (recommended)
brew install danieljustus/tap/symfetch

# From source
go install github.com/danieljustus/symaira-fetch/cmd/symfetch@latest
```

Or download a pre-built binary from the [releases page](https://github.com/danieljustus/symaira-fetch/releases).

## Configuration

```bash
# Write default config to ~/.config/symfetch/config.toml
symfetch config init
```

Environment variables override config file values:

| Variable | Config Field | Description |
|----------|-------------|-------------|
| `SYMFETCH_CACHE_DIR` | `cache.dir` | Override cache directory |
| `SYMFETCH_CACHE_MAX_SIZE_MB` | `cache.max_size_mb` | Maximum cache size in MB |
| `SYMFETCH_HTTP_PROFILE` | `http.profile` | Browser profile |
| `SYMFETCH_HTTP_TIMEOUT_SECONDS` | `http.timeout_seconds` | Request timeout in seconds |
| `SYMFETCH_SECURITY_ALLOW_PRIVATE` | `security.allow_private` | Allow private/loopback addresses |

## Limitations

- **No JavaScript execution** — SPAs requiring JS rendering may return incomplete content. The thin-content auto-fallback tries `.md` twins and `llms.txt` as a workaround.
- **No JS challenges** — Cloudflare Managed Challenge / Turnstile requires a real browser. TLS/HTTP2 fingerprinting passes basic bot-detection but not JS challenges.
