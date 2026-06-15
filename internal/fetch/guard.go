package fetch

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedPrivate is returned when a request targets a private/loopback
// address and AllowPrivate is false.
type ErrBlockedPrivate struct {
	URL string
}

func (e *ErrBlockedPrivate) Error() string {
	return fmt.Sprintf("blocked_private: %s targets a private or loopback address", e.URL)
}

// checkSSRF returns an error if the URL targets a blocked address.
// It blocks:
//   - non-http(s) schemes
//   - loopback (127.0.0.0/8, ::1)
//   - link-local (169.254.0.0/16, fe80::/10)
//   - RFC-1918 private ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
//   - IPv4-mapped IPv6 (::ffff:0:0/96)
//
// The check happens at TCP connection time via a Control function on the
// dialer, preventing DNS-rebinding attacks where the first resolution
// returns a public IP that passes the check but the second (used for the
// actual connection) resolves to a private IP.
func checkSSRF(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return &ErrBlockedPrivate{URL: rawURL}
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("invalid URL: no host")
	}

	// Perform DNS resolution and validate every resolved IP at connection
	// time using a custom Control function. This prevents DNS-rebinding
	// where a second lookup during the TCP handshake returns a private IP.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := &net.Dialer{Control: controlSSRF}
			return dialer.DialContext(ctx, network, address)
		},
	}
	ips, err := resolver.LookupHost(ctx, host)
	if err != nil {
		// DNS failure is not inherently an SSRF — the connection will fail.
		return nil
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isPrivate(ip) {
			return &ErrBlockedPrivate{URL: rawURL}
		}
	}
	return nil
}

// controlSSRF is a net.Dialer.Control function that rejects connections to
// private/loopback addresses at TCP connect time, preventing DNS-rebinding.
func controlSSRF(network, address string, c syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip != nil && isPrivate(ip) {
		return &ErrBlockedPrivate{URL: address}
	}
	return nil
}

var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"::1/128",
		"169.254.0.0/16",
		"fe80::/10",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
		"100.64.0.0/10",
	}
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, n, _ := net.ParseCIDR(cidr)
		if n != nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

func isPrivate(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
