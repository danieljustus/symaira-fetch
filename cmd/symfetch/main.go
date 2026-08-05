package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/logkit"
	"github.com/danieljustus/symaira-corekit/updatecheck"
	"github.com/danieljustus/symaira-corekit/versionkit"
	"github.com/danieljustus/symaira-fetch/internal/apicommon"
	"github.com/danieljustus/symaira-fetch/internal/batch"
	"github.com/danieljustus/symaira-fetch/internal/config"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/httpserver"
	"github.com/danieljustus/symaira-fetch/internal/mcp"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
	"github.com/danieljustus/symaira-fetch/internal/robots"
)

var version = "0.1.0-dev"

func main() {
	slog.SetDefault(logkit.NewFromEnv("symfetch"))

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(int(exitcodes.ExitCodeFromError(err)))
	}
}

func newRootCmd() *cobra.Command {
	var (
		flagFormat        string
		flagRaw           bool
		flagProfile       string
		flagProxy         string
		flagTimeout       string
		flagMaxChars      int
		flagLinks         bool
		flagSession       string
		flagNoCache       bool
		flagCacheTTL      string
		flagHeaders       []string
		flagMethod        string
		flagData          string
		flagConcurrency   int
		flagAllowPriv     bool
		flagRobots        bool
		flagNoRetry       bool
		flagStoreFullText bool
		flagCharLimit     int
		flagWaybackAt     string
		flagWaybackFB     bool
		flagQuery         string
		flagTopK          int
		flagSelector      string
		flagFrontmatter   bool
		flagSchemaPath    string
	)

	root := &cobra.Command{
		Use:     "symfetch [url...]",
		Short:   "AI-native web fetch engine for LLM agents",
		Version: version,
		Long: `symfetch fetches web pages using browser-impersonating TLS and
returns LLM-optimized Markdown, JSON, or plain text.

Multiple URLs are fetched sequentially; output is separated by --- delimiters
in Markdown mode, or as a JSON array in --format json mode.`,
		Args:          cobra.MinimumNArgs(0),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: config error: %v\n", err)
				cfg = config.Defaults()
			}

			ro, err := resolveRootOptions(cmd, cfg)
			if err != nil {
				return err
			}
			defer ro.client.Close()

			ctx := context.Background()
			eng := pipeline.StaticEngine{}

			if ro.opts.Security.AllowPrivate {
				fmt.Fprintf(os.Stderr, "warning: SSRF guard disabled — fetching private/loopback addresses is permitted\n")
			}

			if flagRaw {
				return runRaw(ctx, os.Stdout, ro.client, args, flagMethod, ro.extraHeaders, flagData, ro.allowPrivate)
			}

			if ro.opts.Format == pipeline.FormatJSON && len(args) > 1 {
				return runMultiJSON(ctx, os.Stdout, ro.client, eng, args, ro.opts)
			}

			if len(args) > 1 && ro.fetchOpts.concurrency > 1 {
				return runBatch(ctx, os.Stdout, ro.client, eng, args, ro.opts, ro.fetchOpts.concurrency)
			}

			return runSequential(ctx, os.Stdout, ro.client, eng, args, ro.opts, flagFrontmatter)
		},
	}

	root.Flags().StringVarP(&flagFormat, "format", "f", "markdown", "Output format: markdown, json, text, html")
	root.Flags().BoolVar(&flagRaw, "raw", false, "Return raw decoded response body without semantic processing")
	root.Flags().StringVar(&flagProfile, "profile", "chrome", "Browser profile: chrome, firefox, opera, safari, edge, ios, honest, random")
	root.Flags().StringVar(&flagProxy, "proxy", "", "Proxy URL (http/https/socks5)")
	root.Flags().StringVar(&flagTimeout, "timeout", "30s", "Request timeout (e.g. 30s, 1m)")
	root.Flags().IntVar(&flagMaxChars, "max-chars", 20000, "Maximum characters in semantic output")
	root.Flags().BoolVar(&flagLinks, "links", false, "Append Links section with all hrefs")
	root.Flags().StringVar(&flagSession, "session", "", "Named persistent cookie jar")
	root.Flags().BoolVar(&flagNoCache, "no-cache", false, "Disable response caching")
	root.Flags().StringVar(&flagCacheTTL, "cache-ttl", "15m", "Cache TTL (e.g. 15m, 1h)")
	root.Flags().StringArrayVarP(&flagHeaders, "header", "H", nil, "Extra request header (\"Key: Value\")")
	root.Flags().StringVarP(&flagMethod, "request", "X", "GET", "HTTP method")
	root.Flags().StringVar(&flagData, "data", "", "Request body data")
	root.Flags().IntVar(&flagConcurrency, "concurrency", 4, "Parallel fetch workers for multiple URLs")
	root.Flags().BoolVar(&flagAllowPriv, "allow-private", false, "Allow fetching private/loopback addresses")
	root.Flags().BoolVar(&flagRobots, "robots", false, "Check robots.txt before fetching")
	root.Flags().BoolVar(&flagNoRetry, "no-retry", false, "Disable automatic retry on transient errors")
	root.Flags().BoolVar(&flagStoreFullText, "store-full-text", false, "Enable truncate-and-store for long pages (Hermes-style)")
	root.Flags().IntVar(&flagCharLimit, "char-limit", 15000, "Per-page character limit for truncate-and-store")
	root.Flags().StringVar(&flagWaybackAt, "at", "", "Fetch from Wayback Machine at timestamp (YYYYMMDDHHmmss)")
	root.Flags().BoolVar(&flagWaybackFB, "wayback-fallback", false, "Enable Wayback Machine as fallback on 404/thin-content")
	root.Flags().StringVar(&flagQuery, "query", "", "BM25 query for relevance filtering (returns only matching sections)")
	root.Flags().IntVar(&flagTopK, "top-k", 0, "Number of top sections to return for relevance filtering (0 = all)")
	root.Flags().StringVar(&flagSelector, "selector", "", "CSS selector to extract specific elements (e.g., 'table.pricing', '.article-body')")
	root.Flags().BoolVar(&flagFrontmatter, "frontmatter", false, "Prepend YAML frontmatter with metadata (title, url, fetched_at, lang, tokens)")
	root.Flags().StringVar(&flagSchemaPath, "schema-path", "", "JSON-LD query path (e.g., '@Recipe:name', '@Product:aggregateRating.ratingValue')")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newSnapshotsCmd())

	return root
}

