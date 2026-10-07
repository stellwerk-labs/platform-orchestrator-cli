package command

import (
	"crypto/sha1" //nolint:gosec // Git object IDs are SHA-1 by definition.
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	rollouts "github.com/stellwerk-labs/platform-orchestrator-cli/clients/progressive-rollouts"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	coreContractDir      = "../../clients/platform-orchestrator-cp"
	dataPlaneContractDir = "../../clients/platform-orchestrator-dp"
)

func readVendoredFile(t *testing.T, dir, filename string) []byte {
	t.Helper()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	raw, err := root.ReadFile(filename)
	require.NoError(t, err)
	return raw
}

// recordedSourceValue returns the single "- <label>: `<value>`" entry of a
// vendored contract's SOURCE.md.
func recordedSourceValue(t *testing.T, dir, label string) string {
	t.Helper()
	pattern := regexp.MustCompile("(?m)^- " + regexp.QuoteMeta(label) + ": `([^`]+)`$")
	matches := pattern.FindAllSubmatch(readVendoredFile(t, dir, "SOURCE.md"), -1)
	require.Len(t, matches, 1, "%s/SOURCE.md must record exactly one %s", dir, label)
	return string(matches[0][1])
}

func contractDocument(t *testing.T, dir, filename string) map[string]any {
	t.Helper()
	encoded, err := yaml.YAMLToJSON(readVendoredFile(t, dir, filename))
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(encoded, &result))
	return result
}

// requireRecordedSource checks that a vendored server contract is the
// byte-identical document its SOURCE.md pins.
func requireRecordedSource(t *testing.T, dir, repository string) {
	t.Helper()
	raw := readVendoredFile(t, dir, "spec.yaml")
	require.Equal(t, recordedSourceValue(t, dir, "File SHA-256"), fmt.Sprintf("%x", sha256.Sum256(raw)),
		"spec.yaml must be the byte-identical document recorded in %s/SOURCE.md", dir)
	require.Regexp(t, "^[0-9a-f]{40}$", recordedSourceValue(t, dir, "Commit"))
	blob := sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...))
	require.Equal(t, recordedSourceValue(t, dir, "Git blob"), fmt.Sprintf("%x", blob), "Git blob in %s/SOURCE.md", dir)
	require.Equal(t, repository, recordedSourceValue(t, dir, "Repository"))
	require.Equal(t, "openapi/spec.yaml", recordedSourceValue(t, dir, "File"))
}

func TestCoreContractMatchesRecordedSource(t *testing.T) {
	requireRecordedSource(t, coreContractDir, "stellwerk-labs/platform-orchestrator-cp")
}

func TestDataPlaneContractMatchesRecordedSource(t *testing.T) {
	requireRecordedSource(t, dataPlaneContractDir, "stellwerk-labs/platform-orchestrator-dp")
}

func TestCoreOpenAPISpecValidatesWithExamples(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromFile(coreContractDir + "/spec.yaml")
	require.NoError(t, err)
	require.NoError(t, document.Validate(t.Context(), openapi3.EnableExamplesValidation()))
}

func TestCoreContractOwnsNoProgressiveRolloutsModel(t *testing.T) {
	core := contractDocument(t, coreContractDir, "spec.yaml")
	coreSchemas := core["components"].(map[string]any)["schemas"].(map[string]any)
	for name := range coreSchemas {
		require.NotContains(t, strings.ToLower(name), "progressiverollout", "Core must not define the plugin model %s", name)
	}
	operationIDs := map[string]bool{}
	for path, item := range core["paths"].(map[string]any) {
		require.NotContains(t, path, "/rollouts", "Core must not route plugin path %s", path)
		for _, operation := range item.(map[string]any) {
			if id, ok := operation.(map[string]any)["operationId"].(string); ok {
				operationIDs[id] = true
			}
		}
	}
	for _, id := range []string{"listPluginContributions", "getPluginCapability", "invokePluginOperation", "getPluginDeploymentMaterial"} {
		require.True(t, operationIDs[id], "Core must define %s", id)
	}
	rolloutSchemas := contractDocument(t, progressiveRolloutsContractDir, "spec.yaml")["components"].(map[string]any)["schemas"].(map[string]any)
	for name, schema := range rolloutSchemas {
		if _, mirrored := schema.(map[string]any)["x-stellwerk-core-schema"]; !mirrored {
			require.NotContains(t, coreSchemas, name, "plugin schema %s must not shadow a Core schema", name)
		}
	}
}

func TestRolloutExecutionResponseFieldsRoundTrip(t *testing.T) {
	const response = `{"executions":[{"environment_id":"11111111-1111-4111-8111-111111111111","project_id":"project","wave_id":"wave","state":"Failed","attempts":1,"pre_deployment_attempts":0,"changed":true,"actual_versions":{},"resource_version":2,"effective_base_deployment_id":"22222222-2222-4222-8222-222222222222","first_forward_deployment_id":"33333333-3333-4333-8333-333333333333","admission_base_deployment_id":"44444444-4444-4444-8444-444444444444","rollback_available":true,"rollback_retry_available":false,"rollback_retry_pending":false}]}`
	var detail rollouts.ProgressiveRolloutDetail
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
