package command

import (
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Exercise real generated HTTP clients against an actual empty router. This
// proves route selection and no fallback, not old-server business conformance.
func TestExplicitInterfacePublicationNeverFallsBackToLegacyRoute(t *testing.T) {
	paths := make(chan string, 8)
	router := http.NewServeMux()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := cp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	number := int64(2)
	body := cp.ModuleVersionPublishBody{SemanticVersion: "1.2.0", ResourceTypeVersion: &number}
	_, err = publishVersionForSelectedInterface(t.Context(), client, "example", "storage", &cp.PublishModuleVersionParams{IdempotencyKey: "publish-2"}, body)
	require.ErrorContains(t, err, "HTTP 404")
	require.Len(t, paths, 1)
	require.Equal(t, "/orgs/example/modules/storage/version-publications", <-paths)
	_, err = publishSuccessorForSelectedInterface(t.Context(), client, "example", "storage", "1.2.0-rc.1", &cp.PublishStableModuleVersionSuccessorParams{IdempotencyKey: "graduate-2"}, cp.StableModuleVersionSuccessorBody{Version: body, Reason: "Reviewed", ExpectedPrereleaseResourceVersion: 1})
	require.ErrorContains(t, err, "HTTP 404")
	require.Len(t, paths, 1)
	require.Equal(t, "/orgs/example/modules/storage/versions/1.2.0-rc.1/successor-publications", <-paths)
	body.ResourceTypeVersion = nil
	response, err := publishVersionForSelectedInterface(t.Context(), client, "example", "storage", &cp.PublishModuleVersionParams{IdempotencyKey: "legacy-1"}, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, response.StatusCode())
	require.Len(t, paths, 1)
	require.Equal(t, "/orgs/example/modules/storage/versions", <-paths)
}
