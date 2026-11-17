package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ErrBlockedIP indicates that a network connection was aborted because the destination IP is blocked by SSRF protection.
var ErrBlockedIP = errors.New("connection to blocked IP is prohibited (SSRF prevention)")

// NewSafeHTTPClient creates an HTTP client hardened against SSRF and DNS rebinding attacks.
// It enforces:
// 1. No automatic redirects (CheckRedirect returns http.ErrUseLastResponse).
// 2. Pre-connection IP verification via Dialer.ControlContext (blocking private, loopback, link-local, CGNAT IPs).
func NewSafeHTTPClient(allowLocal bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		ControlContext: func(_ context.Context, _ string, address string, _ syscall.RawConn) error {
			if allowLocal {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("invalid socket address %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("invalid IP in socket address %q", host)
			}
			if IsBlockedIP(ip) {
				return fmt.Errorf("%w: %s", ErrBlockedIP, ip.String())
			}
			return nil
		},
	}

	transport := &http.Transport{
		Proxy:                 nil, // Prevent ambient proxies from bypassing IP controls
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Do not follow redirects. Return last response to caller.
			return http.ErrUseLastResponse
		},
	}
}
