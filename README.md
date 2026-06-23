# Symaira Fetch (`symfetch`)

[![CI](https://github.com/danieljustus/symaira-fetch/actions/workflows/ci.yml/badge.svg)](https://github.com/danieljustus/symaira-fetch/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/danieljustus/symaira-fetch)](https://goreportcard.com/report/github.com/danieljustus/symaira-fetch)
[![Latest Release](https://img.shields.io/github/v/release/danieljustus/symaira-fetch)](https://github.com/danieljustus/symaira-fetch/releases)

> **AI-native web fetch engine for LLM agents.** Fetches web pages using browser-impersonating TLS/HTTP2, transforms HTML into LLM-optimized Markdown or JSON via a semantic DOM pipeline — without JavaScript execution overhead.


## Why Symaira Fetch?

LLM agents require accurate, clean, and low-latency web page content to perform tasks. Standard scraping libraries either execute JavaScript (causing massive CPU/memory overhead and slow response times) or get blocked by bot-detection because of default TLS fingerprints.

Symaira Fetch solves this by:
1. **Simulating real browser TLS and HTTP/2 handshakes** (Chrome/Firefox JA4/JA3) to bypass basic Cloudflare/Akamai bot walls without launching a browser.
2. **Filtering the DOM semantically** before converting to Markdown, reducing LLM token context usage by up to 80% while retaining structure and data islands (`__NEXT_DATA__`, JSON-LD).

## Architecture

```
URL ──▶ Browser TLS ──▶ HTML ──▶ DomFilter ──▶ Content Scoring ──▶ 8-Category ──▶ TokenCompressor ──▶ Markdown/JSON
         (Chrome/Firefox    Fetch   (safe tags,     (text density,     Classification   (semantic,        (LLM-optimized)
          JA4/HTTP2)               remove junk)     link density,      (article, nav,    token-aware)
                                                      island detect)    code, data, ...)
```

## Features

- **Browser-impersonating TLS** — Chrome/Firefox JA4/HTTP2 fingerprints via [azuretls](https://github.com/Noooste/azuretls-client)
- **Semantic DOM pipeline** — DomFilter → content scoring → 8-category classification → TokenCompressor → Markdown/JSON
- **Data island extraction** — `__NEXT_DATA__`, `application/ld+json`, `__PRELOADED_STATE__` without JS execution
- **MCP server** — JSON-RPC 2.0 over stdio, works with Claude Code and any MCP client
- **CGO-free** — cross-compiles to Linux/macOS/Windows amd64+arm64
- **SSRF guard** — blocks private/loopback addresses in MCP mode

## Installation

```bash
brew install danieljustus/tap/symfetch
```

Or download from [GitHub Releases](https://github.com/danieljustus/symaira-fetch/releases).

## Usage Example

```
$ symfetch https://example.com
# Example Domain

This domain is for use in illustrative examples in documents. You may use this
domain in literature without prior coordination or asking for permission.

[More information...](https://www.iana.org/domains/reserved)
```


```bash
# Fetch a URL (LLM-optimized Markdown)
symfetch https://example.com

# JSON output
symfetch https://example.com --format json

# Raw response body
symfetch https://example.com --raw

# Multiple URLs
symfetch https://example.com https://iana.org

# Firefox fingerprint
symfetch https://example.com --profile firefox

# With links table
symfetch https://example.com --links

# POST with JSON body
symfetch https://api.example.com -X POST -H "Content-Type: application/json" -d '{"key":"value"}'

# Custom proxy
symfetch https://example.com --proxy socks5://localhost:1080

# Respect robots.txt
symfetch https://example.com --robots

# Named session (persistent cookie jar)
symfetch https://example.com --session my-session

# Allow private/loopback addresses (dangerous, CLI-only)
symfetch http://localhost:8080 --allow-private

# Print version
symfetch --version

# Write default config
symfetch config init

# Start MCP server
symfetch mcp
```

## MCP Integration

Add to your Claude Code MCP config (`~/.claude/claude_desktop_config.json` or project `.claude/settings.json`):

```json
{
  "mcpServers": {
    "symfetch": {
      "command": "symfetch",
      "args": ["mcp"]
    }
  }
}
```

Available MCP tools:

| Tool | Description |
|------|-------------|
| `fetch_url` | Fetch a single URL, returns Markdown/JSON/text |
| `fetch_batch` | Fetch up to 20 URLs concurrently |

> **Note:** The MCP server caps `timeout_seconds` at 120 seconds. The CLI `--timeout` flag has no maximum, but values above 120s will produce a warning since MCP requests cannot exceed the cap.

## Configuration

Symaira Fetch can be configured via config file or environment variables:

```bash
# Write default config to ~/.config/symfetch/config.toml
symfetch config init
```

Explicit CLI flags (`--no-cache`, `--cache-ttl`, `--concurrency`, etc.) take precedence over config-file and environment-variable values when both are set.

| Environment Variable | Config Field | Description |
|---------------------|--------------|-------------|
| `SYMFETCH_CACHE_DIR` | `cache.dir` | Override cache directory (default: `~/.cache/symfetch`) |
| `SYMFETCH_CACHE_MAX_SIZE_MB` | `cache.max_size_mb` | Maximum cache size in MB (default: 100) |
| `SYMFETCH_HTTP_PROFILE` | `http.profile` | Browser profile: chrome, firefox, honest |
| `SYMFETCH_HTTP_TIMEOUT_SECONDS` | `http.timeout_seconds` | Request timeout in seconds |
| `SYMFETCH_SECURITY_ALLOW_PRIVATE` | `security.allow_private` | Allow fetching private/loopback addresses |

## Browser Fingerprint

Symaira Fetch impersonates real browser TLS and HTTP/2 fingerprints to bypass basic bot-detection. The current target versions are:

| Profile | Target | Notes |
|---------|--------|-------|
| Chrome | Chrome 135 | TLS/HTTP2 JA4 fingerprint, order pseudo-headers |
| Firefox | Firefox | TLS/HTTP2 fingerprint pattern (no specific version pinned) |

These fingerprints are maintained by the [azuretls](https://github.com/Noooste/azuretls-client) library (v1.13.2). The target Chrome version is updated quarterly as Chrome releases drift from the pinned fingerprint.

> **Note:** TLS/HTTP2 fingerprinting passes basic bot-detection (Cloudflare, Akamai) but does **not** pass JavaScript challenges (Cloudflare Managed Challenge, Turnstile). See [Limitations](#limitations-v01) for details.

## Limitations (v0.1)

- **No JavaScript execution** — SPAs that require JS rendering may return incomplete content. The JS-exec seam (`pipeline.Engine`) is designed for future QuickJS/wazero integration.
- **No JS challenges** — Cloudflare Managed Challenge / Turnstile requires a real browser. TLS/HTTP2 fingerprinting passes basic bot-detection.


## Development

### Building from Source

To compile the binary locally:
```bash
make build
```

### Running Tests

To run the unit tests and integration tests:
```bash
make test
```

### Installing from source

To install the latest development version directly via Go:
```bash
go install github.com/danieljustus/symaira-fetch/cmd/symfetch@latest
```

## License

MIT — see [LICENSE](LICENSE).
