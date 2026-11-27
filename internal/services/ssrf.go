package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// isBlockedSSRFIP returns true if ip is loopback, private, link-local (including cloud metadata 169.254.169.254),
// unspecified, or multicast.
func isBlockedSSRFIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

func allowPrivateOutbound() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ALLOW_PRIVATE_WEBHOOKS")))
	return v == "1" || v == "true" || v == "yes"
}

// NewSafeHTTPClient returns an *http.Client that blocks outbound SSRF to private, loopback,
// and link-local/metadata IP ranges unless ALLOW_PRIVATE_WEBHOOKS=true is explicitly set.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if allowPrivateOutbound() {
				return dialer.DialContext(ctx, network, addr)
			}
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid target address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP addresses resolved for host %q", host)
			}
			for _, ipAddr := range ips {
				if isBlockedSSRFIP(ipAddr.IP) {
					return nil, fmt.Errorf("outbound request to private/internal IP %s is blocked", ipAddr.IP.String())
				}
			}
			// Dial the first validated IP directly to prevent DNS rebinding between lookup and dial.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