type fetchOptions struct {
	noCache     bool
	cacheTTL    time.Duration
	concurrency int
}

func resolveFetchOptions(cmd *cobra.Command, cfg *config.Config) (fetchOptions, error) {
	noCache := !cfg.Cache.Enabled
	if cmd.Flags().Changed("no-cache") {
		v, _ := cmd.Flags().GetBool("no-cache")
		noCache = v
	}

	cacheTTL := cfg.Cache.TTL
	if cmd.Flags().Changed("cache-ttl") {
		v, _ := cmd.Flags().GetString("cache-ttl")
		d, err := time.ParseDuration(v)
		if err != nil {
			return fetchOptions{}, exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "invalid cache-ttl")
		}
		cacheTTL = d
	}

	concurrency := cfg.HTTP.Concurrency
	if cmd.Flags().Changed("concurrency") {
		v, _ := cmd.Flags().GetInt("concurrency")
		concurrency = v
	}

	return fetchOptions{
		noCache:     noCache,
		cacheTTL:    cacheTTL,
		concurrency: concurrency,
	}, nil
}

// rootOptions carries every resolved value the fetch command needs to branch
// into its output runners.
type rootOptions struct {
	profile      string
	proxy        string
	format       pipeline.Format
	maxChars     int
	timeoutSec   int
	extraHeaders map[string]string
	allowPrivate bool
	client       fetch.Client
	opts         pipeline.Options
	fetchOpts    fetchOptions
}

