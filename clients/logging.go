package clients

import (
	"log/slog"
	"net/http"

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

	slog.Debug("http request")
	res, err := t.Inner.RoundTrip(req)
	if err != nil {
		slog.Debug("http error")
		return nil, errors.Wrap(err, "failed to execute request")
	}
	slog.Debug("http response", slog.Int("status", res.StatusCode)) // #nosec G706 -- The message is constant and status is an integer attribute.
	return res, nil
}
