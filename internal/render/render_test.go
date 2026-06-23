package render

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/semantic"
)

// helper: parse an HTML fragment into a *html.Node for Markdown tests.
func parseHTML(t *testing.T, fragment string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	// html.Parse wraps in <html><head><body>; return the <body> node.
	var body *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if body == nil {
		t.Fatal("no <body> found in parsed HTML")
	}
	return body
}

func emptyDoc() *agentdom.Document {
	return &agentdom.Document{
		URL:         "https://example.com",
		Content:     []agentdom.Element{},
		Interactive: []agentdom.Element{},
	}
}

func fullDoc() *agentdom.Document {
	return &agentdom.Document{
		URL:      "https://example.com/page",
		FinalURL: "https://example.com/page",
		Title:    "Example Page",
		Lang:     "en",
		Content: []agentdom.Element{
			{
				Category: semantic.Category("heading"),
				Tag:      "h1",
				Text:     "Welcome",
			},
			{
				Category: semantic.Category("paragraph"),
				Tag:      "p",
				Text:     "This is a test page.",
				Children: []agentdom.Element{
					{
						Category: semantic.Category("link"),
						Tag:      "a",
						Text:     "Click here",
						Attrs:    map[string]string{"href": "https://example.com/target"},
					},
				},
			},
		},
		Interactive: []agentdom.Element{
			{
				AgentID:  "@e1",
				Category: semantic.Category("button"),
				Text:     "Submit",
				Attrs:    map[string]string{"type": "submit"},
			},
			{
				AgentID:  "@e2",
				Category: semantic.Category("link"),
				Text:     "Home",
				Attrs:    map[string]string{"href": "/"},
			},
			{
				AgentID:  "@e3",
				Category: semantic.Category("input"),
				Attrs:    map[string]string{"placeholder": "Search...", "type": "text"},
			},
		},
		Islands: []agentdom.DataIsland{
			{
				Source: "__NEXT_DATA__",
				JSON:   json.RawMessage(`{"props":{"pageProps":{"title":"Hello"}}}`),
			},
		},
	}
}

func TestMarkdown_BasicContent(t *testing.T) {
	doc := emptyDoc()
	node := parseHTML(t, "<p>Hello <strong>world</strong></p>")
	out, err := Markdown(doc, node, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected output to contain 'Hello', got:\n%s", out)
	}
	if !strings.Contains(out, "**world**") {
		t.Errorf("expected bold markdown for 'world', got:\n%s", out)
	}
}

func TestMarkdown_NilContentNode(t *testing.T) {
	doc := emptyDoc()
	out, err := Markdown(doc, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for nil contentNode with no interactive/islands, got: %q", out)
	}
}

func TestMarkdown_EmptyDocument(t *testing.T) {
	doc := emptyDoc()
	node := parseHTML(t, "<p></p>")
	out, err := Markdown(doc, node, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty/whitespace output for empty doc, got: %q", out)
	}
}

func TestMarkdown_InteractiveElements(t *testing.T) {
	doc := fullDoc()
	out, err := Markdown(doc, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "## Interactive Elements") {
		t.Error("expected '## Interactive Elements' header")
	}
	if !strings.Contains(out, "@e1") {
		t.Error("expected @e1 in output")
	}
	if !strings.Contains(out, "Submit") {
		t.Error("expected 'Submit' text in output")
	}
	if !strings.Contains(out, "(submit)") {
		t.Error("expected '(submit)' type attribute in output")
	}
	if !strings.Contains(out, "@e2") {
		t.Error("expected @e2 in output")
	}
	if !strings.Contains(out, "→ /") {
		t.Error("expected '→ /' href for @e2")
	}
	if !strings.Contains(out, "@e3") {
		t.Error("expected @e3 in output")
	}
	if !strings.Contains(out, "[Search...]") {
		t.Error("expected '[Search...]' placeholder for @e3")
	}
}

func TestMarkdown_LinksSection(t *testing.T) {
	doc := fullDoc()
	out, err := Markdown(doc, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "## Links") {
		t.Error("expected '## Links' header when includeLinks=true")
	}
	if !strings.Contains(out, "[Click here](https://example.com/target)") {
		t.Errorf("expected link markdown, got:\n%s", out)
	}
}

func TestMarkdown_LinksSectionDisabled(t *testing.T) {
	doc := fullDoc()
	out, err := Markdown(doc, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "## Links") {
		t.Error("did not expect '## Links' header when includeLinks=false")
	}
}