// resolveRootOptions resolves the fetch command's config+flags into the
// pieces its output runners need: profile/proxy/format/max-chars/timeout,
// extra headers, the fetch client, and the assembled pipeline options.
// Flags win over the config file, mirroring resolveMCPOptions. Warnings are
// written to os.Stderr in the same order as the original RunE closure.
func resolveRootOptions(cmd *cobra.Command, cfg *config.Config) (*rootOptions, error) {
	ro := &rootOptions{}

	ro.profile = cfg.HTTP.Profile
	if cmd.Flags().Changed("profile") {
		ro.profile, _ = cmd.Flags().GetString("profile")
	}
	ro.proxy = cfg.HTTP.Proxy
	if cmd.Flags().Changed("proxy") {
		ro.proxy, _ = cmd.Flags().GetString("proxy")
	}
	format := cfg.HTTP.DefaultFormat
	if cmd.Flags().Changed("format") {
		format, _ = cmd.Flags().GetString("format")
	}
	parsedFormat, err := pipeline.ParseFormat(format)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "invalid format")
	}
	ro.format = parsedFormat
	ro.maxChars = cfg.HTTP.MaxChars
	if cmd.Flags().Changed("max-chars") {
		ro.maxChars, _ = cmd.Flags().GetInt("max-chars")
	}

	fo, err := resolveFetchOptions(cmd, cfg)
	if err != nil {
		return nil, err
	}
	ro.fetchOpts = fo

	ro.timeoutSec = cfg.HTTP.TimeoutSeconds
	if cmd.Flags().Changed("timeout") {
		v, _ := cmd.Flags().GetString("timeout")
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "invalid timeout")
		}
		ro.timeoutSec = int(d.Seconds())
	}
	if ro.timeoutSec > 120 {
		fmt.Fprintf(os.Stderr, "warning: timeout %ds exceeds MCP server cap of 120s; MCP requests will be capped\n", ro.timeoutSec)
	}

	headers, _ := cmd.Flags().GetStringArray("header")
	ro.extraHeaders, err = parseHeaders(headers)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "invalid header")
	}

	noRetry, _ := cmd.Flags().GetBool("no-retry")
	p := fetch.ParseProfile(ro.profile)
	ro.client, err = fetch.New(p,
		fetch.WithProxy(ro.proxy),
		fetch.WithTimeout(ro.timeoutSec),
		fetch.WithMaxBody(cfg.HTTP.MaxBodyMB),
		fetch.WithRetry(!noRetry),
	)
	if err != nil {
		return nil, exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "init client")
	}

	data, _ := cmd.Flags().GetString("data")
	body := []byte(nil)
	if data != "" {
		body = []byte(data)
	}
	links, _ := cmd.Flags().GetBool("links")
	session, _ := cmd.Flags().GetString("session")
	robotsEnabled, _ := cmd.Flags().GetBool("robots")
	method, _ := cmd.Flags().GetString("request")
	storeFullText, _ := cmd.Flags().GetBool("store-full-text")
	charLimit, _ := cmd.Flags().GetInt("char-limit")
	waybackFB, _ := cmd.Flags().GetBool("wayback-fallback")
	waybackAt, _ := cmd.Flags().GetString("at")
	query, _ := cmd.Flags().GetString("query")
	topK, _ := cmd.Flags().GetInt("top-k")
	selector, _ := cmd.Flags().GetString("selector")
	frontmatter, _ := cmd.Flags().GetBool("frontmatter")
	schemaPath, _ := cmd.Flags().GetString("schema-path")
	allowPrivateFlag, _ := cmd.Flags().GetBool("allow-private")

	ro.opts = pipeline.Options{
		Format: ro.format,
		Content: pipeline.ContentOptions{
			MaxChars:     ro.maxChars,
			IncludeLinks: links,
		},
		Cache: pipeline.CacheOptions{
			NoCache: ro.fetchOpts.noCache,
			Dir:     cfg.Cache.Dir,
			TTL:     ro.fetchOpts.cacheTTL,
			MaxSize: int64(cfg.Cache.MaxSizeMB) * 1024 * 1024,
		},
		Profile: ro.profile,
		Session: session,
		Security: pipeline.SecurityOptions{
			Robots: robotsEnabled,
		},
		Request: pipeline.RequestOptions{
			Method:  strings.ToUpper(method),
			Headers: ro.extraHeaders,
			Body:    body,
		},
		StoreFullText:    storeFullText,
		CharLimit:        charLimit,
		WaybackFallback:  waybackFB || waybackAt != "",
		WaybackTimestamp: waybackAt,
		Query:            query,
		TopK:             topK,
		CSSSelector:      selector,
		Frontmatter:      frontmatter,
		SchemaPath:       schemaPath,
	}
	if robotsEnabled {
		ro.opts.Security.RobotsChecker = robots.NewChecker()
	}

	ro.allowPrivate = allowPrivateFlag || cfg.Security.AllowPrivate
	ro.opts.Security.AllowPrivate = ro.allowPrivate

	return ro, nil
}

