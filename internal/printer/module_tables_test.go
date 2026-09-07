package printer

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	testModuleSemanticVersion = "1.2.3"
	testModuleDeprecated      = "deprecated"
	testModuleProposed        = "proposed"
	testModulePinActive       = "active"
	testModuleEnvironment     = "production"
	testModuleUnverified      = "unverified"
	testModuleProject         = "northstar"
)

func TestModuleManagementTablesUseRealResponseContracts(t *testing.T) {
	version := cp.CoreModuleVersion{Uuid: uuid.New(), SemanticVersion: ref.Ref(testModuleSemanticVersion), LifecycleStatus: testModuleProposed, VerificationStatus: "unverified"}
	pin := cp.EnvironmentModuleVersionPin{Id: uuid.New(), ProjectId: testModuleProject, EnvironmentId: testModuleEnvironment, VersionUuid: version.Uuid, Status: testModulePinActive}
	tests := []struct {
		name string
		data interface{}
		want []string
	}{
		{"catalogue", []cp.ModuleCatalogueEntry{{Slug: "redis", DisplayName: "Persistent Redis", Status: "archived"}}, []string{"Slug", "Persistent Redis", "archived"}},
		{"published version", version, []string{"SemanticVersion", testModuleSemanticVersion, testModuleProposed, testModuleUnverified, version.Uuid.String()}},
		{"version detail", cp.CoreModuleVersionDetail{Version: version}, []string{testModuleSemanticVersion, version.Uuid.String()}},
		{"version list", []cp.CoreModuleVersionDetail{{Version: version}}, []string{testModuleSemanticVersion, testModuleProposed}},
		{"legacy version", cp.CoreModuleVersion{MigrationGeneration: "v0", LifecycleStatus: testModuleDeprecated}, []string{"v0", testModuleDeprecated}},
		{"lifecycle history", []cp.ModuleVersionLifecycleEvent{{ToStatus: "defective", Reason: ref.Ref("Regression"), Actor: uuid.New()}}, []string{"ToStatus", "defective", "Regression", "Actor"}},
		{"pin detail", pin, []string{testModuleProject, testModuleEnvironment, testModulePinActive, version.Uuid.String()}},
		{"pin list", []cp.EnvironmentModuleVersionPin{pin}, []string{testModuleProject, testModuleEnvironment, testModulePinActive}},
		{"pin history", []cp.ModuleVersionPinEvent{{EventType: "note_added", Note: ref.Ref("Keep stable"), ToStatus: testModulePinActive}}, []string{"note_added", "Keep stable", testModulePinActive}},
		{"bulk preview", cp.ModuleVersionPinBulkPreview{Action: "pin", Eligible: false, Fingerprint: "frozen-plan", Items: []cp.ModuleVersionPinBulkPreviewItem{{ProjectId: testModuleProject, EnvironmentId: testModuleEnvironment, Production: true, Problem: ref.Ref("not authorised")}}}, []string{"Action: pin", "Eligible: false", "frozen-plan", testModuleEnvironment, "true", "not authorised"}},
		{"bulk result", cp.ModuleVersionPinBulkResult{Action: "pin", Pins: []cp.EnvironmentModuleVersionPin{pin}}, []string{"Operation:", "Action: pin", testModuleProject, testModuleEnvironment}},
		{"atomic lifecycle result", cp.ModuleVersionLifecycleTransactionResult{Versions: []cp.CoreModuleVersion{version}}, []string{"Correlation:", testModuleSemanticVersion, testModuleProposed}},
		{"adoption", cp.ModuleVersionUsage{SemanticVersion: testModuleSemanticVersion, ActiveEnvironmentCount: 1, ActivePins: 1, Environments: []cp.ModuleVersionUsageEnvironment{{ProjectId: testModuleProject, EnvironmentId: testModuleEnvironment}}}, []string{"Version: 1.2.3", "Active environments: 1", "1 active", testModuleProject, testModuleEnvironment}},
		{"no adoption", cp.ModuleVersionUsage{UnknownEnvironments: []uuid.UUID{version.Uuid}}, []string{"Active environments: 0", version.Uuid.String()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			require.NoError(t, (&TablePrinter{}).Write(&output, test.data))
			for _, expected := range test.want {
				require.Contains(t, output.String(), expected)
			}
		})
	}
}

