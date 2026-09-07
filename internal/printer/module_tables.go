package printer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

const (
	tableFieldResourceVersion = "ResourceVersion"
	tableFieldEnvironmentID   = "EnvironmentId"
)

var moduleTableColumns = map[string][]string{
	"ModuleCatalogueEntry":            {"Slug", tableFieldUuid, tableFieldDisplayName, tableFieldResourceType, tableFieldStatus, "CurrentDefaultVersionUuid", tableFieldResourceVersion},
	"CoreModuleVersion":               {"SemanticVersion", "MigrationGeneration", "LifecycleStatus", "VerificationStatus", tableFieldUuid, tableFieldResourceVersion, tableFieldCreatedAt},
	"ModuleVersionLifecycleEvent":     {"Sequence", "FromStatus", "ToStatus", "Reason", "Actor", tableFieldCreatedAt},
	"EnvironmentModuleVersionPin":     {tableFieldId, tableFieldProjectId, tableFieldEnvironmentID, "ModuleUuid", "VersionUuid", tableFieldStatus, tableFieldResourceVersion},
	"ModuleVersionPinEvent":           {"Sequence", "EventType", "FromStatus", "ToStatus", "Reason", "Note", "Actor", tableFieldCreatedAt},
	"ModuleVersionPinBulkPreviewItem": {tableFieldProjectId, tableFieldEnvironmentID, "EnvironmentUuid", "EnvironmentType", "Production", "VersionUuid", "Eligible", "Problem"},
	"ModuleVersionUsageEnvironment":   {tableFieldProjectId, tableFieldEnvironmentID, "EnvironmentType", "DeploymentId", "ObservedAt"},
	"moduleVersionDiffRow":            {"Field", "Before", "After"},
}

// Project nested response envelopes only for the human-readable table format.
// JSON and YAML retain the complete, unchanged server contract.
func moduleTableValue(w io.Writer, item interface{}) (interface{}, error) {
	switch value := item.(type) {
	case cp.CoreModuleVersionDetail:
		return value.Version, nil
	case []cp.CoreModuleVersionDetail:
		versions := make([]cp.CoreModuleVersion, len(value))
		for index := range value {
			versions[index] = value[index].Version
		}
		return versions, nil
	case cp.ModuleVersionLifecycleTransactionResult:
		_, err := fmt.Fprintf(w, "Correlation: %s\n", value.CorrelationId)
		return value.Versions, err
	case cp.StableModuleVersionSuccessorResult:
		_, err := fmt.Fprintf(w, "Correlation: %s\n", value.CorrelationId)
		return []cp.CoreModuleVersion{value.Prerelease, value.Stable}, err
	case cp.ModuleVersionPinBulkPreview:
		_, err := fmt.Fprintf(w, "Action: %s\nModule: %s\nEligible: %t\nPreview fingerprint: %s\n", value.Action, value.ModuleUuid, value.Eligible, value.Fingerprint)
		return value.Items, err
	case cp.ModuleVersionPinBulkResult:
		_, err := fmt.Fprintf(w, "Operation: %s\nAction: %s\n", value.OperationId, value.Action)
		return value.Pins, err
	case cp.ModuleVersionUsage:
		_, err := fmt.Fprintf(w, "Version: %s (%s)\nObserved: %s\nActive environments: %d\nPins: %d active, %d override pending, %d historical\nUnknown environments: %v\n",
			value.SemanticVersion, value.VersionUuid, value.ObservedAt, value.ActiveEnvironmentCount,
			value.ActivePins, value.OverridePendingPins, value.HistoricalPins, value.UnknownEnvironments)
		return value.Environments, err
	case cp.ModuleVersionComparison:
		if _, err := fmt.Fprintf(w, "Compare: %s -> %s\n", value.FromVersionUuid, value.ToVersionUuid); err != nil {
			return nil, err
		}
		rows, err := moduleVersionDiff(value.Before, value.After)
		if err == nil && len(rows) == 0 {
			_, err = fmt.Fprintln(w, "No definition changes.")
		}
		return rows, err
	default:
		return item, nil
	}
}

type moduleVersionDiffRow struct {
	Field  string
	Before string
	After  string
}

func moduleVersionDiff(before, after cp.ModuleVersionComparisonSnapshot) ([]moduleVersionDiffRow, error) {
	encode := func(snapshot cp.ModuleVersionComparisonSnapshot) (map[string]json.RawMessage, error) {
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		err = json.Unmarshal(data, &fields)
		return fields, err
	}
	beforeFields, err := encode(before)
	if err != nil {
		return nil, err
	}
	afterFields, err := encode(after)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(beforeFields))
	for key := range beforeFields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	rows := make([]moduleVersionDiffRow, 0, len(keys))
	for _, key := range keys {
		if !bytes.Equal(beforeFields[key], afterFields[key]) {
			rows = append(rows, moduleVersionDiffRow{Field: key, Before: string(beforeFields[key]), After: string(afterFields[key])})
		}
	}
	return rows, nil
}