// writeSeparator writes the --- delimiter placed between multi-URL results.
func writeSeparator(w io.Writer) {
	fmt.Fprint(w, "\n---\n\n")
}

func runSequential(ctx context.Context, w io.Writer, client fetch.Client, eng pipeline.Engine, urls []string, opts pipeline.Options, frontmatter bool) error {
	var failCount int
	for i, rawURL := range urls {
		if i > 0 {
			writeSeparator(w)
		}
		res, err := pipeline.Run(ctx, client, eng, rawURL, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", rawURL, err)
			failCount++
			continue
		}
		if opts.Format == pipeline.FormatMarkdown {
			printMarkdownResult(w, res, frontmatter)
		} else {
			fmt.Fprint(w, res.Output)
			if !strings.HasSuffix(res.Output, "\n") {
				fmt.Fprintln(w)
			}
		}
	}
	if failCount > 0 {
		return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(urls)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
	}
	return nil
}

func runRaw(ctx context.Context, w io.Writer, client fetch.Client, urls []string, method string, headers map[string]string, data string, allowPrivate bool) error {
	var failCount int
	for _, rawURL := range urls {
		req := fetch.Request{
			URL:          rawURL,
			Method:       strings.ToUpper(method),
			Headers:      headers,
			AllowPrivate: allowPrivate,
		}
		if data != "" {
			req.Body = []byte(data)
		}
		resp, err := client.Fetch(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", rawURL, err)
			failCount++
			continue
		}
		fmt.Fprint(w, string(resp.Body))
	}
	if failCount > 0 {
		return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(urls)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
	}
	return nil
}

