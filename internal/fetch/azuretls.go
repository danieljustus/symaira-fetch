package fetch

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	azuretls "github.com/Noooste/azuretls-client"
)

// azureClient uses azuretls for browser-impersonating TLS+HTTP/2.
type azureClient struct {
	session   *azuretls.Session
	opts      *clientOptions
	profile   Profile
	sessStore *sessionStore
}

func newAzureClient(p Profile, o *clientOptions) (*azureClient, error) {
	var browser string
	switch p {
	case ProfileFirefox:
		browser = azuretls.Firefox
	default:
		browser = azuretls.Chrome
	}

	sess := azuretls.NewSession()
	sess.Browser = browser

	if o.proxy != "" {
		if err := sess.SetProxy(o.proxy); err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
	}

	var sessDir string
	if o.sessionsDir != "" {
		sessDir = o.sessionsDir
	}

	return &azureClient{
		session:   sess,
		opts:      o,
		profile:   p,
		sessStore: newSessionStore(sessDir),
	}, nil
}

func (c *azureClient) Fetch(ctx context.Context, req Request) (*Response, error) {
	if !req.AllowPrivate {
		if err := checkSSRF(req.URL); err != nil {
			return nil, err
		}
	}

	method := req.Method
	if method == "" {
		method = "GET"
	}

	timeout := time.Duration(c.opts.timeoutSeconds) * time.Second
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	maxBody := int64(c.opts.maxBodyMB) * 1024 * 1024
	if req.MaxBody > 0 {
		maxBody = req.MaxBody
	}

	azReq := &azuretls.Request{
		Method:  method,
		Url:     req.URL,
		TimeOut: timeout,
	}
	if len(req.Body) > 0 {
		azReq.Body = req.Body
	}

	// Build ordered headers
	if len(req.Headers) > 0 {
		oh := make(azuretls.OrderedHeaders, 0, len(req.Headers))
		for k, v := range req.Headers {
			oh = append(oh, []string{k, v})
		}
		azReq.OrderedHeaders = oh
	}

	// Per-request proxy override via session-level (azuretls supports SetProxy on session only)
	if req.Proxy != "" {
		// Create a one-off session for proxy override
		return c.fetchWithProxy(ctx, req, method, timeout, maxBody)
	}

	start := time.Now()
	azResp, err := c.session.Do(azReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", req.URL, err)
	}

	body := azResp.Body
	if int64(len(body)) > maxBody {
		return nil, &ErrTooLarge{URL: req.URL, Limit: maxBody}
	}

	// Charset normalise
	ct := azResp.Header.Get("Content-Type")
	body, err = normaliseCharset(body, ct)
	if err != nil {
		body = bytes.ToValidUTF8(body, []byte("?"))
	}

	proto := "HTTP/1.1"
	if azResp.HttpResponse != nil {
		if strings.HasPrefix(azResp.HttpResponse.Proto, "HTTP/2") {
			proto = "HTTP/2.0"
		} else if strings.HasPrefix(azResp.HttpResponse.Proto, "HTTP/3") {
			proto = "HTTP/3.0"
		}
	}

	headers := make(map[string][]string)
	for k, v := range azResp.Header {
		headers[k] = v
	}

	finalURL := req.URL
	if azResp.Request != nil {
		finalURL = azResp.Request.Url
	}

	return &Response{
		FinalURL:    finalURL,
		StatusCode:  azResp.StatusCode,
		Headers:     headers,
		Body:        body,
		Protocol:    proto,
		ContentType: ct,
		Elapsed:     elapsed,
	}, nil
}

func (c *azureClient) fetchWithProxy(ctx context.Context, req Request, method string, timeout time.Duration, maxBody int64) (*Response, error) {
	var browser string
	switch c.profile {
	case ProfileFirefox:
		browser = azuretls.Firefox
	default:
		browser = azuretls.Chrome
	}
	sess := azuretls.NewSession()
	sess.Browser = browser
	if err := sess.SetProxy(req.Proxy); err != nil {
		return nil, fmt.Errorf("invalid proxy %s: %w", req.Proxy, err)
	}
	defer sess.Close()

	azReq := &azuretls.Request{
		Method:  method,
		Url:     req.URL,
		TimeOut: timeout,
	}
	if len(req.Body) > 0 {
		azReq.Body = req.Body
	}

	start := time.Now()
	azResp, err := sess.Do(azReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", req.URL, err)
	}

	body := azResp.Body
	if int64(len(body)) > maxBody {
		return nil, &ErrTooLarge{URL: req.URL, Limit: maxBody}
	}

	ct := azResp.Header.Get("Content-Type")
	body, _ = normaliseCharset(body, ct)

	headers := make(map[string][]string)
	for k, v := range azResp.Header {
		headers[k] = v
	}

	finalURL := req.URL
	if azResp.Request != nil {
		finalURL = azResp.Request.Url
	}

	return &Response{
		FinalURL:    finalURL,
		StatusCode:  azResp.StatusCode,
		Headers:     headers,
		Body:        body,
		Protocol:    "HTTP/2.0",
		ContentType: ct,
		Elapsed:     elapsed,
	}, nil
}

func (c *azureClient) Close() error {
	c.session.Close()
	return nil
}
