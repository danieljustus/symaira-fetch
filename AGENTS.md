# Agent Instructions

This repository is the public MIT-licensed Symaira Fetch self-hosted foundation.

## Repository Role

- Keep this repository buildable, testable, and runnable without any private commercial code.
- Self-hosted Symaira Fetch remains free and open source under the MIT License.
- Do not add Cloud Pro, hosted-service, tenant-management, billing, subscription, customer-support, or commercial deployment code here.

## Architecture & Code Style Guidelines

- **CGO-Free Go**: All HTTP clients and parsing logic must remain 100% CGO-free for cross-platform compilation. Use `CGO_ENABLED=0` in all builds.
- **Zero Stdio Pollution**: The MCP server transport runs over stdio. Under no circumstances must any package print to `os.Stdout` unless it is a structured JSON-RPC 2.0 message. All logs, warnings, and trace states must route to `os.Stderr` via `log/slog` to prevent client handshake drop errors.
- **Safe Directories**: Use standard XDG directories (`~/.config/symfetch`, `~/.cache/symfetch`, `~/.local/state/symfetch`) for config, cache, and sessions.
- **SSRF Guard**: In MCP mode, `AllowPrivate` must default to `false`. Never allow fetching RFC1918/loopback addresses via an LLM-driven server without explicit opt-in.

## Browser Fingerprint Policy

- The Chrome/Firefox impersonation profiles target specific Chrome/Firefox versions. When the pinned version drifts significantly from current releases (typically every quarter), update the preset in `internal/fetch/azuretls.go` and document the target Chrome version.
- v0.1 passes TLS/HTTP2-layer fingerprint checks but **does not pass JavaScript challenges** (Cloudflare Managed Challenge, Turnstile). JS execution is a future milestone via the `pipeline.Engine` interface.

## Dependency Maintenance

- The primary impersonation client is `github.com/Noooste/azuretls-client` (MIT, pure Go).
- If azuretls becomes unmaintained, swap the implementation in `internal/fetch/azuretls.go` only — the `fetch.Client` interface isolates all callers.
- Keep `modernc.org/sqlite` out of this repo in v0.x (no query use cases). Add it only if a crawl frontier feature lands.
