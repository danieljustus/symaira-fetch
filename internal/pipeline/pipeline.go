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
	Format   Format
	Content  ContentOptions
	Cache    CacheOptions
	Profile  string
	Session  string
	Security SecurityOptions
}

// ContentOptions controls content extraction limits and scoring.
type ContentOptions struct {
	MaxChars       int // character budget for content output
	IncludeLinks   bool
	CharThreshold  int // minimum chars for content scoring; below this triggers retry
	MaxIslandBytes int // max size of a single data island
}

// CacheOptions controls response caching.
type CacheOptions struct {
	NoCache  bool
	Dir      string
	TTL      time.Duration
	MaxSize  int64 // max cache size in bytes; 0 uses default (100 MB)
	Instance *cache.Cache // shared cache instance; when nil, per-call cache is created
}

// SecurityOptions controls SSRF protection and robots.txt compliance.
type SecurityOptions struct {
	AllowPrivate  bool
	Robots        bool
	RobotsChecker *robots.Checker
}

func (o *Options) setDefaults() {
	if o.Content.MaxChars <= 0 {
		o.Content.MaxChars = 20000
	}
	if o.Content.CharThreshold <= 0 {
		o.Content.CharThreshold = 500
	}
	if o.Content.MaxIslandBytes <= 0 {
		o.Content.MaxIslandBytes = o.Content.MaxChars / 4
	}
}

// ContentKey returns a deterministic string encoding every option that
// affects the rendered output so the cache can distinguish requests that
// would produce different results.
func (o *ContentOptions) ContentKey() string {
	return fmt.Sprintf("mc=%d il=%v ct=%d mi=%d", o.MaxChars, o.IncludeLinks, o.CharThreshold, o.MaxIslandBytes)
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

	if !o.Security.AllowPrivate {
		if err := fetch.CheckSSRF(rawURL); err != nil {
			return nil, err
		}
	}

	var cacher *cache.Cache
	if !o.Cache.NoCache {
		if o.Cache.Instance != nil {
			cacher = o.Cache.Instance
		} else {
			dir := o.Cache.Dir
			if dir == "" {
				dir = cache.DefaultDir()
			}
			ttl := o.Cache.TTL
			if ttl <= 0 {
				ttl = 24 * time.Hour
			}
			cacher = cache.New(dir, ttl, o.Cache.MaxSize)
		}
	}

	if cacher != nil {
		profile := o.Profile
		if profile == "" {
			profile = "chrome"
		}
		ck := o.Content.ContentKey()
		if body, meta, ok := cacher.Get(rawURL, profile, string(o.Format), o.Session, ck); ok {
			if !o.Security.AllowPrivate && meta.FinalURL != "" && meta.FinalURL != rawURL {
				if err := fetch.CheckSSRF(meta.FinalURL); err != nil {
					slog.Debug("cache hit blocked by SSRF (redirect target)", "url", rawURL, "finalURL", meta.FinalURL)
					cacher = nil
				}
			}
			if cacher != nil {
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
	}

	if o.Security.Robots && o.Security.RobotsChecker != nil {
		allowed, err := o.Security.RobotsChecker.Check(ctx, "symfetch", rawURL)
		if err != nil {
			slog.Debug("robots check error", "url", rawURL, "error", err)
		} else if !allowed {
			return nil, &BlockedError{URL: rawURL, Reason: "disallowed by robots.txt"}
		}
	}

	resp, err := c.Fetch(ctx, fetch.Request{
		URL:          rawURL,
		AllowPrivate: o.Security.AllowPrivate,
		Session:      o.Session,
	})
	if err != nil {
		return nil, &FetchError{URL: rawURL, Err: err}
	}

	if resp.StatusCode >= 400 {
		return nil, &FetchError{URL: rawURL, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	tree, err := eng.Materialize(ctx, resp)
	if err != nil {
		return nil, &ParseError{URL: rawURL, Err: err}
	}

	// 4. Extract data islands BEFORE filtering (islands are in <script> tags)
	rawIslands := semantic.ExtractIslands(tree.Root, o.Content.MaxIslandBytes)

	// 5. Filter DOM
	dom.Filter(tree.Root)

	// 6. Score and pick best block
	bestNode := semantic.BestBlock(tree.Root, o.Content.CharThreshold)

	// 7. Build agentdom
	doc := &agentdom.Document{
		URL:      rawURL,
		FinalURL: resp.FinalURL,
		Title:    tree.Title,
		Lang:     tree.Lang,
	}

	builder := agentdom.NewBuilder(o.Content.MaxChars)
	builder.Build(bestNode, doc)

	// Convert islands
	for _, island := range rawIslands {
		doc.Islands = append(doc.Islands, agentdom.DataIsland{
			Source: island.Source,
			JSON:   island.JSON,
		})
	}

	var output string
	switch o.Format {
	case FormatJSON:
		output, err = render.JSON(doc)
		if err != nil {
			return nil, &RenderError{Format: "json", Err: err}
		}
	case FormatText:
		output = render.Text(doc)
	case FormatHTML:
		output = rawHTMLFallback(resp.Body)
	default:
		output, err = render.Markdown(doc, bestNode, o.Content.IncludeLinks)
		if err != nil {
			return nil, &RenderError{Format: "markdown", Err: err}
		}
	}

	charCount := utf8.RuneCountInString(output)
	truncated := charCount >= o.Content.MaxChars

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
		ck := o.Content.ContentKey()
		if err := cacher.Put(rawURL, profile, string(o.Format), o.Session, ck, []byte(output), cache.Meta{
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
