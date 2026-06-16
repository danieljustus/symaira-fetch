package batch

import (
	"context"
	"net/url"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/danieljustus/symaira-fetch/internal/fetch"
	"github.com/danieljustus/symaira-fetch/internal/pipeline"
)

const defaultPerHost = 2

// Item is a single URL in a batch request.
type Item struct {
	URL     string
	Request fetch.Request // per-item overrides
}

// Result is the outcome for one URL in a batch.
type Result struct {
	URL    string
	OK     bool
	Output string
	Error  string
}

// Pool runs batch fetch+pipeline jobs with global and per-host concurrency limits.
type Pool struct {
	Workers int
	PerHost int
}

// RunBatch executes items in parallel, preserving input order in the results.
func (p Pool) RunBatch(ctx context.Context, c fetch.Client, eng pipeline.Engine, items []Item, opts pipeline.Options) []Result {
	workers := p.Workers
	if workers <= 0 {
		workers = 4
	}
	perHost := p.PerHost
	if perHost <= 0 {
		perHost = defaultPerHost
	}

	results := make([]Result, len(items))
	var hostMu sync.Mutex
	hostSems := make(map[string]chan struct{})

	g, gctx := errgroup.WithContext(ctx)
	globalSem := make(chan struct{}, workers)

	for i, item := range items {
		i, item := i, item
		g.Go(func() error {
			globalSem <- struct{}{}
			defer func() { <-globalSem }()

			host := HostOf(item.URL)
			hostMu.Lock()
			if _, ok := hostSems[host]; !ok {
				hostSems[host] = make(chan struct{}, perHost)
			}
			hs := hostSems[host]
			hostMu.Unlock()

			hs <- struct{}{}
			defer func() { <-hs }()

			res, err := pipeline.Run(gctx, c, eng, item.URL, opts)
			if err != nil {
				results[i] = Result{URL: item.URL, OK: false, Error: err.Error()}
			} else {
				results[i] = Result{URL: item.URL, OK: true, Output: res.Output}
			}
			return nil
		})
	}

	_ = g.Wait()
	return results
}

// HostOf extracts the host (with optional port) from a raw URL string.
func HostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
