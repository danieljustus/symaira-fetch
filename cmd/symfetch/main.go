package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-corekit/logkit"
	"github.com/danieljustus/symaira-corekit/updatecheck"
	"github.com/danieljustus/symaira-fetch/internal/batch"
	"github.com/danieljustus/symaira-fetch/internal/config"
	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/mcp"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
	"github.com/danieljustus/symaira-fetch/internal/render"
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
		flagFormat      string
		flagRaw         bool
		flagProfile     string
		flagProxy       string
		flagTimeout     string
		flagMaxChars    int
		flagLinks       bool
		flagSession     string
		flagNoCache     bool
		flagCacheTTL    string
		flagHeaders     []string
		flagMethod      string
		flagData        string
		flagConcurrency int
		flagAllowPriv   bool
		flagRobots      bool
		flagNoRetry     bool
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

			profile := cfg.HTTP.Profile
			if cmd.Flags().Changed("profile") {
				profile = flagProfile
			}
			proxy := cfg.HTTP.Proxy
			if cmd.Flags().Changed("proxy") {
				proxy = flagProxy
			}
			format := cfg.HTTP.DefaultFormat
			if cmd.Flags().Changed("format") {
				format = flagFormat
			}
			maxChars := cfg.HTTP.MaxChars
			if cmd.Flags().Changed("max-chars") {
				maxChars = flagMaxChars
			}

			fo, err := resolveFetchOptions(cmd, cfg)
			if err != nil {
				return err
			}

			timeoutSec := cfg.HTTP.TimeoutSeconds
			if cmd.Flags().Changed("timeout") {
				d, err := time.ParseDuration(flagTimeout)
				if err != nil {
					return exitcodes.Wrap(err, exitcodes.ExitConfig, exitcodes.KindValidation, "invalid timeout")
				}
				timeoutSec = int(d.Seconds())
			}
			if timeoutSec > 120 {
				fmt.Fprintf(os.Stderr, "warning: timeout %ds exceeds MCP server cap of 120s; MCP requests will be capped\n", timeoutSec)
			}

			extraHeaders := parseHeaders(flagHeaders)

			p := fetch.ParseProfile(profile)
			client, err := fetch.New(p,
				fetch.WithProxy(proxy),
				fetch.WithTimeout(timeoutSec),
				fetch.WithMaxBody(cfg.HTTP.MaxBodyMB),
				fetch.WithRetry(!flagNoRetry),
			)
			if err != nil {
				return exitcodes.Wrap(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "init client")
			}
			defer client.Close()

			eng := pipeline.StaticEngine{}
			opts := pipeline.Options{
				Format: pipeline.ParseFormat(format),
				Content: pipeline.ContentOptions{
					MaxChars:     maxChars,
					IncludeLinks: flagLinks,
				},
				Cache: pipeline.CacheOptions{
					NoCache: fo.noCache,
					Dir:     cfg.Cache.Dir,
					TTL:     fo.cacheTTL,
					MaxSize: int64(cfg.Cache.MaxSizeMB) * 1024 * 1024,
				},
				Profile: profile,
				Session: flagSession,
				Security: pipeline.SecurityOptions{
					Robots: flagRobots,
				},
			}
			if flagRobots {
				opts.Security.RobotsChecker = robots.NewChecker()
			}

			ctx := context.Background()
			allowPrivate := flagAllowPriv || cfg.Security.AllowPrivate

			opts.Security.AllowPrivate = allowPrivate

			if allowPrivate {
				fmt.Fprintf(os.Stderr, "warning: SSRF guard disabled — fetching private/loopback addresses is permitted\n")
			}

			if flagRaw {
				return runRaw(ctx, client, args, flagMethod, extraHeaders, flagData, allowPrivate)
			}

			if opts.Format == pipeline.FormatJSON && len(args) > 1 {
				return runMultiJSON(ctx, client, eng, args, opts)
			}

			if len(args) > 1 && fo.concurrency > 1 {
				return runBatch(ctx, client, eng, args, opts, fo.concurrency)
			}

			var failCount int
			for i, rawURL := range args {
				if i > 0 {
					fmt.Print("\n---\n\n")
				}
				res, err := pipeline.Run(ctx, client, eng, rawURL, opts)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", rawURL, err)
					failCount++
					continue
				}
				if opts.Format == pipeline.FormatMarkdown {
					printMarkdownResult(res)
				} else {
					fmt.Print(res.Output)
					if !strings.HasSuffix(res.Output, "\n") {
						fmt.Println()
					}
				}
			}
			if failCount > 0 {
				return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(args)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
			}
			return nil
		},
	}

	root.Flags().StringVarP(&flagFormat, "format", "f", "markdown", "Output format: markdown, json, text, html")
	root.Flags().BoolVar(&flagRaw, "raw", false, "Return raw decoded response body without semantic processing")
	root.Flags().StringVar(&flagProfile, "profile", "chrome", "Browser profile: chrome, firefox, honest")
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

	root.AddCommand(newVersionCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newConfigCmd())

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

func runRaw(ctx context.Context, client fetch.Client, urls []string, method string, headers map[string]string, data string, allowPrivate bool) error {
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
		fmt.Print(string(resp.Body))
	}
	if failCount > 0 {
		return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(urls)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
	}
	return nil
}

func runMultiJSON(ctx context.Context, client fetch.Client, eng pipeline.Engine, urls []string, opts pipeline.Options) error {
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
	fmt.Println(string(data))
	if failCount > 0 {
		return exitcodes.Wrap(fmt.Errorf("%d of %d URLs failed", failCount, len(urls)), exitcodes.ExitGeneric, exitcodes.KindUnavailable, "partial failure")
	}
	return nil
}

func runBatch(ctx context.Context, client fetch.Client, eng pipeline.Engine, urls []string, opts pipeline.Options, concurrency int) error {
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
			fmt.Print("\n---\n\n")
		}
		if r.OK {
			fmt.Print(r.Output)
			if !strings.HasSuffix(r.Output, "\n") {
				fmt.Println()
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

func printMarkdownResult(res *pipeline.Result) {
	fmt.Print(render.FormatMarkdownWithMeta(res.Meta, res.Output))
	if !strings.HasSuffix(res.Output, "\n") {
		fmt.Println()
	}
}

func parseHeaders(raw []string) map[string]string {
	m := make(map[string]string, len(raw))
	for _, h := range raw {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m
}

func newVersionCmd() *cobra.Command {
	var flagCheck bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("symfetch", version)

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
	return cmd
}

func newMCPCmd() *cobra.Command {
	var flagProfile string
	var flagProxy string

	cmd := &cobra.Command{
		Use:          "mcp",
		Aliases:      []string{"serve"},
		Short:        "Start the MCP stdio server",
		Long:         "Start a JSON-RPC 2.0 MCP server over stdin/stdout for use with AI agents.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			mcp.ServerVersion = version
			p := fetch.ParseProfile(flagProfile)
			return mcp.StartServer(p, flagProxy)
		},
	}

	cmd.Flags().StringVar(&flagProfile, "profile", "chrome", "Browser profile: chrome, firefox, honest")
	cmd.Flags().StringVar(&flagProxy, "proxy", "", "Proxy URL")
	return cmd
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
