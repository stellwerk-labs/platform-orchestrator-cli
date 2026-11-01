package mcpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientBoundary(t *testing.T) {
	var methods []string
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect destination received a request") }))
	defer destination.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		assert.Equal(t, "Bearer private-test-token", r.Header.Get("Authorization"))
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer api.Close()
	client, err := NewClient(api.URL, "private-test-token", "")
	require.NoError(t, err)
	response, err := client.ListProjectsWithResponse(context.Background(), "acme", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusTemporaryRedirect, response.StatusCode())
	require.Equal(t, []string{http.MethodGet}, methods)
	denied, err := client.CreateProject(context.Background(), "acme", cp.ProjectCreateBody{Id: "denied"})
	if denied != nil {
		_ = denied.Body.Close()
	}
	require.Error(t, err)
	require.Equal(t, []string{http.MethodGet}, methods)
}

func TestClientEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com?token=secret", "https://example.com#secret", "file:///tmp/api"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := NewClient(endpoint, "token", "")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestClientResponseLimitAndCancellation(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("s", maxResponseBytes+1))
	}))
	defer api.Close()
	client, err := NewClient(api.URL, "token", "")
	require.NoError(t, err)
	_, err = client.ListProjectsWithResponse(context.Background(), "acme", nil)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.ListProjectsWithResponse(ctx, "acme", nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "token")
}

func TestClientRequiresTokenForLoopback(t *testing.T) {
	for _, endpoint := range []string{"http://localhost:8080", "http://127.0.0.1:8080", "https://localhost"} {
		_, err := NewClient(endpoint, "", "")
		require.ErrorContains(t, err, "requires configured authentication")
	}
}
