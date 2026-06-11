package fetch

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrBlockedPrivate is returned when a request targets a private/loopback
// address and AllowPrivate is false.
type ErrBlockedPrivate struct {
	URL string
}

func (e *ErrBlockedPrivate) Error() string {
	return fmt.Sprintf("blocked_private: %s targets a private or loopback address", e.URL)
}

// checkSSRF returns an error if the URL resolves to a blocked address.
// It blocks:
//   - non-http(s) schemes
//   - loopback (127.0.0.0/8, ::1)
//   - link-local (169.254.0.0/16, fe80::/10)
//   - RFC-1918 private ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
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

	ips, err := net.LookupHost(host)
	if err != nil {
		// Let the actual request fail with a DNS error; don't pre-block.
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
		"100.64.0.0/10", // Carrier-grade NAT
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
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
