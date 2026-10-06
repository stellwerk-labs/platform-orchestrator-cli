package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	pluginapi "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-plugin"
)

const (
	nativeTestBodyField  = "name"
	nativeTestFirstQuery = "one"
)

func pluginClientContext(t *testing.T, server *httptest.Server) context.Context {
	t.Helper()
	client, err := pluginapi.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	return context.WithValue(t.Context(), PluginClientContextKey, client)
}

func TestNativeProgressiveRolloutInvocationPreservesClosedEnvelope(t *testing.T) {
	t.Parallel()
	type requestRecord struct {
		path       string
		invocation pluginapi.PluginInvocation
	}
	records := make(chan requestRecord, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/orgs/demo/plugins/"+progressiveRolloutsPluginID+"/invoke" {
			http.NotFound(w, request)
			return
		}
		var invocation pluginapi.PluginInvocation
		if err := json.NewDecoder(request.Body).Decode(&invocation); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		records <- requestRecord{path: request.URL.Path, invocation: invocation}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":201,"body":{"id":"11111111-1111-4111-8111-111111111111"}}`)
	}))
	t.Cleanup(server.Close)

	body := map[string]string{nativeTestBodyField: "release"}
	result, err := invokeNativeProgressiveRollout[map[string]string](pluginClientContext(t, server), "demo", "createProgressiveRollout", nil, nil, body, "stable-key", http.StatusCreated)
	require.NoError(t, err)
	require.Equal(t, "11111111-1111-4111-8111-111111111111", (*result)["id"])
	record := <-records
	require.Equal(t, "/orgs/demo/plugins/"+progressiveRolloutsPluginID+"/invoke", record.path)
	require.Equal(t, "createProgressiveRollout", record.invocation.OperationId)
	require.Equal(t, "stable-key", *record.invocation.IdempotencyKey)
	require.Nil(t, record.invocation.Query)
	require.JSONEq(t, `{"name":"release"}`, string(*record.invocation.Body))
}

func TestNativeProgressiveRolloutListQueryOmitsEmptyRepeatedValues(t *testing.T) {
	t.Parallel()
	query := nativeProgressiveRolloutListQuery("", nil, "", "", "running", "", nil)
	require.Equal(t, map[string][]string{"view": {"running"}}, query)

	query = nativeProgressiveRolloutListQuery("next", []string{"Running", "Paused"}, "project", "module", "history", "all", nil)
	require.Equal(t, []string{"Running", "Paused"}, query["state"])
	require.NotNil(t, query["state"])
}

func TestNativeProgressiveRolloutInvocationRejectsUnknownOperationAndMissingMutationKey(t *testing.T) {
	t.Parallel()
	_, err := invokeNativeProgressiveRollout[struct{}](t.Context(), "demo", "arbitraryURL", nil, nil, nil, "", http.StatusOK)
	require.ErrorContains(t, err, "client does not permit")
	_, err = invokeNativeProgressiveRollout[struct{}](t.Context(), "demo", "createProgressiveRollout", nil, nil, struct{}{}, "", http.StatusCreated)
	require.ErrorContains(t, err, "idempotency key is required")
}

