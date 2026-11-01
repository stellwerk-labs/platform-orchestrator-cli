// Package mcpserver exposes a deliberately small, read-only metadata interface.
package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

const maxResponseBytes = 2 << 20

// NewClient uses the CLI's TLS configuration without its debug logging or retries.
// Credentials are attached only after checking the configured API origin.
func NewClient(endpoint, token, caFile string) (*cp.ClientWithResponses, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid MCP API endpoint")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return nil, errors.New("MCP API endpoint requires HTTPS (HTTP is allowed only for loopback development)")
	}
	if token == "" {
		return nil, errors.New("MCP requires configured authentication")
	}
	transport, err := clients.NewSecureTransport(caFile)
	if err != nil {
		return nil, errors.New("failed to configure MCP API TLS")
	}
	client := &http.Client{
		Timeout:       30 * time.Second,
		Transport:     &readTransport{base: transport, origin: u.Scheme + "://" + u.Host, token: token},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return cp.NewClientWithResponses(strings.TrimSuffix(endpoint, "/"), cp.WithHTTPClient(client))
}

type readTransport struct {
	base          http.RoundTripper
	origin, token string
}

func (t *readTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || req.URL.Scheme+"://"+req.URL.Host != t.origin {
		return nil, errors.New("MCP API request is outside the read-only boundary")
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	res, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, errors.New("MCP API request failed")
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return nil, errors.New("MCP API response exceeds its limit or could not be read")
	}
	res.Body = io.NopCloser(bytes.NewReader(body))
	return res, nil
}

func requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}
