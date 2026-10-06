package command

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
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

func TestQualifiedCoreContractSpecMatchesPinnedSHA(t *testing.T) {
	root, err := os.OpenRoot("../../clients/platform-orchestrator-cp")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	raw, err := root.ReadFile("spec.yaml")
	require.NoError(t, err)
	require.Equal(t, "af2e0ee1dcdb8196cc25469287da3e331d225a14e72c5cb927d668742100c4d9", fmt.Sprintf("%x", sha256.Sum256(raw)),
		"spec.yaml SHA has changed; update the pin in check-api-specs.sh and this test")
}

func TestCoreOpenAPISpecValidatesWithExamples(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromFile("../../clients/platform-orchestrator-cp/spec.yaml")
	require.NoError(t, err)
	require.NoError(t, document.Validate(t.Context(), openapi3.EnableExamplesValidation()))
}

func TestRolloutSchemasLiveInCoreSpecWithoutConflict(t *testing.T) {
	core := contractDocument(t, "spec.yaml")
	schemas, ok := core["components"].(map[string]any)["schemas"].(map[string]any)
	require.True(t, ok)
	for _, name := range []string{
		"ProgressiveRollout", "ProgressiveRolloutDetail", "ProgressiveRolloutSpec",
		"ProgressiveRolloutPage", "ProgressiveRolloutEvent", "ProgressiveRolloutPlan",
	} {
		require.Contains(t, schemas, name, "rollout type %s must be in spec.yaml components/schemas", name)
	}
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
