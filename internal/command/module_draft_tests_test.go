package command

import (
	"github.com/google/uuid"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDraftTestSelectionsRequireExactUnambiguousImmutableCoordinates(t *testing.T) {
	module, snapshot := uuid.NewString(), uuid.NewString()
	selected, err := parseDraftTestSelections([]string{module + "=" + snapshot})
	require.NoError(t, err)
	require.Equal(t, uuid.MustParse(snapshot), selected[module])
	empty, err := parseDraftTestSelections(nil)
	require.NoError(t, err)
	require.Nil(t, empty)
	for _, value := range []string{"greeting=" + snapshot, module + "=moving-head", module + "=" + uuid.Nil.String(), "urn:uuid:" + module + "=" + snapshot, module + "=" + snapshot + "=other"} {
		_, err = parseDraftTestSelections([]string{value})
		require.Error(t, err, value)
	}
	_, err = parseDraftTestSelections([]string{module + "=" + snapshot, module + "=" + uuid.NewString()})
	require.ErrorContains(t, err, "more than one")
}

func TestDraftTestDeploymentDoesNotFallBackWhenNewRouteIsAbsent(t *testing.T) {
	var ordinaryCalls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orgs/org/deployments", func(w http.ResponseWriter, _ *http.Request) { ordinaryCalls.Add(1); w.WriteHeader(http.StatusCreated) })
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := dp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	response, err := createDeploymentWithDraftSelection(t.Context(), client, "org", &dp.CreateDeploymentParams{}, dp.DeploymentCreateBody{}, map[string]uuid.UUID{uuid.NewString(): uuid.New()})
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, response.StatusCode())
	require.Zero(t, ordinaryCalls.Load(), "an older server must never deploy its Default instead of the requested test")
}
