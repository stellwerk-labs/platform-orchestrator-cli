package command

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const progressiveRolloutsContractDir = "../../clients/progressive-rollouts"

// contractOperations returns the Operation Objects of x-stellwerk-operations by
// operation ID.
func contractOperations(t *testing.T, document map[string]any) map[string]map[string]any {
	t.Helper()
	entries, ok := document["x-stellwerk-operations"].([]any)
	require.True(t, ok, "x-stellwerk-operations must be a list")
	operations := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		operation, ok := entry.(map[string]any)
		require.True(t, ok)
		id, ok := operation["operationId"].(string)
		require.True(t, ok, "every operation needs an operationId")
		require.NotContains(t, operations, id, "duplicate operation %s", id)
		operations[id] = operation
	}
	return operations
}

// contractParameters resolves an operation's parameters into name lists per
// envelope location.
func contractParameters(t *testing.T, document, operation map[string]any) (pathNames, queryNames []string) {
	t.Helper()
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if ref, ok := parameter["$ref"].(string); ok {
			parameter = resolveContractRef(t, document, ref).(map[string]any)
		}
		name := parameter["name"].(string)
		switch parameter["in"] {
		case "path":
			pathNames = append(pathNames, name)
		case "query":
			queryNames = append(queryNames, name)
		default:
			t.Fatalf("%s: parameter %s in %v does not map onto the invoke envelope", operation["operationId"], name, parameter["in"])
		}
	}
	return pathNames, queryNames
}

func resolveContractRef(t *testing.T, document map[string]any, ref string) any {
	t.Helper()
	require.True(t, strings.HasPrefix(ref, "#/components/"), "reference %s must be local", ref)
	var current any = document
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		object, ok := current.(map[string]any)
		require.True(t, ok, "reference %s does not resolve", ref)
		current = object[part]
	}
	require.NotNil(t, current, "reference %s does not resolve", ref)
	return current
}

func collectContractRefs(value any, refs map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok {
			refs[ref] = struct{}{}
		}
		for _, nested := range typed {
			collectContractRefs(nested, refs)
		}
	case []any:
		for _, nested := range typed {
			collectContractRefs(nested, refs)
		}
	}
}

func contractSuccessStatus(t *testing.T, operation map[string]any) int {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	require.True(t, ok)
	var statuses []int
	for code := range responses {
		if code == "default" {
			continue
		}
		status, err := strconv.Atoi(code)
		require.NoError(t, err)
		statuses = append(statuses, status)
	}
	require.Len(t, statuses, 1, "%s must document exactly one success status", operation["operationId"])
	return statuses[0]
}

func TestProgressiveRolloutsContractMatchesRecordedSource(t *testing.T) {
	raw := readVendoredFile(t, progressiveRolloutsContractDir, "spec.yaml")
	require.Equal(t, recordedSourceValue(t, progressiveRolloutsContractDir, "File SHA-256"), fmt.Sprintf("%x", sha256.Sum256(raw)),
		"spec.yaml must be the byte-identical contract recorded in SOURCE.md")
}

func TestProgressiveRolloutsContractValidatesWithExamples(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromFile(progressiveRolloutsContractDir + "/spec.yaml")
	require.NoError(t, err)
	require.NoError(t, document.Validate(t.Context(), openapi3.EnableExamplesValidation()))
	assert.Equal(t, "Stellwerk Progressive Rollouts plugin API", document.Info.Title)
	assert.Equal(t, "v1alpha", document.Info.Version)
	assert.Equal(t, progressiveRolloutsPluginID, document.Extensions["x-stellwerk-plugin-id"])
	assert.Zero(t, document.Paths.Len(), "the plugin publishes no client routes")
	assert.Empty(t, document.Servers)
}

// The operations live in an extension, which OpenAPI validators skip. Placing
// them under synthetic paths validates each one as a strict Operation Object
// whose references resolve.
func TestProgressiveRolloutsContractOperationsAreValidOperationObjects(t *testing.T) {
	document := contractDocument(t, progressiveRolloutsContractDir, "spec.yaml")
	operations := contractOperations(t, document)
	paths := make(map[string]any, len(operations))
	for id, operation := range operations {
		method, ok := operation["x-method"].(string)
		require.True(t, ok, id)
		require.Contains(t, []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete}, method, id)
		pathNames, _ := contractParameters(t, document, operation)
		path := "/" + id
		for _, name := range pathNames {
			path += "/{" + name + "}"
		}
		paths[path] = map[string]any{strings.ToLower(method): operation}
	}
	synthetic := maps.Clone(document)
	delete(synthetic, "x-stellwerk-operations")
	synthetic["paths"] = paths
	encoded, err := json.Marshal(synthetic)
	require.NoError(t, err)
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	loaded, err := loader.LoadFromData(encoded)
	require.NoError(t, err)
	require.NoError(t, loaded.Validate(t.Context(), openapi3.EnableExamplesValidation()))
}

