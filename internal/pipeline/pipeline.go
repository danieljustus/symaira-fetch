package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danieljustus/symaira-fetch/internal/agentdom"
	"github.com/danieljustus/symaira-fetch/internal/cache"
	"github.com/danieljustus/symaira-fetch/internal/dom"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/render"
	"github.com/danieljustus/symaira-fetch/internal/robots"
	"github.com/danieljustus/symaira-fetch/internal/semantic"
)

// Format is the output format for the rendered result.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
	FormatText     Format = "text"
	FormatHTML     Format = "html"
)

// ParseFormat parses a string into a Format.
func ParseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	case "text":
		return FormatText
	case "html":
		return FormatHTML
	default:
		return FormatMarkdown
	}
}

// Options configures the pipeline run.
type Options struct {
	Format         Format
	MaxChars       int  // character budget for content output
	IncludeLinks   bool
	CharThreshold  int  // minimum chars for content scoring; below this triggers retry
	MaxIslandBytes int  // max size of a single data island
	AllowPrivate   bool

	NoCache  bool
	CacheDir string
	CacheTTL time.Duration
	Profile  string
	Session  string

	Robots        bool
	RobotsChecker *robots.Checker
}

func (o *Options) setDefaults() {
	if o.MaxChars <= 0 {
		o.MaxChars = 20000
	}
	if o.CharThreshold <= 0 {
		o.CharThreshold = 500
	}
	if o.MaxIslandBytes <= 0 {
		o.MaxIslandBytes = o.MaxChars / 4
	}
}

// Result holds the pipeline output.
type Result struct {
	Doc    *agentdom.Document
	Output string
	Meta   agentdom.Meta
}

// Run executes the full semantic pipeline:
// fetch → materialize → filter → score → classify → agentdom → render.
func Run(ctx context.Context, c fetch.Client, eng Engine, rawURL string, o Options) (*Result, error) {
	o.setDefaults()

	var cacher *cache.Cache
	if !o.NoCache {
		dir := o.CacheDir
		if dir == "" {
			dir = cache.DefaultDir()
		}
		ttl := o.CacheTTL
		if ttl <= 0 {
			ttl = 24 * time.Hour
		}
		cacher = cache.New(dir, ttl)

		profile := o.Profile
		if profile == "" {
			profile = "chrome"
		}
		if body, meta, ok := cacher.Get(rawURL, profile); ok {
			slog.Debug("cache hit", "url", rawURL)
			return &Result{
				Output: string(body),
				Meta: agentdom.Meta{
					FinalURL:   meta.FinalURL,
					StatusCode: meta.StatusCode,
					Protocol:   meta.Protocol,
				},
			}, nil
		}
	}

	// 1. Robots check
	if o.Robots && o.RobotsChecker != nil {
		allowed, err := o.RobotsChecker.Check(ctx, "symfetch", rawURL)
		if err != nil {
			slog.Debug("robots check error", "url", rawURL, "error", err)
		} else if !allowed {
			return nil, fmt.Errorf("robots: disallowed by robots.txt: %s", rawURL)
		}
	}

	// 2. Fetch
	resp, err := c.Fetch(ctx, fetch.Request{
		URL:          rawURL,
		AllowPrivate: o.AllowPrivate,
		Session:      o.Session,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http_%d: server returned %d for %s", resp.StatusCode, resp.StatusCode, rawURL)
	}

	// 3. Materialize (parse DOM)
	tree, err := eng.Materialize(ctx, resp)
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}

	// 4. Extract data islands BEFORE filtering (islands are in <script> tags)
	rawIslands := semantic.ExtractIslands(tree.Root, o.MaxIslandBytes)

	// 5. Filter DOM
	dom.Filter(tree.Root)

	// 6. Score and pick best block
	bestNode := semantic.BestBlock(tree.Root, o.CharThreshold)

	// 7. Build agentdom
	doc := &agentdom.Document{
		URL:      rawURL,
		FinalURL: resp.FinalURL,
		Title:    tree.Title,
		Lang:     tree.Lang,
	}

	builder := agentdom.NewBuilder(o.MaxChars)
	builder.Build(bestNode, doc)

	// Convert islands
	for _, island := range rawIslands {
		doc.Islands = append(doc.Islands, agentdom.DataIsland{
			Source: island.Source,
			JSON:   island.JSON,
		})
	}

	// 8. Render
	var output string
	switch o.Format {
	case FormatJSON:
		output, err = render.JSON(doc)
		if err != nil {
			return nil, fmt.Errorf("render json: %w", err)
		}
	case FormatText:
		output = render.Text(doc)
	case FormatHTML:
		output = rawHTMLFallback(resp.Body)
	default: // FormatMarkdown
		output, err = render.Markdown(doc, bestNode, o.IncludeLinks)
		if err != nil {
			return nil, fmt.Errorf("render markdown: %w", err)
		}
	}

	charCount := utf8.RuneCountInString(output)
	truncated := charCount >= o.MaxChars

	meta := agentdom.Meta{
		FinalURL:   resp.FinalURL,
		StatusCode: resp.StatusCode,
		Title:      tree.Title,
		Lang:       tree.Lang,
		CharCount:  charCount,
		EstTokens:  charCount / 4,
		Truncated:  truncated,
		Protocol:   resp.Protocol,
	}

	if cacher != nil {
		profile := o.Profile
		if profile == "" {
			profile = "chrome"
		}
		if err := cacher.Put(rawURL, profile, []byte(output), cache.Meta{
			URL:         rawURL,
			FinalURL:    resp.FinalURL,
			StatusCode:  resp.StatusCode,
			ContentType: resp.ContentType,
			Protocol:    resp.Protocol,
			Headers:     resp.Headers,
		}); err != nil {
			slog.Debug("cache put failed", "url", rawURL, "error", err)
		}
	}

	return &Result{Doc: doc, Output: output, Meta: meta}, nil
}

// RunRaw fetches a URL and returns the raw decoded body without any pipeline processing.
func RunRaw(ctx context.Context, c fetch.Client, rawURL string, req fetch.Request) (*fetch.Response, error) {
	req.URL = rawURL
	return c.Fetch(ctx, req)
}

func rawHTMLFallback(body []byte) string {
	return string(body)
}

// IslandSummary renders a short summary of data islands for Markdown mode.
func IslandSummary(islands []agentdom.DataIsland) string {
	if len(islands) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, island := range islands {
		var preview interface{}
		if err := json.Unmarshal(island.JSON, &preview); err == nil {
			if m, ok := preview.(map[string]interface{}); ok {
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sb.WriteString(fmt.Sprintf("- **%s**: keys=%v\n", island.Source, keys))
				continue
			}
		}
		sb.WriteString(fmt.Sprintf("- **%s**: (raw JSON, %d bytes)\n", island.Source, len(island.JSON)))
	}
	return sb.String()
}