func TestModuleComparisonTableShowsConcreteBeforeAndAfter(t *testing.T) {
	comparison := cp.ModuleVersionComparison{
		Before: cp.ModuleVersionComparisonSnapshot{ModuleSource: "inline", ModuleInputs: map[string]interface{}{"color": "blue", "unchanged": true}},
		After:  cp.ModuleVersionComparisonSnapshot{ModuleSource: "inline", ModuleInputs: map[string]interface{}{"color": "green", "unchanged": true}},
	}
	var output bytes.Buffer
	require.NoError(t, (&TablePrinter{}).Write(&output, comparison))
	for _, expected := range []string{"Field", "Before", "After", "module_inputs", `"color":"blue"`, `"color":"green"`} {
		require.Contains(t, output.String(), expected)
	}
	require.NotContains(t, output.String(), "module_source")
	comparison.After = comparison.Before
	output.Reset()
	require.NoError(t, (&TablePrinter{}).Write(&output, comparison))
	require.Contains(t, output.String(), "No definition changes.")
}

func TestStableGraduationTableShowsBothImmutableVersions(t *testing.T) {
	result := cp.StableModuleVersionSuccessorResult{
		CorrelationId: uuid.New(),
		Prerelease:    cp.CoreModuleVersion{Uuid: uuid.New(), SemanticVersion: ref.Ref("2.0.0-rc.1"), LifecycleStatus: testModuleDeprecated},
		Stable:        cp.CoreModuleVersion{Uuid: uuid.New(), SemanticVersion: ref.Ref("2.0.0"), LifecycleStatus: testModuleProposed},
	}
	var output bytes.Buffer
	require.NoError(t, (&TablePrinter{}).Write(&output, result))
	for _, expected := range []string{result.CorrelationId.String(), result.Prerelease.Uuid.String(), result.Stable.Uuid.String(), "2.0.0-rc.1", "2.0.0", testModuleDeprecated, testModuleProposed} {
		require.Contains(t, output.String(), expected)
	}
}

func TestModuleComparisonTablePreservesNullAndEmptyDifferences(t *testing.T) {
	rows, err := moduleVersionDiff(cp.ModuleVersionComparisonSnapshot{}, cp.ModuleVersionComparisonSnapshot{ModuleSourceCode: ref.Ref(""), ModuleInputs: map[string]interface{}{}})
	require.NoError(t, err)
	require.Equal(t, []moduleVersionDiffRow{
		{Field: "module_inputs", Before: "null", After: "{}"},
		{Field: "module_source_code", Before: "null", After: `""`},
	}, rows)
}

type moduleTableFailingWriter struct{}

func TestModuleComparisonOutputDeclarationAdditionAndRemoval(t *testing.T) {
	before := cp.ModuleVersionComparisonSnapshot{}
	after := cp.ModuleVersionComparisonSnapshot{OutputSchema: ref.Ref(cp.ModuleOutputSchema{})}
	rows, err := moduleVersionDiff(before, after)
	require.NoError(t, err)
	require.Equal(t, []moduleVersionDiffRow{{Field: "output_schema", Before: "(omitted)", After: "{}"}}, rows)
	rows, err = moduleVersionDiff(after, before)
	require.NoError(t, err)
	require.Equal(t, []moduleVersionDiffRow{{Field: "output_schema", Before: "{}", After: "(omitted)"}}, rows)
}

func (moduleTableFailingWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestModuleTableDoesNotSwallowOutputErrors(t *testing.T) {
	require.ErrorContains(t, (&TablePrinter{}).Write(moduleTableFailingWriter{}, cp.ModuleVersionPinBulkPreview{}), "closed output")
}