func TestProgressiveRolloutsContractIsClosedOverItsOperations(t *testing.T) {
	document := contractDocument(t, progressiveRolloutsContractDir, "spec.yaml")
	reachable := map[string]struct{}{}
	pending := map[string]struct{}{}
	collectContractRefs(document["x-stellwerk-operations"], pending)
	for len(pending) > 0 {
		next := map[string]struct{}{}
		for ref := range pending {
			if _, seen := reachable[ref]; seen {
				continue
			}
			reachable[ref] = struct{}{}
			collectContractRefs(resolveContractRef(t, document, ref), next)
		}
		pending = next
	}
	declared := map[string]struct{}{}
	for kind, values := range document["components"].(map[string]any) {
		for name := range values.(map[string]any) {
			declared["#/components/"+kind+"/"+name] = struct{}{}
		}
	}
	assert.Equal(t, slices.Sorted(maps.Keys(declared)), slices.Sorted(maps.Keys(reachable)),
		"components must be exactly what the operations reference")
}

func TestProgressiveRolloutsContractOwnsRolloutModels(t *testing.T) {
	document := contractDocument(t, progressiveRolloutsContractDir, "spec.yaml")
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{
		"ProgressiveRollout", "ProgressiveRolloutDetail", "ProgressiveRolloutSpec",
		"ProgressiveRolloutPage", "ProgressiveRolloutEvent", "ProgressiveRolloutPlan",
	} {
		require.Contains(t, schemas, name, "rollout type %s must be in the plugin contract", name)
	}
}

func TestProgressiveRolloutsContractMirrorsCoreValueSets(t *testing.T) {
	rollouts := contractDocument(t, progressiveRolloutsContractDir, "spec.yaml")["components"].(map[string]any)["schemas"].(map[string]any)
	core := contractDocument(t, coreContractDir, "spec.yaml")["components"].(map[string]any)["schemas"].(map[string]any)
	var mirrors []string
	for name, raw := range rollouts {
		schema := raw.(map[string]any)
		coreName, mirrored := schema["x-stellwerk-core-schema"]
		if !mirrored {
			require.True(t, strings.HasPrefix(name, "ProgressiveRollout"), "schema %s is neither plugin-owned nor a marked Core mirror", name)
			continue
		}
		require.Equal(t, name, coreName)
		mirror := maps.Clone(schema)
		delete(mirror, "x-stellwerk-core-schema")
		delete(mirror, "description")
		require.Equal(t, core[name], mirror, "mirror %s must equal the Core definition", name)
		mirrors = append(mirrors, name)
	}
	assert.ElementsMatch(t, []string{"ModuleVersionPinStatus", "ModuleVersionSemanticStatus"}, mirrors)
}

func TestProgressiveRolloutsContractCoversCLIOperations(t *testing.T) {
	document := contractDocument(t, progressiveRolloutsContractDir, "spec.yaml")
	operations := contractOperations(t, document)
	for id, expected := range nativeRolloutOperations {
		operation, found := operations[id]
		require.True(t, found, "CLI operation %s is not in the plugin contract", id)
		assert.Equal(t, expected.method, operation["x-method"], id)
		assert.Equal(t, expected.mutating, operation["x-mutating"], id)
		pathNames, queryNames := contractParameters(t, document, operation)
		assert.ElementsMatch(t, expected.pathParameters, pathNames, id)
		assert.ElementsMatch(t, expected.queryParameters, queryNames, id)
		_, hasBody := operation["requestBody"]
		assert.Equal(t, expected.bodyRequired, hasBody, id)
		assert.Equal(t, expected.responseStatus, contractSuccessStatus(t, operation), id)
	}
	unused := slices.DeleteFunc(slices.Sorted(maps.Keys(operations)), func(id string) bool {
		_, used := nativeRolloutOperations[id]
		return used
	})
	assert.Equal(t, []string{"listProgressiveRolloutTargetEnvironments"}, unused, "the CLI deliberately offers no target discovery command")

	safety := operations["commandProgressiveRollout"]["x-safety-path-values"].(map[string]any)["rolloutCommand"].([]any)
	for _, raw := range operations["commandProgressiveRollout"]["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["name"] != "rolloutCommand" {
			continue
		}
		for _, command := range parameter["schema"].(map[string]any)["enum"].([]any) {
			expected := progressiveRolloutsMutate
			if slices.Contains(safety, command) {
				expected = progressiveRolloutsSafety
			}
			assert.Equal(t, expected, progressiveRolloutCommandAccess(command.(string)), command)
		}
	}
}
