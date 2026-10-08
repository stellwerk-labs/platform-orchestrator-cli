package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	rollouts "github.com/stellwerk-labs/platform-orchestrator-cli/clients/progressive-rollouts"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
)

const (
	nativeTestBodyField     = "name"
	nativeTestFirstQuery    = "one"
	nativeTestOrg           = "demo"
	nativeTestDiscoveryPath = "/orgs/" + nativeTestOrg + "/plugins"
)

func cpClientContext(t *testing.T, server *httptest.Server) context.Context {
	t.Helper()
	client, err := cp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	return context.WithValue(t.Context(), CpClientContextKey, client)
}

func TestNativeProgressiveRolloutInvocationPreservesClosedEnvelope(t *testing.T) {
	t.Parallel()
	type requestRecord struct {
		path       string
		invocation cp.PluginInvocation
	}
	records := make(chan requestRecord, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/orgs/demo/plugins/"+progressiveRolloutsPluginID+"/invoke" {
			http.NotFound(w, request)
			return
		}
		var invocation cp.PluginInvocation
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
	result, err := invokeNativeProgressiveRollout[map[string]string](cpClientContext(t, server), "demo", "createProgressiveRollout", nil, nil, body, "stable-key", http.StatusCreated)
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
	err := requireNativeProgressiveRolloutsOperation(cpClientContext(t, server), "demo", "createProgressiveRollout", progressiveRolloutsMutate)
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
	detail := rollouts.ProgressiveRolloutDetail{
		Executions: []rollouts.ProgressiveRolloutEnvironmentExecution{{EnvironmentId: environmentID, DeploymentId: &deploymentID}},
		Plan:       &rollouts.ProgressiveRolloutPlan{Environments: []rollouts.ProgressiveRolloutPlannedEnvironment{{Uuid: environmentID, ProjectUuid: projectID}}},
	}
	body, status, err := nativeProgressiveRolloutMaterial(cpClientContext(t, server), "demo", detail, deploymentID, "outputs")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "retained output", string(body))
	assert.Equal(t, "/orgs/demo/plugins/"+progressiveRolloutsPluginID+"/projects/"+projectID.String()+"/envs/"+environmentID.String()+"/deployments/"+deploymentID.String()+"/outputs", <-paths)
}

func TestNativeProgressiveRolloutMaterialRejectsUnboundDeployment(t *testing.T) {
	t.Parallel()
	detail := rollouts.ProgressiveRolloutDetail{Plan: &rollouts.ProgressiveRolloutPlan{}}
	_, _, err := nativeProgressiveRolloutMaterial(t.Context(), "demo", detail, uuid.New(), "logs")
	require.ErrorContains(t, err, "plugin_material_reference_missing")
}

const graceTestWarning = `{"code":"plugin_entitlement_grace","message":"The signed plugin licence is in its grace period.","grace_until":"2026-11-01T00:00:00Z","features":["progressive-rollouts"]}`

// graceWarningServer advertises listProgressiveRollouts and answers it; both
// discovery and the invocation carry the grace warning when asked to.
func graceWarningServer(t *testing.T, discoveryWarns, invokeWarns bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		warning := func(enabled bool, field string) string {
			if !enabled {
				return ""
			}
			return fmt.Sprintf(`,%q:%s`, field, graceTestWarning)
		}
		switch request.URL.Path {
		case "/orgs/demo/plugins":
			_, _ = fmt.Fprintf(w, `{"contract_level":"v1alpha","items":[{"id":%q,"display_name":"Progressive Rollouts","state":"available","mode":"full","operations":[{"id":"listProgressiveRollouts","method":"GET","mutating":false}],"entitlement":{"features":{}%s}}]}`,
				progressiveRolloutsPluginID, warning(discoveryWarns, "warning"))
		case "/orgs/demo/plugins/" + progressiveRolloutsPluginID + "/invoke":
			_, _ = fmt.Fprintf(w, `{"status":200,"body":{"items":[]}%s}`, warning(invokeWarns, "entitlement_warning"))
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPluginGraceWarningIsShownOnceAndNeverFails(t *testing.T) {
	t.Parallel()
	for name, warns := range map[string][2]bool{
		"discovery and invoke": {true, true},
		"discovery only":       {true, false},
		"invoke only":          {false, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stderr strings.Builder
			ctx := withPluginWarnings(cpClientContext(t, graceWarningServer(t, warns[0], warns[1])), &stderr)
			require.NoError(t, requireNativeProgressiveRolloutsOperation(ctx, "demo", "listProgressiveRollouts", progressiveRolloutsRead))
			page, err := invokeNativeProgressiveRollout[rollouts.ProgressiveRolloutPage](ctx, "demo", "listProgressiveRollouts", nil, nil, nil, "", http.StatusOK)
			require.NoError(t, err)
			require.Empty(t, page.Items)
			require.Equal(t, "Warning: The signed plugin licence is in its grace period. (plugin_entitlement_grace, grace until 2026-11-01T00:00:00Z)\n", stderr.String())
		})
	}
	t.Run("no warning", func(t *testing.T) {
		t.Parallel()
		var stderr strings.Builder
		ctx := withPluginWarnings(cpClientContext(t, graceWarningServer(t, false, false)), &stderr)
		require.NoError(t, requireNativeProgressiveRolloutsOperation(ctx, "demo", "listProgressiveRollouts", progressiveRolloutsRead))
		_, err := invokeNativeProgressiveRollout[rollouts.ProgressiveRolloutPage](ctx, "demo", "listProgressiveRollouts", nil, nil, nil, "", http.StatusOK)
		require.NoError(t, err)
		require.Empty(t, stderr.String())
	})
}

func TestRolloutCommandPrintsTheGraceWarningOnceToStderr(t *testing.T) {
	server := graceWarningServer(t, true, true)
	ctx := context.WithValue(cpClientContext(t, server), ConfigContextKey, config.Config{DefaultOrg: "demo"})
	t.Setenv("PO_API_URL", server.URL)
	cobra.EnableTraverseRunHooks = true

	stdout, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, "demo", testGetCmd, "rollouts", outFlag, jsonOutput})
	require.NoError(t, err)
	require.JSONEq(t, `[]`, stdout)
	require.Equal(t, 1, strings.Count(stderr, "Warning: The signed plugin licence is in its grace period."))
	require.Contains(t, stderr, "grace until 2026-11-01T00:00:00Z")
}

