package clients

import (
	"log/slog"
	"maps"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/pkg/errors"
)

type DebuggingTransport struct {
	Inner http.RoundTripper
}

func NewDebuggingTransport(inner http.RoundTripper) *DebuggingTransport {
	return &DebuggingTransport{
		Inner: inner,
	}
}

func (t *DebuggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !slog.Default().Enabled(req.Context(), slog.LevelDebug) {
		return t.Inner.RoundTrip(req)
	}

	oldHeaders := maps.Clone(req.Header)
	if x := req.Header.Get("Authorization"); x != "" {
		req.Header.Set("Authorization", strings.Repeat("*", len(x)))
	}
	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to dump request")
	}
	req.Header = oldHeaders
	slog.Debug("http request", slog.String("dump", string(dump))) //nolint:gosec // G706: intentional debug-only HTTP dump
	res, err := t.Inner.RoundTrip(req)
	if err != nil {
		slog.Debug("http error", slog.Any("err", err))
		return nil, errors.Wrap(err, "failed to execute request")
	}
	dump, err = httputil.DumpResponse(res, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to dump response")
	}
	slog.Debug("http response", slog.String("dump", string(dump))) //nolint:gosec // G706: intentional debug-only HTTP dump
	return res, nil
}