func runMultiJSON(ctx context.Context, w io.Writer, client fetch.Client, eng pipeline.Engine, urls []string, opts pipeline.Options) error {
	type jsonOut struct {
		URL    string `json:"url"`
		OK     bool   `json:"ok"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	results := make([]jsonOut, 0, len(urls))
	var failCount int
	for _, rawURL := range urls {
		res, err := pipeline.Run(ctx, client, eng, rawURL, opts)
		if err != nil {
			results = append(results, jsonOut{URL: rawURL, OK: false, Error: err.Error()})
			failCount++
		} else {
			results = append(results, jsonOut{URL: rawURL, OK: true, Output: res.Output})
		}
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(data))
	if failCount > 0 {
		return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(urls)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
	}
	return nil
}

func runBatch(ctx context.Context, w io.Writer, client fetch.Client, eng pipeline.Engine, urls []string, opts pipeline.Options, concurrency int) error {
	items := make([]batch.Item, len(urls))
	for i, u := range urls {
		items[i] = batch.Item{URL: u}
	}

	adaptivePool := batch.NewAdaptivePool(2, 8)
	pool := batch.Pool{Workers: concurrency, PerHost: 2, Adaptive: true, AdaptivePool: adaptivePool}
	results := pool.RunBatch(ctx, client, eng, items, opts)

	var failCount int
	for i, r := range results {
		if i > 0 {
			writeSeparator(w)
		}
		if r.OK {
			fmt.Fprint(w, r.Output)
			if !strings.HasSuffix(r.Output, "\n") {
				fmt.Fprintln(w)
			}
		} else {
			fmt.Fprintf(os.Stderr, "error fetching %s: %s\n", r.URL, r.Error)
			failCount++
		}
	}
	if failCount > 0 {
		return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(urls)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
	}
	return nil
}

func printMarkdownResult(w io.Writer, res *pipeline.Result, frontmatter bool) {
	fmt.Fprint(w, apicommon.FormatWithMeta(res, pipeline.FormatMarkdown, frontmatter))
	if !strings.HasSuffix(res.Output, "\n") {
		fmt.Fprintln(w)
	}
}

func parseHeaders(raw []string) (map[string]string, error) {
	m := make(map[string]string, len(raw))
	for _, h := range raw {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header %q: expected \"Key: Value\"", h)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("invalid header %q: expected \"Key: Value\"", h)
		}
		m[key] = strings.TrimSpace(parts[1])
	}
	return m, nil
}

func newVersionCmd() *cobra.Command {
	var flagCheck bool
	var flagJSON bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := versionkit.New("symfetch", version, 1)
			if flagJSON {
				return info.Write(os.Stdout)
			}
			fmt.Println(info.String())

			if flagCheck {
				checker := updatecheck.NewChecker("danieljustus", "symaira-fetch")
				release, err := checker.Check(context.Background(), version)
				if err != nil {
					fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
					return nil
				}
				if release != nil {
					fmt.Printf("Update available: %s\n", release.TagName)
					fmt.Printf("Download: %s\n", release.HTMLURL)
				} else {
					fmt.Println("Already up to date.")
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagCheck, "check", false, "Check for updates on GitHub")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Emit version as machine-readable JSON")
	return cmd
}

func newMCPCmd() *cobra.Command {
	var flagProfile string
	var flagProxy string
	var flagHTTP bool
	var flagAddr string
	var flagToken string

	cmd := &cobra.Command{
		Use:     "mcp",
		Aliases: []string{"serve"},
		Short:   "Start the MCP stdio server or HTTP REST server",
		Long: `Start a JSON-RPC 2.0 MCP server over stdin/stdout for use with AI agents.

Use --http to start an HTTP REST server instead (POST /fetch endpoint with bearer auth).`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: config error: %v\n", err)
				cfg = config.Defaults()
			}

			profile, proxy, timeoutSec, maxBodyMB := resolveMCPOptions(cmd, cfg)

			if flagHTTP {
				token := flagToken
				if token == "" {
					token = os.Getenv("SYMFETCH_HTTP_TOKEN")
				}
				return httpserver.Start(flagAddr, token, profile, proxy)
			}
			mcp.ServerVersion = version
			p := fetch.ParseProfile(profile)
			return mcp.StartServer(p, proxy, timeoutSec, maxBodyMB)
		},
	}

	cmd.Flags().StringVar(&flagProfile, "profile", "chrome", "Browser profile: chrome, firefox, opera, safari, edge, ios, honest, random")
	cmd.Flags().StringVar(&flagProxy, "proxy", "", "Proxy URL")
	cmd.Flags().BoolVar(&flagHTTP, "http", false, "Start HTTP REST server instead of MCP stdio server")
	cmd.Flags().StringVar(&flagAddr, "addr", ":8787", "HTTP listen address (host:port)")
	cmd.Flags().StringVar(&flagToken, "token", "", "Bearer token for HTTP auth (or set SYMFETCH_HTTP_TOKEN)")
	return cmd
}

// resolveMCPOptions resolves MCP server options with flags winning over the
// config file, mirroring the root fetch command. The mcp command has no
// timeout/max-body flags, so those always come from config.
func resolveMCPOptions(cmd *cobra.Command, cfg *config.Config) (profile, proxy string, timeoutSec, maxBodyMB int) {
	profile = cfg.HTTP.Profile
	if cmd.Flags().Changed("profile") {
		profile, _ = cmd.Flags().GetString("profile")
	}
	proxy = cfg.HTTP.Proxy
	if cmd.Flags().Changed("proxy") {
		proxy, _ = cmd.Flags().GetString("proxy")
	}
	timeoutSec = cfg.HTTP.TimeoutSeconds
	maxBodyMB = cfg.HTTP.MaxBodyMB
	return profile, proxy, timeoutSec, maxBodyMB
}

func newConfigCmd() *cobra.Command {
	cfg := &cobra.Command{
		Use:   "config",
		Short: "Manage symfetch configuration",
	}

	initCmd := &cobra.Command{
		Use:          "init",
		Short:        "Write default config to ~/.config/symfetch/config.toml",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "cannot determine home directory")
			}
			dir := home + "/.config/symfetch"
			if err := os.MkdirAll(dir, 0755); err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "cannot create config directory")
			}
			path := dir + "/config.toml"
			force, _ := cmd.Flags().GetBool("force")
			if _, err := os.Stat(path); err == nil {
				if !force {
					fmt.Fprintf(os.Stderr, "config already exists at %s\n", path)
					return nil
				}
				fmt.Fprintf(os.Stderr, "warning: overwriting existing config at %s\n", path)
			}
			if err := os.WriteFile(path, []byte(config.DefaultConfigTOML()), 0600); err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindConfig, "cannot write config file")
			}
			fmt.Printf("Config written to %s\n", path)
			return nil
		},
	}
	initCmd.Flags().Bool("force", false, "overwrite existing config file")

	cfg.AddCommand(initCmd)
	return cfg
}
