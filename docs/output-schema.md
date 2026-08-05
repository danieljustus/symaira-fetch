# Symfetch Output Schema

Version: 1 (schema version of this contract)

This document is the versioned, machine-checkable contract for symfetch
outputs consumed by other tools (e.g. symaira-browse) and future clients.
Any consumer SHOULD validate against this document. Changes to field names,
semantics, or the rendering rules below MUST be additive and backward
compatible, and MUST bump `schema_version` (see
[Versioning](#versioning-and-backward-compatibility)).

Counterpart issues: symaira-browse#24 (B-20), symaira-fetch#245 (X-2).

---

## 1. Output surfaces

symfetch has three output surfaces; all of them derive from the same
`agentdom.Document` / `Meta` model:

| Surface | Entry point | Format |
|---|---|---|
| CLI (stdout) | `symfetch [url...]` | markdown (default), json, text, html |
| MCP tool `fetch_url` | MCP stdio server | text content (markdown by default) |
| MCP tool `fetch_batch` | MCP stdio server | text content |
| HTTP `POST /fetch` | `symfetch mcp --http` | JSON envelope |

This contract covers the CLI and the JSON envelopes; MCP tool results carry
the same rendered payloads inside JSON-RPC `content[].text`.

## 2. JSON single-page output (`--format json`)

A single fetched page renders as the `Document` object:

```json
{
  "url": "https://example.com",
  "final_url": "https://example.com/",
  "title": "Example Domain",
  "lang": "en",
  "content": [
    {
      "id": "@e1",
      "category": "heading",
      "tag": "h1",
      "text": "Example Domain",
      "attrs": {"href": "https://example.com/"},
      "children": []
    }
  ],
  "interactive": [],
  "islands": [
    {"source": "ld+json", "json": {"@context": "https://schema.org"}}
  ]
}
```

### Document fields (stable)

| Field | Type | Presence | Meaning |
|---|---|---|---|
| `url` | string | always | Original requested URL |
| `final_url` | string | optional | Final URL after redirects; omitted when equal to `url` |
| `title` | string | optional | Document title, if any |
| `lang` | string | optional | Detected language tag, if any |
| `content` | array of Element | always | Scored main content in document order |
| `interactive` | array of Element | always | Flat list of `@eN`-tagged interactive elements |
| `islands` | array of DataIsland | optional | JSON-LD / hydration data islands (`source`, `json`) |

### Element fields (stable)

| Field | Type | Presence | Meaning |
|---|---|---|---|
| `id` | string | optional | Agent tag `@eN` for interactive elements |
| `category` | string | always | Semantic category (heading, text, link, image, list, table, code, …) |
| `tag` | string | optional | Original HTML tag |
| `text` | string | optional | Visible text content |
| `attrs` | object | optional | Relevant HTML attributes (e.g. `href`, `src`) |
| `children` | array of Element | optional | Nested elements |

### DataIsland fields (stable)

| Field | Type | Presence | Meaning |
|---|---|---|---|
| `source` | string | always | Island origin, e.g. `ld+json`, `__NEXT_DATA__` |
| `json` | object/array | always | Parsed island payload |

## 3. JSON batch output (CLI `--format json` with multiple URLs)

Multiple URLs with `--format json` render as a top-level JSON **array** of
envelope objects (not Documents), one per URL, in request order:

```json
[
  {
    "url": "https://example.com/ok",
    "ok": true,
    "output": "{...Document...}"
  },
  {
    "url": "https://example.com/broken",
    "ok": false,
    "error": "[http_4xx] ..."
  }
]
```

### Envelope fields (stable)

| Field | Type | Presence | Meaning |
|---|---|---|---|
| `url` | string | always | Requested URL |
| `ok` | boolean | always | Success flag |
| `output` | string | only when `ok` | Rendered output for the URL (format-dependent; for json format it is the stringified Document) |
| `error` | string | only when `!ok` | Categorised error message (see §5) |

## 4. Markdown output and YAML frontmatter

Default markdown output is a blockquote metadata header followed by the
rendered markdown:

```text
> **Title** · 200 · ~1234 tokens
> https://example.com/

…rendered markdown…
```

`--frontmatter` prepends YAML frontmatter between `---` delimiters.
Frontmatter keys are stable:

| Key | Type | Presence | Meaning |
|---|---|---|---|
| `title` | string | optional | Document title |
| `url` | string | always | Requested URL |
| `final_url` | string | optional | Final URL after redirects (when different from `url`) |
| `fetched_at` | string | always | UTC RFC3339 fetch timestamp |
| `lang` | string | optional | Detected language tag |
| `tokens_est` | int | always | Estimated token count (chars / 4) |
| `schema_type` | string | optional | JSON-LD type extracted from the first `ld+json` island |
| `source` | string | optional | Origin marker (e.g. wayback snapshot) |
| `snapshot_at` | string | optional | Wayback snapshot timestamp when fetched from the archive |

## 5. Error categories

Errors are prefixed with a bracketed category so clients can branch without
parsing message text:

| Category | Meaning |
|---|---|
| `[blocked_private]` | SSRF guard rejected a private/loopback address |
| `[too_large]` | Response body exceeded the configured max-body |
| `[http_4xx]` | HTTP 4xx (with nearest reachable ancestor + candidates when recovery probing ran) |
| `[http_5xx]` | HTTP 5xx |
| `[dns]` | DNS resolution failure |
| `[timeout]` | Request exceeded the timeout |
| *(none)* | Any other error (validation, parse, render) |

## 6. HTTP server envelope

`POST /fetch` always returns a JSON envelope:

```json
{"ok": true, "output": "<rendered output>", "meta": {...}}
{"ok": false, "error": "..."}
```

`meta` mirrors `Meta` (§7). The HTTP envelope is the only surface where the
`Meta` object is exposed directly.

## 7. Meta object (stable)

| Field | Type | Presence | Meaning |
|---|---|---|---|
| `final_url` | string | always | Final URL after redirects |
| `status_code` | int | always | Final HTTP status |
| `title` | string | optional | Document title |
| `lang` | string | optional | Detected language tag |
| `char_count` | int | always | Character count of rendered output |
| `est_tokens` | int | always | Estimated tokens (chars / 4) |
| `truncated` | bool | always | Output was truncated |
| `protocol` | string | optional | Protocol (e.g. `https/2`) |
| `likely_client_rendered` | bool | always | SPA-skeleton signal: page is probably a client-rendered shell |
| `escalate` | object | optional | Non-blocking escalation hint (see §7a) |

### 7a. Escalation hint (additive, introduced with schema version 1)

When the fetched page is likely a client-rendered shell (SPA skeleton, thin
content, or a JS-challenge page), the result carries an optional, purely
advisory hint object:

```json
{
  "tool": "symbrowse",
  "reason": "spa_skeleton",
  "command": "symbrowse https://example.com"
}
```

- `tool`: always `symbrowse` (the suggested JS-capable re-fetch tool).
- `reason`: `spa_skeleton` (structural SPA shell detected) or `thin_content`
  (thin/link-heavy content; includes JS-challenge pages, which have no
  separate detector).
- `command`: a ready-to-run re-fetch command for the original URL.

The hint NEVER causes the fetch to fail and symfetch never invokes the tool
itself. It is surfaced as:
- an additional blockquote line in markdown output (CLI and MCP default),
- the top-level `escalate` field of the HTTP server envelope (sibling of
  `meta`), mirroring `Meta.escalate`.

The CLI JSON Document output stays contract-pure: the hint is NOT embedded
in the `Document` object (see §2).

## 8. Machine-checkable invariants

Consumers MAY assert:

1. Single JSON output is an object with a `url` string field and a `content`
   array field.
2. Batch JSON output is an array; every element has `url` (string) and `ok`
   (boolean); exactly one of `output` / `error` is present.
3. `meta.status_code` is a 3-digit integer; `meta.est_tokens` is an integer
   `>= 0`.
4. Every field name in this document that is marked `always` is present in
   the corresponding output; `optional` fields appear only when they have a
   value.
5. Error strings start with one of the `[...]` categories in §5, or have no
   bracket prefix.

## 9. Versioning and backward compatibility

- This contract has a `schema_version`. The current version is **1**.
- Additive changes (new optional fields, new error categories, new
  frontmatter keys) are allowed **without** bumping the major version, but
  MUST be recorded here with the version in which they were introduced.
- Removing, renaming, or changing the type of an existing field, or changing
  the meaning of a category prefix, is a **breaking change** and requires a
  major `schema_version` bump coordinated with consumers (symaira-browse#24).
- Existing outputs and tests must stay compatible with this document; any
  deliberate deviation must be documented here before release.
