package agentdom_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
)

func parseHTML(t *testing.T, src string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestBuilderAssignsAgentIDs(t *testing.T) {
	src := `<html><body>
<form action="/submit">
  <input type="text" name="q"/>
  <button type="submit">Go</button>
</form>
</body></html>`
	doc := parseHTML(t, src)
	agDoc := &agentdom.Document{}
	builder := agentdom.NewBuilder(10000)
	builder.Build(doc, agDoc)

	if len(agDoc.Interactive) == 0 {
		t.Fatal("expected interactive elements")
	}
	for _, el := range agDoc.Interactive {
		if el.AgentID == "" {
			t.Errorf("interactive element %q has no AgentID", el.Category)
		}
		if !strings.HasPrefix(el.AgentID, "@e") {
			t.Errorf("expected AgentID prefix @e, got %q", el.AgentID)
		}
	}
}

func TestBuilderAgentIDsAreUnique(t *testing.T) {
	src := `<html><body>
<input type="text" name="a"/>
<input type="text" name="b"/>
<button>X</button>
</body></html>`
	doc := parseHTML(t, src)
	agDoc := &agentdom.Document{}
	agentdom.NewBuilder(10000).Build(doc, agDoc)

	seen := map[string]bool{}
	for _, el := range agDoc.Interactive {
		if seen[el.AgentID] {
			t.Errorf("duplicate AgentID %q", el.AgentID)
		}
		seen[el.AgentID] = true
	}
}

func TestBuilderTruncatesAtRuneBoundary(t *testing.T) {
	// Japanese characters — 3 bytes each in UTF-8
	content := strings.Repeat("あ", 100)
	src := "<html><body><p>" + content + "</p></body></html>"
	doc := parseHTML(t, src)

	maxChars := 30
	agDoc := &agentdom.Document{}
	agentdom.NewBuilder(maxChars).Build(doc, agDoc)

	total := 0
	for _, el := range agDoc.Content {
		total += utf8.RuneCountInString(el.Text)
	}

	// Should not exceed maxChars (plus 1 for the ellipsis)
	if total > maxChars+1 {
		t.Errorf("expected at most %d runes, got %d", maxChars+1, total)
	}
	// Verify the result is valid UTF-8
	for _, el := range agDoc.Content {
		if !utf8.ValidString(el.Text) {
			t.Errorf("element text is invalid UTF-8: %q", el.Text)
		}
	}
}

func TestBuilderTruncatedFlagSet(t *testing.T) {
	content := strings.Repeat("x", 1000)
	src := "<html><body><p>" + content + "</p></body></html>"
	doc := parseHTML(t, src)

	agDoc := &agentdom.Document{}
	agentdom.NewBuilder(50).Build(doc, agDoc)

	// At least one element should have truncated text (ending with ellipsis)
	hasTruncation := false
	for _, el := range agDoc.Content {
		if strings.HasSuffix(el.Text, "…") {
			hasTruncation = true
			break
		}
	}
	if !hasTruncation {
		t.Error("expected truncated text to end with ellipsis")
	}
}
