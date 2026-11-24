package clients

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDebuggingTransportPreservesHTTPValuesWithoutLoggingThem(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(map[bool]string{false: "debug disabled", true: "debug enabled"}[debug], func(t *testing.T) {
			var logs bytes.Buffer
			level := slog.LevelInfo
			if debug {
				level = slog.LevelDebug
			}
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: level})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			requestBody := "request-body-canary\x00\xff"
			responseBody := "response-body-canary\x00\xfe"
			received := make(chan map[string]string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", http.StatusBadRequest)
					return
				}
				received <- map[string]string{
					"method": r.Method, "query": r.URL.RawQuery, "body": string(body),
					"cookie": r.Header.Get("Cookie"), "authorization": r.Header.Get("Authorization"), "custom": r.Header.Get("X-Private"),
				}
				w.Header().Set("Set-Cookie", "response-cookie-canary")
				w.Header().Set("X-Private", "response-header-canary")
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, responseBody)
			}))
			t.Cleanup(server.Close)
			inner := http.DefaultTransport.(*http.Transport).Clone()
			t.Cleanup(inner.CloseIdleConnections)
			client := &http.Client{Transport: NewDebuggingTransport(inner), Timeout: 2 * time.Second}
			req, err := http.NewRequestWithContext(t.Context(), "METHOD-CANARY", server.URL+"/path-canary?query=query-canary", strings.NewReader(requestBody))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer authorization-canary")
			req.Header.Set("Cookie", "request-cookie-canary")
			req.Header.Set("X-Private", "request-header-canary")
			response, err := client.Do(req)
			require.NoError(t, err)
			defer func() { require.NoError(t, response.Body.Close()) }()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, responseBody, string(body))
			require.Equal(t, http.StatusAccepted, response.StatusCode)
			require.Equal(t, "response-cookie-canary", response.Header.Get("Set-Cookie"))
			require.Equal(t, "response-header-canary", response.Header.Get("X-Private"))
			require.Equal(t, map[string]string{
				"method": "METHOD-CANARY", "query": "query=query-canary", "body": requestBody,
				"cookie": "request-cookie-canary", "authorization": "Bearer authorization-canary", "custom": "request-header-canary",
			}, <-received)
			require.Equal(t, "Bearer authorization-canary", req.Header.Get("Authorization"))
			require.NotContains(t, logs.String(), "canary")
			require.NotContains(t, logs.String(), "METHOD-CANARY")
			if debug {
				decoder := json.NewDecoder(&logs)
				var requestEvent, responseEvent map[string]any
				require.NoError(t, decoder.Decode(&requestEvent))
				require.NoError(t, decoder.Decode(&responseEvent))
				delete(requestEvent, slog.TimeKey)
				delete(responseEvent, slog.TimeKey)
				require.Equal(t, map[string]any{slog.LevelKey: slog.LevelDebug.String(), slog.MessageKey: "http request"}, requestEvent)
				require.Equal(t, map[string]any{slog.LevelKey: slog.LevelDebug.String(), slog.MessageKey: "http response", "status": float64(http.StatusAccepted)}, responseEvent)
				require.ErrorIs(t, decoder.Decode(&responseEvent), io.EOF)
			} else {
				require.Empty(t, logs.String())
			}
		})
	}
}

func TestDebuggingTransportFailureKeepsCauseOutOfLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	inner := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(inner.CloseIdleConnections)
	client := &http.Client{Transport: NewDebuggingTransport(inner), Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/error-path-canary?query=error-query-canary", nil)
	require.NoError(t, err)
	response, err := client.Do(req)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.Nil(t, response)
	require.ErrorContains(t, err, "failed to execute request")
	require.ErrorContains(t, err, "error-query-canary")
	var connectionError *net.OpError
	require.ErrorAs(t, err, &connectionError)
	require.Equal(t, "dial", connectionError.Op)
	require.NotContains(t, logs.String(), "canary")
	require.NotContains(t, logs.String(), address)
	decoder := json.NewDecoder(&logs)
	var requestEvent, errorEvent map[string]any
	require.NoError(t, decoder.Decode(&requestEvent))
	require.NoError(t, decoder.Decode(&errorEvent))
	delete(requestEvent, slog.TimeKey)
	delete(errorEvent, slog.TimeKey)
	require.Equal(t, map[string]any{slog.LevelKey: slog.LevelDebug.String(), slog.MessageKey: "http request"}, requestEvent)
	require.Equal(t, map[string]any{slog.LevelKey: slog.LevelDebug.String(), slog.MessageKey: "http error"}, errorEvent)
	require.ErrorIs(t, decoder.Decode(&errorEvent), io.EOF)
}
