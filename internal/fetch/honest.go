package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const honestUA = "symfetch/0.1 (+https://github.com/danieljustus/symaira-fetch)"

// honestClient uses stdlib net/http with a plain user-agent.
type honestClient struct {
	hc        *http.Client
	opts      *clientOptions
	sessStore *sessionStore
}

func newHonestClient(o *clientOptions) (*honestClient, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	if o.proxy != "" {
		proxyURL, err := url.Parse(o.proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	hc := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	var sessDir string
	if o.sessionsDir != "" {
		sessDir = o.sessionsDir
	}

	return &honestClient{
		hc:        hc,
		opts:      o,
		sessStore: newSessionStore(sessDir),
	}, nil
}

func (c *honestClient) Fetch(ctx context.Context, req Request) (*Response, error) {
	if !req.AllowPrivate {
		if err := checkSSRF(req.URL); err != nil {
			return nil, err
		}
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	timeout := time.Duration(c.opts.timeoutSeconds) * time.Second
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	maxBody := int64(c.opts.maxBodyMB) * 1024 * 1024
	if req.MaxBody > 0 {
		maxBody = req.MaxBody
	}

	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	httpReq.Header.Set("User-Agent", honestUA)
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.5")

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq = httpReq.WithContext(timeoutCtx)

	if req.Proxy != "" {
		proxyURL, err := url.Parse(req.Proxy)
		if err == nil {
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			tempClient := &http.Client{Transport: transport, CheckRedirect: c.hc.CheckRedirect}
			return c.doRequest(tempClient, httpReq, maxBody)
		}
	}

	return c.doRequest(c.hc, httpReq, maxBody)
}

func (c *honestClient) doRequest(hc *http.Client, req *http.Request, maxBody int64) (*Response, error) {
	start := time.Now()
	resp, err := hc.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", req.URL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxBody {
		return nil, &ErrTooLarge{URL: req.URL.String(), Limit: maxBody}
	}

	ct := resp.Header.Get("Content-Type")
	body, err = normaliseCharset(body, ct)
	if err != nil {
		body = bytes.ToValidUTF8(body, []byte("?"))
	}

	proto := "HTTP/1.1"
	if strings.HasPrefix(resp.Proto, "HTTP/2") {
		proto = "HTTP/2.0"
	}

	headers := make(map[string][]string)
	for k, v := range resp.Header {
		headers[k] = v
	}

	return &Response{
		FinalURL:    resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		Headers:     headers,
		Body:        body,
		Protocol:    proto,
		ContentType: ct,
		Elapsed:     elapsed,
	}, nil
}

func (c *honestClient) Close() error {
	c.hc.CloseIdleConnections()
	return nil
}
