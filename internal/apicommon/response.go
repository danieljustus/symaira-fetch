// Package apicommon holds error categorisation and response formatting
// shared by the MCP server and the HTTP server, so the two network-facing
// entry points classify errors and shape output identically.
package apicommon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
	"github.com/danieljustus/symaira-fetch/internal/render"
)

// CategoriseError tags a pipeline error with a bracketed category prefix
// (e.g. "[blocked_private]", "[http_4xx]") for consumption by API clients.
func CategoriseError(err error) error {
	if err == nil {
		return nil
	}

	var blockedErr *pipeline.BlockedError
	if errors.As(err, &blockedErr) {
		return fmt.Errorf("[blocked_private] %w", err)
	}

	var tooLargeErr *fetch.ErrTooLarge
	if errors.As(err, &tooLargeErr) {
		return fmt.Errorf("[too_large] %w", err)
	}

	var fetchErr *pipeline.FetchError
	if errors.As(err, &fetchErr) {
		switch {
		case fetchErr.StatusCode >= 400 && fetchErr.StatusCode < 500:
			if fetchErr.Recovery != nil {
				base := fmt.Sprintf("[http_4xx] %%v (nearest reachable ancestor: %s [%d])", fetchErr.Recovery.NearestAncestor, fetchErr.Recovery.AncestorStatus)
				if len(fetchErr.Recovery.Candidates) > 0 {
					candURLs := make([]string, 0, len(fetchErr.Recovery.Candidates))
					for _, cand := range fetchErr.Recovery.Candidates {
						candURLs = append(candURLs, cand.URL)
					}
					base += fmt.Sprintf("; candidates: %s", strings.Join(candURLs, ", "))
				}
				return fmt.Errorf(base, err)
			}
			return fmt.Errorf("[http_4xx] %w", err)
		case fetchErr.StatusCode >= 500:
			return fmt.Errorf("[http_5xx] %w", err)
		}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("[dns] %w", err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("[timeout] %w", err)
	}

	return err
}

// FormatWithMeta renders a pipeline result for the given format, optionally
// prepending YAML frontmatter for Markdown output.
func FormatWithMeta(res *pipeline.Result, format pipeline.Format, frontmatter bool) string {
	if format == pipeline.FormatMarkdown {
		output := res.Output
		if frontmatter && res.Doc != nil {
			fm := render.GenerateFrontmatter(res.Meta, res.Doc)
			output = fm + output
		}
		return render.FormatMarkdownWithMeta(res.Meta, output)
	}
	return res.Output
}

// ValidateURLScheme rejects non-http(s) URLs.
func ValidateURLScheme(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q: only http and https are allowed", u.Scheme)
	}
	return nil
}
