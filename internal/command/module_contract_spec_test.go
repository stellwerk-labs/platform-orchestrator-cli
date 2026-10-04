package command

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func contractDocument(t *testing.T, filename string) map[string]any {
	t.Helper()
	root, err := os.OpenRoot("../../clients/platform-orchestrator-cp")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	raw, err := root.ReadFile(filename)
	require.NoError(t, err)
	encoded, err := yaml.YAMLToJSON(raw)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(encoded, &result))
	return result
}

func TestQualifiedCoreContractLegacyOverlayDoesNotOverwriteCore(t *testing.T) {
	root, err := os.OpenRoot("../../clients/platform-orchestrator-cp")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	for filename, expected := range map[string]string{
		"spec.yaml":                   "a8e547bdaaa17c544330c1b68f034b65673f095f935e8e8a94d1aa7a17cf7777",
		"legacy-enterprise-spec.yaml": "41a1f9a231d9dc0c5850e381a44caa938330099b7ff730b0b834f8d950e2f1e1",
	} {
		raw, err := root.ReadFile(filename)
		require.NoError(t, err)
		require.Equal(t, expected, fmt.Sprintf("%x", sha256.Sum256(raw)))
	}
	core := contractDocument(t, "spec.yaml")
	legacy := contractDocument(t, "legacy-enterprise-spec.yaml")
	overlay := contractDocument(t, "legacy-rollouts-overlay.yaml")
	require.Equal(t, "1.0.0", overlay["overlay"])
	info := overlay["info"].(map[string]any)
	require.Equal(t, "3a10169d37b40f66adea76c530da81953a91b7ad", info["version"])
	require.Contains(t, info["title"], "not native Core routes")
	actions := overlay["actions"].([]any)
	require.Len(t, actions, 1)
	action := actions[0].(map[string]any)
	require.Equal(t, "$", action["target"])
	require.NotContains(t, action, "remove")
	require.Len(t, action, 3)
	update := action["update"].(map[string]any)
	require.Len(t, update, 2)
	paths := update["paths"].(map[string]any)
	corePaths, legacyPaths := core["paths"].(map[string]any), legacy["paths"].(map[string]any)
	expectedPaths := map[string]any{}
	for path, value := range legacyPaths {
		if path == "/orgs/{orgId}/rollouts" || strings.HasPrefix(path, "/orgs/{orgId}/rollouts/") {
			expectedPaths[path] = value
		}
	}
	require.NotEmpty(t, expectedPaths)
	require.Equal(t, expectedPaths, paths)
	for path, value := range paths {
		require.NotContains(t, corePaths, path)
		corePaths[path] = value
	}
	coreComponents := core["components"].(map[string]any)
	legacyComponents := legacy["components"].(map[string]any)
	components := update["components"].(map[string]any)
	added := map[string]bool{}
	for category, values := range components {
		current := coreComponents[category].(map[string]any)
		original := legacyComponents[category].(map[string]any)
		for name, value := range values.(map[string]any) {
			require.NotContains(t, current, name)
			require.Equal(t, original[name], value, category+"/"+name)
			current[name] = value
			added["#/components/"+category+"/"+name] = false
		}
	}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if reference, ok := typed["$ref"].(string); ok {
				if reached, exists := added[reference]; exists && !reached {
					added[reference] = true
					parts := strings.Split(reference, "/")
					walk(components[parts[2]].(map[string]any)[parts[3]])
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(paths)
	for reference, reachable := range added {
		require.True(t, reachable, reference)
	}
	encoded, err := json.Marshal(core)
	require.NoError(t, err)
	document, err := openapi3.NewLoader().LoadFromData(encoded)
	require.NoError(t, err, "all retained legacy and current Core references must resolve")
	require.NotNil(t, document)
}

func TestCoreOpenAPISpecValidatesWithExamples(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromFile("../../clients/platform-orchestrator-cp/spec.yaml")
	require.NoError(t, err)
	require.NoError(t, document.Validate(t.Context(), openapi3.EnableExamplesValidation()))
}

func TestRolloutExecutionResponseFieldsRoundTrip(t *testing.T) {
	const response = `{"executions":[{"environment_id":"11111111-1111-4111-8111-111111111111","project_id":"project","wave_id":"wave","state":"Failed","attempts":1,"pre_deployment_attempts":0,"changed":true,"actual_versions":{},"resource_version":2,"effective_base_deployment_id":"22222222-2222-4222-8222-222222222222","first_forward_deployment_id":"33333333-3333-4333-8333-333333333333","admission_base_deployment_id":"44444444-4444-4444-8444-444444444444","rollback_available":true,"rollback_retry_available":false,"rollback_retry_pending":false}]}`
	var detail cp.ProgressiveRolloutDetail
	require.NoError(t, json.Unmarshal([]byte(response), &detail))
	require.Len(t, detail.Executions, 1)
	execution := detail.Executions[0]
	require.NotNil(t, execution.FirstForwardDeploymentId)
	require.Equal(t, "33333333-3333-4333-8333-333333333333", execution.FirstForwardDeploymentId.String())
	require.NotNil(t, execution.AdmissionBaseDeploymentId)
	require.Equal(t, "44444444-4444-4444-8444-444444444444", execution.AdmissionBaseDeploymentId.String())
	require.NotNil(t, execution.RollbackAvailable)
	require.True(t, *execution.RollbackAvailable)
	require.NotNil(t, execution.RollbackRetryAvailable)
	require.False(t, *execution.RollbackRetryAvailable)
	require.NotNil(t, execution.RollbackRetryPending)
	require.False(t, *execution.RollbackRetryPending)

	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	var roundTrip map[string]any
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	roundTripExecution := roundTrip["executions"].([]any)[0].(map[string]any)
	require.Equal(t, "33333333-3333-4333-8333-333333333333", roundTripExecution["first_forward_deployment_id"])
	require.Equal(t, "44444444-4444-4444-8444-444444444444", roundTripExecution["admission_base_deployment_id"])
	require.Equal(t, true, roundTripExecution["rollback_available"])
	require.Equal(t, false, roundTripExecution["rollback_retry_available"])
	require.Equal(t, false, roundTripExecution["rollback_retry_pending"])
}