func TestMarkdown_LinksSkipHashOnly(t *testing.T) {
	doc := &agentdom.Document{
		URL: "https://example.com",
		Content: []agentdom.Element{
			{
				Category: semantic.Category("link"),
				Text:     "Anchor",
				Attrs:    map[string]string{"href": "#section"},
			},
			{
				Category: semantic.Category("link"),
				Text:     "Real",
				Attrs:    map[string]string{"href": "https://example.com/real"},
			},
		},
		Interactive: []agentdom.Element{},
	}
	out, err := Markdown(doc, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "#section") {
		t.Error("hash-only links should be skipped")
	}
	if !strings.Contains(out, "https://example.com/real") {
		t.Error("real links should be included")
	}
}

func TestMarkdown_LinksUseHrefAsTextWhenEmpty(t *testing.T) {
	doc := &agentdom.Document{
		URL: "https://example.com",
		Content: []agentdom.Element{
			{
				Category: semantic.Category("link"),
				Text:     "",
				Attrs:    map[string]string{"href": "https://example.com/no-text"},
			},
		},
		Interactive: []agentdom.Element{},
	}
	out, err := Markdown(doc, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "[https://example.com/no-text](https://example.com/no-text)") {
		t.Errorf("expected href used as text when text is empty, got:\n%s", out)
	}
}

func TestMarkdown_DataIslands(t *testing.T) {
	doc := fullDoc()
	out, err := Markdown(doc, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "## Data") {
		t.Error("expected '## Data' header")
	}
	if !strings.Contains(out, "```json") {
		t.Error("expected json code fence")
	}
	if !strings.Contains(out, "// Source: __NEXT_DATA__") {
		t.Error("expected source comment in data island")
	}
	if !strings.Contains(out, `"props"`) {
		t.Error("expected island JSON content in output")
	}
}

func TestMarkdown_NoInteractiveElements(t *testing.T) {
	doc := &agentdom.Document{
		URL:         "https://example.com",
		Content:     []agentdom.Element{},
		Interactive: []agentdom.Element{},
	}
	out, err := Markdown(doc, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "## Interactive Elements") {
		t.Error("should not emit interactive header when list is empty")
	}
}

func TestMarkdown_NoIslands(t *testing.T) {
	doc := &agentdom.Document{
		URL:         "https://example.com",
		Content:     []agentdom.Element{},
		Interactive: []agentdom.Element{},
		Islands:     []agentdom.DataIsland{},
	}
	out, err := Markdown(doc, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "## Data") {
		t.Error("should not emit data header when islands list is empty")
	}
}

func TestMarkdown_CollectLinksFromChildren(t *testing.T) {
	doc := &agentdom.Document{
		URL: "https://example.com",
		Content: []agentdom.Element{
			{
				Category: semantic.Category("paragraph"),
				Text:     "Parent",
				Children: []agentdom.Element{
					{
						Category: semantic.Category("link"),
						Text:     "Child Link",
						Attrs:    map[string]string{"href": "https://example.com/child"},
					},
				},
			},
		},
		Interactive: []agentdom.Element{},
	}
	out, err := Markdown(doc, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "[Child Link](https://example.com/child)") {
		t.Errorf("expected nested child link in output, got:\n%s", out)
	}
}

