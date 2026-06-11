# Symaira Fetch (`symfetch`)

AI-native web fetch engine for LLM agents. Fetches web pages using browser-impersonating TLS/HTTP2, transforms HTML into LLM-optimized Markdown or JSON via a semantic DOM pipeline — without JavaScript execution overhead.

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

## Usage

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

## Limitations (v0.1)

- **No JavaScript execution** — SPAs that require JS rendering may return incomplete content. The JS-exec seam (`pipeline.Engine`) is designed for future QuickJS/wazero integration.
- **No JS challenges** — Cloudflare Managed Challenge / Turnstile requires a real browser. TLS/HTTP2 fingerprinting passes basic bot-detection.

## License

MIT — see [LICENSE](LICENSE).