const (
	licenceTestAction = " Ask a platform operator to import a current licence for this organization with 'octl plugin-manager import-entitlements'."
	licenceTestPrefix = "plugin_not_entitled: Progressive Rollouts (" + progressiveRolloutsPluginID + ") is not_entitled. "
)

// notEntitledServer advertises the plugin as Core does once no licence allows
// forward work: not_entitled, safety_only, only read and safety operations,
// and the entitlement projection given as JSON. Any invocation is an error.
func notEntitledServer(t *testing.T, entitlement string) *httptest.Server {
	t.Helper()
	if entitlement != "" {
		entitlement = `,"entitlement":` + entitlement
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != nativeTestDiscoveryPath {
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"contract_level":"v1alpha","items":[{"id":%q,"display_name":"Progressive Rollouts","state":"not_entitled","mode":"safety_only","operations":[`+
			`{"id":"listProgressiveRollouts","method":"GET","mutating":false,"required_feature":"progressive-rollouts"}]%s}]}`, progressiveRolloutsPluginID, entitlement)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestNotEntitledNamesTheLicenceStateCoreReports(t *testing.T) {
	t.Parallel()
	feature := func(state, extra string) string {
		return `{"features":{"progressive-rollouts":{"state":"` + state + `","trust_class":"development","forward":false,"read":true,"safety":true` + extra + `}}}`
	}
	for name, tc := range map[string]struct{ entitlement, reason string }{
		"expired after grace": {
			entitlement: feature("expired", `,"expires_at":"2026-10-07T22:54:33Z","grace_until":"2026-10-08T01:04:33+02:00"`),
			reason:      "The licence for feature progressive-rollouts expired at 2026-10-07T22:54:33Z; its grace period ended at 2026-10-07T23:04:33Z.",
		},
		"expired without dates": {entitlement: feature("expired", ""), reason: "The licence for feature progressive-rollouts expired."},
		"revoked":               {entitlement: feature("revoked", ""), reason: "The licence for feature progressive-rollouts was revoked."},
		"not yet valid":         {entitlement: feature("not_yet_valid", ""), reason: "The licence for feature progressive-rollouts is not valid yet."},
		"untrusted":             {entitlement: feature("untrusted", ""), reason: "The licence for feature progressive-rollouts no longer verifies against the licence issuers this Control Plane trusts."},
		"missing":               {entitlement: feature("missing", ""), reason: "No licence covers feature progressive-rollouts for this organization."},
		"not entitled":          {entitlement: feature("not_entitled", ""), reason: "No licence covers feature progressive-rollouts for this organization."},
		"unknown state":         {entitlement: feature("suspended", ""), reason: "The licence for feature progressive-rollouts is suspended."},
		"only refusing features": {
			entitlement: `{"features":{"z-reports":{"state":"valid","trust_class":"production","forward":true,"read":true,"safety":true},` +
				`"progressive-rollouts":{"state":"expired","trust_class":"production","forward":false,"read":true,"safety":true},` +
				`"approvals":{"state":"missing","trust_class":"","forward":false,"read":false,"safety":false}}}`,
			reason: "No licence covers feature approvals for this organization. The licence for feature progressive-rollouts expired.",
		},
		"no projection":    {reason: "No current licence covers this organization: it is missing or expired."},
		"no feature state": {entitlement: `{"features":{}}`, reason: "No current licence covers this organization: it is missing or expired."},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := cpClientContext(t, notEntitledServer(t, tc.entitlement))
			err := requireNativeProgressiveRolloutsOperation(ctx, nativeTestOrg, "commandProgressiveRollout", progressiveRolloutsMutate)
			require.EqualError(t, err, licenceTestPrefix+tc.reason+licenceTestAction)
			require.NoError(t, requireNativeProgressiveRolloutsOperation(ctx, nativeTestOrg, "listProgressiveRollouts", progressiveRolloutsRead), "reads stay available")
		})
	}
}

func TestRolloutForwardCommandSaysWhenTheLicenceExpired(t *testing.T) {
	server := notEntitledServer(t, `{"features":{"progressive-rollouts":{"state":"expired","trust_class":"development","forward":false,"read":true,"safety":true,"expires_at":"2026-10-07T22:54:33Z","grace_until":"2026-10-07T23:04:33Z"}}}`)
	ctx := context.WithValue(cpClientContext(t, server), ConfigContextKey, config.Config{DefaultOrg: nativeTestOrg})
	t.Setenv("PO_API_URL", server.URL)
	cobra.EnableTraverseRunHooks = true

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, nativeTestOrg, "rollout", "resume", uuid.NewString(), "--" + rolloutExpectedVersionFlag, "3", noPromptFlag})
	require.EqualError(t, err, licenceTestPrefix+"The licence for feature progressive-rollouts expired at 2026-10-07T22:54:33Z; its grace period ended at 2026-10-07T23:04:33Z."+licenceTestAction)
	require.NotContains(t, stderr, "Warning:")
}
