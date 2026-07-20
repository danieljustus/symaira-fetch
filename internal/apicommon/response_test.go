package apicommon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

func TestCategoriseError_Nil(t *testing.T) {
	if got := CategoriseError(nil); got != nil {
		t.Errorf("expected nil, got: %v", got)
	}
}

func TestCategoriseError_BlockedPrivate(t *testing.T) {
	err := &pipeline.BlockedError{URL: "http://127.0.0.1"}
	got := CategoriseError(err)

	if !strings.Contains(got.Error(), "[blocked_private]") {
		t.Errorf("expected [blocked_private] prefix, got: %s", got.Error())
	}
	if !errors.Is(got, err) {
		t.Error("expected wrapped original error")
	}
}

func TestCategoriseError_TooLarge(t *testing.T) {
	err := &pipeline.FetchError{URL: "http://example.com", Err: &fetch.ErrTooLarge{URL: "http://example.com", Limit: 1024}}
	got := CategoriseError(err)

	if !strings.Contains(got.Error(), "[too_large]") {
		t.Errorf("expected [too_large] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP4xx(t *testing.T) {
	err := &pipeline.FetchError{URL: "http://example.com", Err: errors.New("HTTP 404"), StatusCode: 404}
	got := CategoriseError(err)

	if !strings.Contains(got.Error(), "[http_4xx]") {
		t.Errorf("expected [http_4xx] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP4xxWithRecovery(t *testing.T) {
	err := &pipeline.FetchError{
		URL:        "https://example.com/missing",
		Err:        errors.New("HTTP 404 Not Found"),
		StatusCode: 404,
		Recovery: &pipeline.RecoveryHints{
			NearestAncestor: "https://example.com/",
			AncestorStatus:  200,
			Candidates: []pipeline.CandidateURL{
				{URL: "https://example.com/one"},
				{URL: "https://example.com/two"},
			},
		},
	}
	got := CategoriseError(err)

	if !strings.Contains(got.Error(), "nearest reachable ancestor") {
		t.Errorf("expected recovery hint, got: %s", got.Error())
	}
	if !strings.Contains(got.Error(), "https://example.com/one") || !strings.Contains(got.Error(), "https://example.com/two") {
		t.Errorf("expected candidate URLs, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP5xx(t *testing.T) {
	err := &pipeline.FetchError{URL: "http://example.com", Err: errors.New("HTTP 503"), StatusCode: 503}
	got := CategoriseError(err)

	if !strings.Contains(got.Error(), "[http_5xx]") {
		t.Errorf("expected [http_5xx] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_Timeout(t *testing.T) {
	got := CategoriseError(context.DeadlineExceeded)
	if !strings.Contains(got.Error(), "[timeout]") {
		t.Errorf("expected [timeout] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_DNS(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "example.com"}
	got := CategoriseError(err)
	if !strings.Contains(got.Error(), "[dns]") {
		t.Errorf("expected [dns] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_Unknown(t *testing.T) {
	err := errors.New("some other error")
	got := CategoriseError(err)
	if got.Error() != "some other error" {
		t.Errorf("expected original error message, got: %s", got.Error())
	}
	if !errors.Is(got, err) {
		t.Error("expected wrapped original error")
	}
}

func TestFormatWithMeta_Markdown(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello World",
		Meta: agentdom.Meta{
			Title:      "Test Page",
			StatusCode: 200,
			EstTokens:  42,
			FinalURL:   "https://example.com",
			Truncated:  false,
		},
	}

	got := FormatWithMeta(res, pipeline.FormatMarkdown, false)

	if !strings.Contains(got, "> **Test Page** · 200 · ~42 tokens") {
		t.Errorf("expected metadata header, got: %s", got)
	}
	if !strings.Contains(got, "> https://example.com") {
		t.Errorf("expected final URL, got: %s", got)
	}
	if !strings.Contains(got, "# Hello World") {
		t.Errorf("expected output content, got: %s", got)
	}
}

func TestFormatWithMeta_MarkdownTruncated(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Content",
		Meta: agentdom.Meta{
			Title:      "Page",
			StatusCode: 200,
			EstTokens:  100,
			FinalURL:   "https://example.com",
			Truncated:  true,
		},
	}

	got := FormatWithMeta(res, pipeline.FormatMarkdown, false)

	if !strings.Contains(got, "· ⚠ truncated") {
		t.Errorf("expected truncated warning, got: %s", got)
	}
}

func TestFormatWithMeta_JSON(t *testing.T) {
	res := &pipeline.Result{
		Output: `{"key": "value"}`,
		Meta:   agentdom.Meta{Title: "Page"},
	}

	got := FormatWithMeta(res, pipeline.FormatJSON, false)

	if got != `{"key": "value"}` {
		t.Errorf("expected raw output for JSON format, got: %s", got)
	}
}

func TestFormatWithMeta_Text(t *testing.T) {
	res := &pipeline.Result{
		Output: "Plain text content",
		Meta:   agentdom.Meta{Title: "Page"},
	}

	got := FormatWithMeta(res, pipeline.FormatText, false)

	if got != "Plain text content" {
		t.Errorf("expected raw output for text format, got: %s", got)
	}
}

func TestFormatWithMeta_MarkdownFrontmatter(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello World",
		Meta: agentdom.Meta{
			Title:      "Test Page",
			StatusCode: 200,
			EstTokens:  42,
			FinalURL:   "https://example.com",
		},
		Doc: &agentdom.Document{
			URL:     "https://example.com",
			Islands: []agentdom.DataIsland{},
		},
	}

	got := FormatWithMeta(res, pipeline.FormatMarkdown, true)

	if !strings.Contains(got, "---") {
		t.Errorf("expected YAML frontmatter delimiters, got: %s", got)
	}
	if !strings.Contains(got, "title: Test Page") {
		t.Errorf("expected title in frontmatter, got: %s", got)
	}
	if !strings.Contains(got, "# Hello World") {
		t.Errorf("expected output content, got: %s", got)
	}
}

func TestFormatWithMeta_MarkdownFrontmatterWithSchema(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Content",
		Meta: agentdom.Meta{
			Title:      "Recipe Page",
			StatusCode: 200,
			EstTokens:  50,
			FinalURL:   "https://example.com/recipe",
		},
		Doc: &agentdom.Document{
			URL: "https://example.com/recipe",
			Islands: []agentdom.DataIsland{
				{
					Source: "ld+json",
					JSON:   json.RawMessage(`{"@type": "Recipe", "name": "Cake"}`),
				},
			},
		},
	}

	got := FormatWithMeta(res, pipeline.FormatMarkdown, true)

	if !strings.Contains(got, "schema_type: Recipe") {
		t.Errorf("expected schema_type in frontmatter, got: %s", got)
	}
}

func TestFormatWithMeta_NilDoc(t *testing.T) {
	res := &pipeline.Result{
		Output: "# Hello",
		Meta:   agentdom.Meta{Title: "Test"},
		Doc:    nil,
	}

	var panicVal interface{}
	func() {
		defer func() { panicVal = recover() }()
		got := FormatWithMeta(res, pipeline.FormatMarkdown, true)
		if strings.Contains(got, "---") {
			t.Errorf("expected no YAML frontmatter when doc is nil, got: %s", got)
		}
		if !strings.Contains(got, "# Hello") {
			t.Errorf("expected markdown body, got: %s", got)
		}
	}()
	if panicVal != nil {
		t.Fatalf("FormatWithMeta panicked with nil Doc: %v", panicVal)
	}
}

func TestValidateURLScheme_HTTPS(t *testing.T) {
	if err := ValidateURLScheme("https://example.com"); err != nil {
		t.Errorf("expected no error for https, got: %v", err)
	}
}

func TestValidateURLScheme_HTTP(t *testing.T) {
	if err := ValidateURLScheme("http://example.com"); err != nil {
		t.Errorf("expected no error for http, got: %v", err)
	}
}

func TestValidateURLScheme_FTP(t *testing.T) {
	err := ValidateURLScheme("ftp://example.com/file")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got: %v", err)
	}
}

func TestValidateURLScheme_Empty(t *testing.T) {
	if err := ValidateURLScheme(""); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestValidateURLScheme_ParseError(t *testing.T) {
	err := ValidateURLScheme("://invalid\x00")
	if err == nil {
		t.Fatal("expected error for unparseable URL")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Errorf("expected 'invalid URL' in error, got: %s", err.Error())
	}
}
