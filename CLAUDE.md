# Symaira Fetch Developer Guidelines

Guidelines and commands for developers and AI agents working on this codebase.

## Build and Test Commands

- **Build binary**: `go build -o symfetch ./cmd/symfetch`
- **Run all tests**: `go test ./...`
- **Run verbose tests**: `go test -v ./...`
- **Run with race detector**: `go test -race ./...`
- **Build via make**: `make build`

## CLI Verification Cheatsheet

- **Check version**: `./symfetch version`
- **Fetch a URL**: `./symfetch https://example.com`
- **Raw response**: `./symfetch https://example.com --raw`
- **JSON output**: `./symfetch https://example.com --format json`
- **Multiple URLs**: `./symfetch https://example.com https://iana.org`
- **With links**: `./symfetch https://example.com --links`
- **Firefox profile**: `./symfetch https://example.com --profile firefox`
- **Write default config**: `./symfetch config init`
- **Start MCP server**: `./symfetch mcp`

## Code Style & Formatting

- **Go code style**: Follow standard `gofmt` guidelines.
- **Indentation**: Go source files: tabs. YAML/JSON/Markdown: 2 spaces.
- **Imports order**: Standard Go grouping (stdlib block, external modules block).
- **Zero-CGO**: Maintain CGO-free compilations. `CGO_ENABLED=0` always.
- **Standard Library first**: Prefer stdlib over external dependencies where possible.
- **Logging**: Use `log/slog` routed to `os.Stderr`. Never use `fmt.Print*` to stdout outside the CLI layer.
