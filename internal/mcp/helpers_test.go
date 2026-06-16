package mcp

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

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

	got := formatWithMeta(res, pipeline.FormatMarkdown)

	if !strings.Contains(got, "> **Test Page** · 200 · ~42 tokens") {
		t.Errorf("expected metadata header, got: %s", got)
	}
	if !strings.Contains(got, "> https://example.com") {
		t.Errorf("expected final URL, got: %s", got)
	}
	if !strings.Contains(got, "# Hello World") {
		t.Errorf("expected output content, got: %s", got)
	}
	if strings.Contains(got, "truncated") {
		t.Errorf("should not contain truncated warning, got: %s", got)
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

	got := formatWithMeta(res, pipeline.FormatMarkdown)

	if !strings.Contains(got, "· ⚠ truncated") {
		t.Errorf("expected truncated warning, got: %s", got)
	}
}

func TestFormatWithMeta_JSON(t *testing.T) {
	res := &pipeline.Result{
		Output: `{"key": "value"}`,
		Meta:   agentdom.Meta{Title: "Page"},
	}

	got := formatWithMeta(res, pipeline.FormatJSON)

	if got != `{"key": "value"}` {
		t.Errorf("expected raw output for JSON format, got: %s", got)
	}
}

func TestFormatWithMeta_Text(t *testing.T) {
	res := &pipeline.Result{
		Output: "Plain text content",
		Meta:   agentdom.Meta{Title: "Page"},
	}

	got := formatWithMeta(res, pipeline.FormatText)

	if got != "Plain text content" {
		t.Errorf("expected raw output for text format, got: %s", got)
	}
}

func TestCategoriseError_Nil(t *testing.T) {
	got := categoriseError(nil)
	if got != nil {
		t.Errorf("expected nil, got: %v", got)
	}
}

func TestCategoriseError_BlockedPrivate(t *testing.T) {
	err := errors.New("blocked_private: address is private")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[blocked_private]") {
		t.Errorf("expected [blocked_private] prefix, got: %s", got.Error())
	}
	if !errors.Is(got, err) {
		t.Error("expected wrapped original error")
	}
}

func TestCategoriseError_TooLarge(t *testing.T) {
	err := errors.New("too_large: body exceeds limit")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[too_large]") {
		t.Errorf("expected [too_large] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP4xx(t *testing.T) {
	err := errors.New("http_404: not found")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[http_4xx]") {
		t.Errorf("expected [http_4xx] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_HTTP5xx(t *testing.T) {
	err := errors.New("http_503: service unavailable")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[http_5xx]") {
		t.Errorf("expected [http_5xx] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_Timeout(t *testing.T) {
	err := errors.New("timeout: context deadline exceeded")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[timeout]") {
		t.Errorf("expected [timeout] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_Deadline(t *testing.T) {
	err := errors.New("deadline exceeded")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[timeout]") {
		t.Errorf("expected [timeout] prefix for deadline error, got: %s", got.Error())
	}
}

func TestCategoriseError_DNS(t *testing.T) {
	err := errors.New("no such host")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[dns]") {
		t.Errorf("expected [dns] prefix, got: %s", got.Error())
	}
}

func TestCategoriseError_DNSError(t *testing.T) {
	err := errors.New("dns: lookup failed")
	got := categoriseError(err)

	if !strings.Contains(got.Error(), "[dns]") {
		t.Errorf("expected [dns] prefix for dns error, got: %s", got.Error())
	}
}

func TestCategoriseError_Unknown(t *testing.T) {
	err := fmt.Errorf("some other error")
	got := categoriseError(err)

	if got.Error() != "some other error" {
		t.Errorf("expected original error message, got: %s", got.Error())
	}
	if !errors.Is(got, err) {
		t.Error("expected wrapped original error")
	}
}
