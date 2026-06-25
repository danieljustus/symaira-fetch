package pipeline

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseHTMLNode(t *testing.T, h string) *html.Node {
	t.Helper()
	n, err := html.Parse(strings.NewReader(h))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIsThinContent_TriggersOnNavShell(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<a href="/home">Home</a>
		<a href="/about">About</a>
		<a href="/contact">Contact</a>
		<a href="/blog">Blog</a>
		<a href="/docs">Docs</a>
	</body>`)
	if !isThinContent(node, 500, false) {
		t.Error("expected thin content for nav-heavy page")
	}
}

func TestIsThinContent_DoesNotTriggerOnRichContent(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<article>
			<h1>Introduction to Go Concurrency</h1>
			<p>Go provides powerful concurrency primitives through goroutines and channels. This article explores how to use them effectively in production systems.</p>
			<p>Goroutines are lightweight threads managed by the Go runtime. Unlike OS threads, they start with just a few kilobytes of stack, making it practical to spawn hundreds of thousands concurrently.</p>
			<p>Channels are typed conduits for communication between goroutines. They provide synchronization and data passing in a single construct, eliminating the need for locks in many cases.</p>
			<p>The select statement lets a goroutine wait on multiple channel operations, enabling complex coordination patterns without explicit synchronization primitives.</p>
			<p>In practice, most concurrent Go programs follow a few well-established patterns: fan-out/fan-in for parallel processing, pipeline for streaming data, and worker pools for bounded concurrency.</p>
		</article>
	</body>`)
	if isThinContent(node, 500, false) {
		t.Error("expected rich content to NOT trigger thin-content fallback")
	}
}

func TestIsThinContent_TriggerOnZeroText(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<div id="app"></div>
	</body>`)
	if !isThinContent(node, 500, false) {
		t.Error("expected thin content for empty body")
	}
}

func TestIsThinContent_ShortButMostlyText(t *testing.T) {
	node := parseHTMLNode(t, `<body><p>Hello world, short page.</p></body>`)
	if isThinContent(node, 500, false) {
		t.Error("short page with low link density should NOT be thin")
	}
}

func TestIsThinContent_BelowThresholdHighLinkDensity(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<p>Some text here.</p>
		<a href="/link1">link1 text is longer</a>
		<a href="/link2">link2 text is longer</a>
		<a href="/link3">link3 text is longer</a>
	</body>`)
	if !isThinContent(node, 500, false) {
		t.Error("expected thin content: below threshold + high link density")
	}
}

func TestIsThinContent_SPASkeletonTrigger(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<p>Some text here.</p>
		<a href="/link1">link text</a>
	</body>`)
	if isThinContent(node, 500, false) {
		t.Error("low link density without SPA signal should NOT be thin")
	}
	if !isThinContent(node, 500, true) {
		t.Error("SPA skeleton signal should trigger thin content when below threshold")
	}
}

func TestIsThinContent_SPASkeletonDoesNotOverrideRichContent(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<article>
			<h1>Go Concurrency Patterns</h1>
			<p>Go provides powerful concurrency primitives through goroutines and channels. This article explores how to use them effectively in production systems, covering best practices and common pitfalls.</p>
			<p>Goroutines are lightweight threads managed by the Go runtime. Unlike OS threads, they start with just a few kilobytes of stack, making it practical to spawn hundreds of thousands concurrently.</p>
			<p>Channels are typed conduits for communication between goroutines. They provide synchronization and data passing in a single construct, eliminating the need for locks in many cases.</p>
			<p>The select statement lets a goroutine wait on multiple channel operations, enabling complex coordination patterns without explicit synchronization primitives.</p>
			<p>In practice, most concurrent Go programs follow a few well-established patterns: fan-out/fan-in for parallel processing, pipeline for streaming data, and worker pools for bounded concurrency.</p>
		</article>
	</body>`)
	if isThinContent(node, 500, true) {
		t.Error("SPA skeleton signal should NOT override rich content above threshold")
	}
}

func TestDeriveMDTwinURL_BasicPath(t *testing.T) {
	got := deriveMDTwinURL("https://example.com/docs/guide")
	want := "https://example.com/docs/guide.md"
	if got != want {
		t.Errorf("deriveMDTwinURL(%q) = %q, want %q", "https://example.com/docs/guide", got, want)
	}
}

func TestDeriveMDTwinURL_WithQueryAndFragment(t *testing.T) {
	got := deriveMDTwinURL("https://example.com/docs/guide?tab=overview#section-2")
	want := "https://example.com/docs/guide.md?tab=overview#section-2"
	if got != want {
		t.Errorf("deriveMDTwinURL(%q) = %q, want %q", "https://example.com/docs/guide?tab=overview#section-2", got, want)
	}
}

func TestDeriveMDTwinURL_RootPath(t *testing.T) {
	got := deriveMDTwinURL("https://example.com/")
	want := "https://example.com/.md"
	if got != want {
		t.Errorf("deriveMDTwinURL(%q) = %q, want %q", "https://example.com/", got, want)
	}
}

func TestDeriveMDTwinURL_InvalidURL(t *testing.T) {
	got := deriveMDTwinURL("://bad-url")
	if got != "" {
		t.Errorf("expected empty string for invalid URL, got %q", got)
	}
}

func TestDeriveLLMsTxtURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/docs/guide", "https://example.com/llms.txt"},
		{"https://example.com/docs/guide?tab=1", "https://example.com/llms.txt"},
		{"https://example.com/docs/guide#section", "https://example.com/llms.txt"},
		{"https://example.com/", "https://example.com/llms.txt"},
	}
	for _, tt := range tests {
		got := deriveLLMsTxtURL(tt.input)
		if got != tt.want {
			t.Errorf("deriveLLMsTxtURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDeriveLLMsTxtURL_InvalidURL(t *testing.T) {
	got := deriveLLMsTxtURL("://bad-url")
	if got != "" {
		t.Errorf("expected empty string for invalid URL, got %q", got)
	}
}

func TestCountTextContent(t *testing.T) {
	node := parseHTMLNode(t, `<body><p>Hello World</p></body>`)
	got := countTextContent(node)
	if got != 11 {
		t.Errorf("countTextContent = %d, want 11", got)
	}
}

func TestCountLinkContent(t *testing.T) {
	node := parseHTMLNode(t, `<body>
		<p>Plain text</p>
		<a href="/foo">Link text here</a>
	</body>`)
	got := countLinkContent(node)
	if got != 14 {
		t.Errorf("countLinkContent = %d, want 14", got)
	}
}