func TestJSON_ValidOutput(t *testing.T) {
	doc := fullDoc()
	out, err := JSON(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestJSON_FieldPresence(t *testing.T) {
	doc := fullDoc()
	out, err := JSON(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	requiredFields := []string{"url", "title", "lang", "content", "interactive", "islands"}
	for _, f := range requiredFields {
		if _, ok := parsed[f]; !ok {
			t.Errorf("expected field %q in JSON output", f)
		}
	}

	if parsed["url"] != "https://example.com/page" {
		t.Errorf("expected url 'https://example.com/page', got %v", parsed["url"])
	}
	if parsed["title"] != "Example Page" {
		t.Errorf("expected title 'Example Page', got %v", parsed["title"])
	}
}

func TestJSON_EmptyDocument(t *testing.T) {
	doc := emptyDoc()
	out, err := JSON(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["url"] != "https://example.com" {
		t.Errorf("expected url 'https://example.com', got %v", parsed["url"])
	}
	// Content and Interactive should be empty arrays, not null.
	content, ok := parsed["content"]
	if !ok {
		t.Fatal("expected 'content' field")
	}
	arr, ok := content.([]any)
	if !ok {
		t.Fatalf("expected content to be an array, got %T", content)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty content array, got %d items", len(arr))
	}
}

func TestJSON_PrettyPrinted(t *testing.T) {
	doc := emptyDoc()
	out, err := JSON(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "\n") {
		t.Error("expected pretty-printed JSON with newlines")
	}
	if !strings.Contains(out, "  ") {
		t.Error("expected 2-space indentation in JSON output")
	}
}

func TestJSON_IslandsPresent(t *testing.T) {
	doc := fullDoc()
	out, err := JSON(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	islands, ok := parsed["islands"].([]any)
	if !ok {
		t.Fatalf("expected islands to be an array, got %T", parsed["islands"])
	}
	if len(islands) != 1 {
		t.Fatalf("expected 1 island, got %d", len(islands))
	}
	island := islands[0].(map[string]any)
	if island["source"] != "__NEXT_DATA__" {
		t.Errorf("expected island source '__NEXT_DATA__', got %v", island["source"])
	}
}

func TestJSON_InteractiveElements(t *testing.T) {
	doc := fullDoc()
	out, err := JSON(doc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	interactive, ok := parsed["interactive"].([]any)
	if !ok {
		t.Fatalf("expected interactive to be an array, got %T", parsed["interactive"])
	}
	if len(interactive) != 3 {
		t.Fatalf("expected 3 interactive elements, got %d", len(interactive))
	}
}

func TestText_BasicContent(t *testing.T) {
	doc := fullDoc()
	out := Text(doc)
	if !strings.Contains(out, "Welcome") {
		t.Error("expected 'Welcome' in text output")
	}
	if !strings.Contains(out, "This is a test page.") {
		t.Error("expected paragraph text in output")
	}
}

func TestText_ChildrenIndented(t *testing.T) {
	doc := fullDoc()
	out := Text(doc)
	if !strings.Contains(out, "  Click here") {
		t.Errorf("expected indented child text, got:\n%s", out)
	}
}

func TestText_EmptyDocument(t *testing.T) {
	doc := emptyDoc()
	out := Text(doc)
	if out != "" {
		t.Errorf("expected empty string for empty doc, got: %q", out)
	}
}

func TestText_NoTextElements(t *testing.T) {
	doc := &agentdom.Document{
		URL: "https://example.com",
		Content: []agentdom.Element{
			{
				Category: semantic.Category("image"),
				Tag:      "img",
			},
		},
		Interactive: []agentdom.Element{},
	}
	out := Text(doc)
	if out != "" {
		t.Errorf("expected empty string when elements have no text, got: %q", out)
	}
}

func TestText_ExcludesInteractive(t *testing.T) {
	doc := &agentdom.Document{
		URL:     "https://example.com",
		Content: []agentdom.Element{},
		Interactive: []agentdom.Element{
			{
				AgentID:  "@e1",
				Category: semantic.Category("button"),
				Text:     "Submit",
			},
		},
	}
	out := Text(doc)
	if strings.Contains(out, "Submit") {
		t.Error("Text renderer should not include interactive element text")
	}
}

func TestText_ExcludesIslands(t *testing.T) {
	doc := &agentdom.Document{
		URL:         "https://example.com",
		Content:     []agentdom.Element{},
		Interactive: []agentdom.Element{},
		Islands: []agentdom.DataIsland{
			{
				Source: "test",
				JSON:   json.RawMessage(`{"key":"value"}`),
			},
		},
	}
	out := Text(doc)
	if strings.Contains(out, "key") {
		t.Error("Text renderer should not include island data")
	}
}

func TestText_MultipleElements(t *testing.T) {
	doc := &agentdom.Document{
		URL: "https://example.com",
		Content: []agentdom.Element{
			{Category: semantic.Category("heading"), Text: "Title"},
			{Category: semantic.Category("paragraph"), Text: "Paragraph one."},
			{Category: semantic.Category("paragraph"), Text: "Paragraph two."},
		},
		Interactive: []agentdom.Element{},
	}
	out := Text(doc)
	if !strings.Contains(out, "Title") {
		t.Error("expected 'Title'")
	}
	if !strings.Contains(out, "Paragraph one.") {
		t.Error("expected 'Paragraph one.'")
	}
	if !strings.Contains(out, "Paragraph two.") {
		t.Error("expected 'Paragraph two.'")
	}
}

func TestText_ChildWithNoText(t *testing.T) {
	doc := &agentdom.Document{
		URL: "https://example.com",
		Content: []agentdom.Element{
			{
				Category: semantic.Category("paragraph"),
				Text:     "Parent",
				Children: []agentdom.Element{
					{Category: semantic.Category("image"), Tag: "img"},
				},
			},
		},
		Interactive: []agentdom.Element{},
	}
	out := Text(doc)
	if !strings.Contains(out, "Parent") {
		t.Error("expected parent text")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d: %v", len(lines), lines)
	}
}