func TestNativeProgressiveRolloutOperationCatalogueMatchesCLIFlows(t *testing.T) {
	t.Parallel()
	type transportMetadata struct {
		method   string
		mutating bool
	}
	want := map[string]transportMetadata{
		"listProgressiveRollouts":           {method: http.MethodGet},
		"getProgressiveRollout":             {method: http.MethodGet},
		"previewProgressiveRollout":         {method: http.MethodPost},
		"createProgressiveRollout":          {method: http.MethodPost, mutating: true},
		"updateProgressiveRolloutDraft":     {method: http.MethodPatch, mutating: true},
		"prepareProgressiveRollout":         {method: http.MethodPost, mutating: true},
		"commandProgressiveRollout":         {method: http.MethodPost, mutating: true},
		"approveProgressiveRolloutScope":    {method: http.MethodPost, mutating: true},
		"revokeProgressiveRolloutApproval":  {method: http.MethodDelete, mutating: true},
		"resolveProgressiveRolloutConflict": {method: http.MethodPost, mutating: true},
		"listProgressiveRolloutApprovals":   {method: http.MethodGet},
		"listProgressiveRolloutConflicts":   {method: http.MethodGet},
		"getProgressiveRolloutPlan":         {method: http.MethodGet},
		"listProgressiveRolloutEvents":      {method: http.MethodGet},
	}
	require.Len(t, nativeRolloutOperations, len(want))
	for operationID, metadata := range want {
		operation, found := nativeRolloutOperations[operationID]
		require.True(t, found, operationID)
		require.Equal(t, metadata.method, operation.method, operationID)
		require.Equal(t, metadata.mutating, operation.mutating, operationID)
	}
	require.NoError(t, validateNativeParameters("listProgressiveRollouts", nativeRolloutOperations["listProgressiveRollouts"], nil, map[string][]string{"state": {nativeTestFirstQuery, "two"}}, nil, "", http.StatusOK))
	require.ErrorContains(t, validateNativeParameters("listProgressiveRollouts", nativeRolloutOperations["listProgressiveRollouts"], map[string]string{"url": "https://attacker.invalid"}, nil, nil, "", http.StatusOK), "unexpected path")
	require.ErrorContains(t, validateNativeParameters("listProgressiveRollouts", nativeRolloutOperations["listProgressiveRollouts"], nil, map[string][]string{"url": {"https://attacker.invalid"}}, nil, "", http.StatusOK), "unexpected query")
}

func TestNativeProgressiveRolloutDiscoveryRequiresExactAdvertisedMetadata(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/orgs/demo/plugins" {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"items":[{"id":%q,"display_name":"Progressive Rollouts","state":"available","mode":"full","operations":[{"id":"createProgressiveRollout","method":"GET","mutating":true}]}]}`, progressiveRolloutsPluginID)
	}))
	t.Cleanup(server.Close)
	err := requireNativeProgressiveRolloutsOperation(pluginClientContext(t, server), "demo", "createProgressiveRollout", progressiveRolloutsMutate)
	require.ErrorContains(t, err, "unexpected transport metadata")
}

func TestNativeProgressiveRolloutMaterialUsesFrozenScopeWithoutInventoryLookup(t *testing.T) {
	t.Parallel()
	projectID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	environmentID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	deploymentID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	paths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths <- request.URL.Path
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "retained output")
	}))
	t.Cleanup(server.Close)
	detail := cp.ProgressiveRolloutDetail{
		Executions: []cp.ProgressiveRolloutEnvironmentExecution{{EnvironmentId: environmentID, DeploymentId: &deploymentID}},
		Plan:       &cp.ProgressiveRolloutPlan{Environments: []cp.ProgressiveRolloutPlannedEnvironment{{Uuid: environmentID, ProjectUuid: projectID}}},
	}
	body, status, err := nativeProgressiveRolloutMaterial(pluginClientContext(t, server), "demo", detail, deploymentID, "outputs")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "retained output", string(body))
	assert.Equal(t, "/orgs/demo/plugins/"+progressiveRolloutsPluginID+"/projects/"+projectID.String()+"/envs/"+environmentID.String()+"/deployments/"+deploymentID.String()+"/outputs", <-paths)
}

func TestNativeProgressiveRolloutMaterialRejectsUnboundDeployment(t *testing.T) {
	t.Parallel()
	detail := cp.ProgressiveRolloutDetail{Plan: &cp.ProgressiveRolloutPlan{}}
	_, _, err := nativeProgressiveRolloutMaterial(t.Context(), "demo", detail, uuid.New(), "logs")
	require.ErrorContains(t, err, "plugin_material_reference_missing")
}
