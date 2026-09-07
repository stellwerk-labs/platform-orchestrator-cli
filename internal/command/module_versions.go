package command

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	moduleVersionIdempotencyFlag = "idempotency-key"
	moduleVersionActionFlag      = "action"
	moduleVersionReasonFlag      = "reason"
	moduleVersionExpectedFlag    = "expected-version"
	moduleVersionNoteFlag        = "note"
)

func moduleCommandIdempotencyKey(cmd *cobra.Command) string {
	value, _ := cmd.Flags().GetString(moduleVersionIdempotencyFlag)
	if value == "" {
		return uuid.NewString()
	}
	return value
}

func moduleCommandError(action string, status int, body []byte) error {
	return errors.Errorf("%s failed with HTTP %d: %s", action, status, body)
}

var CreateManagedModuleVersion = &cobra.Command{
	Use: "module-version <module-id>", Args: cobra.ExactArgs(1),
	Short: "Publish an immutable Proposed Module Version",
	Long:  fmt.Sprintf("Publish an immutable Proposed Module Version. Fields: %s.", generateTopLevelSetFields(cp.ModuleVersionPublishBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleVersionPublishBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).PublishModuleVersionWithResponse(cmd.Context(), orgID, args[0],
			&cp.PublishModuleVersionParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to publish Module Version")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("publish Module Version", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var GetManagedModuleVersion = &cobra.Command{
	Use: "module-version <module-id> <version>", Args: cobra.ExactArgs(2), Short: "Get one immutable Module Version",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).GetModuleVersionWithResponse(cmd.Context(), orgID, args[0], args[1])
		if err != nil {
			return errors.Wrap(err, "failed to get Module Version")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("get Module Version", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var ListManagedModuleVersions = &cobra.Command{
	Use: "module-versions <module-id>", Args: cobra.ExactArgs(1), Short: "List immutable Module Versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		includeDeprecated, _ := cmd.Flags().GetBool("include-deprecated")
		includeDefective, _ := cmd.Flags().GetBool("include-defective")
		response, err := MustCpClient(cmd.Context()).ListModuleVersionsWithResponse(cmd.Context(), orgID, args[0], &cp.ListModuleVersionsParams{
			IncludeDeprecated: ref.Ref(includeDeprecated), IncludeDefective: ref.Ref(includeDefective),
		})
		if err != nil {
			return errors.Wrap(err, "failed to list Module Versions")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("list Module Versions", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), response.JSON200.Items)
	},
}

var UpdateManagedModuleVersion = &cobra.Command{
	Use: "module-version <module-id> <version>", Args: cobra.ExactArgs(2), Short: "Promote, deprecate, mark Defective or restore a Module Version",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		action, _ := cmd.Flags().GetString(moduleVersionActionFlag)
		reason, _ := cmd.Flags().GetString(moduleVersionReasonFlag)
		expected, _ := cmd.Flags().GetInt64(moduleVersionExpectedFlag)
		response, err := MustCpClient(cmd.Context()).TransitionModuleVersionWithResponse(cmd.Context(), orgID, args[0], args[1],
			cp.TransitionModuleVersionParamsLifecycleAction(action),
			&cp.TransitionModuleVersionParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)},
			cp.ModuleReasonedCommand{ExpectedResourceVersion: expected, Reason: reason})
		if err != nil {
			return errors.Wrap(err, "failed to transition Module Version")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("transition Module Version", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var GetModuleVersionHistory = &cobra.Command{
	Use: "module-version-history <module-id> <version>", Args: cobra.ExactArgs(2), Short: "List append-only Module Version lifecycle events",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).ListModuleVersionLifecycleEventsWithResponse(cmd.Context(), orgID, args[0], args[1])
		if err != nil {
			return errors.Wrap(err, "failed to list Module Version history")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("list Module Version history", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var GetModuleVersionComparison = &cobra.Command{
	Use: "module-version-comparison <module-id> <from-version> <to-version>", Args: cobra.ExactArgs(3), Short: "Compare two immutable Module Version definitions",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).CompareModuleVersionsWithResponse(cmd.Context(), orgID, args[0], args[1], args[2])
		if err != nil {
			return errors.Wrap(err, "failed to compare Module Versions")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("compare Module Versions", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var GetModuleVersionUsage = &cobra.Command{
	Use: "module-version-usage <module-id> <version>", Args: cobra.ExactArgs(2), Short: "Inspect observed Environment adoption and Pins",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).GetModuleVersionUsageWithResponse(cmd.Context(), orgID, args[0], args[1])
		if err != nil {
			return errors.Wrap(err, "failed to get Module Version usage")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("get Module Version usage", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var CreateModuleCatalogueEntry = &cobra.Command{
	Use: "module-catalogue-entry", Args: cobra.NoArgs, Short: "Create an empty stable Module identity",
	Long: fmt.Sprintf("Create an empty Module catalogue entry. Fields: %s.", generateTopLevelSetFields(cp.ModuleCatalogueCreateBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleCatalogueCreateBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).CreateModuleCatalogueEntryWithResponse(cmd.Context(), orgID,
			&cp.CreateModuleCatalogueEntryParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create Module catalogue entry")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("create Module catalogue entry", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var GetModuleCatalogueEntry = &cobra.Command{
	Use: "module-catalogue-entry <module-id>", Args: cobra.ExactArgs(1), Short: "Get a stable Module catalogue identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).GetModuleCatalogueEntryWithResponse(cmd.Context(), orgID, args[0])
		if err != nil {
			return errors.Wrap(err, "failed to get Module catalogue entry")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("get Module catalogue entry", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var ListModuleCatalogueEntries = &cobra.Command{
	Use: "module-catalogue", Args: cobra.NoArgs, Short: "List stable Module catalogue identities",
	RunE: func(cmd *cobra.Command, _ []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		includeArchived, _ := cmd.Flags().GetBool("include-archived")
		response, err := MustCpClient(cmd.Context()).ListModuleCatalogueEntriesWithResponse(cmd.Context(), orgID,
			&cp.ListModuleCatalogueEntriesParams{IncludeArchived: ref.Ref(includeArchived)})
		if err != nil {
			return errors.Wrap(err, "failed to list Module catalogue")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("list Module catalogue", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var UpdateModuleCatalogueEntry = &cobra.Command{
	Use: "module-catalogue-entry <module-id>", Args: cobra.ExactArgs(1), Short: "Update mutable Module catalogue metadata",
	Long: fmt.Sprintf("Update Module catalogue metadata without publishing a version. Fields: %s.", generateTopLevelSetFields(cp.ModuleCatalogueUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleCatalogueUpdateBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).UpdateModuleCatalogueEntryWithResponse(cmd.Context(), orgID, args[0], *body)
		if err != nil {
			return errors.Wrap(err, "failed to update Module catalogue entry")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("update Module catalogue entry", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var UpdateModuleCatalogueStatus = &cobra.Command{
	Use: "module-catalogue-status <module-id>", Args: cobra.ExactArgs(1), Short: "Archive or unarchive a Module catalogue identity",
	RunE: func(cmd *cobra.Command, args []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		action, _ := cmd.Flags().GetString(moduleVersionActionFlag)
		reason, _ := cmd.Flags().GetString(moduleVersionReasonFlag)
		expected, _ := cmd.Flags().GetInt64(moduleVersionExpectedFlag)
		response, err := MustCpClient(cmd.Context()).ChangeModuleCatalogueStatusWithResponse(cmd.Context(), orgID, args[0],
			cp.ChangeModuleCatalogueStatusParamsCatalogueAction(action),
			&cp.ChangeModuleCatalogueStatusParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)},
			cp.ModuleReasonedCommand{Reason: reason, ExpectedResourceVersion: expected})
		if err != nil {
			return errors.Wrap(err, "failed to change Module catalogue status")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("change Module catalogue status", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var PublishStableModuleVersion = &cobra.Command{
	Use: "stable-module-version <module-id> <prerelease>", Args: cobra.ExactArgs(2), Short: "Atomically publish a stable successor and deprecate its prerelease",
	Long: fmt.Sprintf("Publish a stable successor atomically. Fields: %s.", generateTopLevelSetFields(cp.StableModuleVersionSuccessorBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := readSetFlagsIntoType[cp.StableModuleVersionSuccessorBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).PublishStableModuleVersionSuccessorWithResponse(cmd.Context(), orgID, args[0], args[1],
			&cp.PublishStableModuleVersionSuccessorParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to publish stable Module Version successor")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("publish stable Module Version successor", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var TransactModuleVersions = &cobra.Command{
	Use: "module-version-transaction", Args: cobra.NoArgs, Short: "Apply lifecycle transitions across Modules atomically",
	Long: fmt.Sprintf("Apply an atomic Module lifecycle transaction. Fields: %s.", generateTopLevelSetFields(cp.ModuleVersionLifecycleTransactionBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleVersionLifecycleTransactionBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).TransactModuleVersionLifecyclesWithResponse(cmd.Context(), orgID,
			&cp.TransactModuleVersionLifecyclesParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to transact Module Version lifecycles")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("transact Module Version lifecycles", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var PreviewModuleVersionPinBulk = &cobra.Command{
	Use: "module-version-pin-bulk-preview", Args: cobra.NoArgs, Short: "Preview an atomic frozen Environment Pin set",
	Long: fmt.Sprintf("Preview a Module Pin bulk operation. Fields: %s.", generateTopLevelSetFields(cp.ModuleVersionPinBulkPreviewBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleVersionPinBulkPreviewBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).PreviewEnvironmentModuleVersionPinBulkOperationWithResponse(cmd.Context(), orgID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to preview Module Version Pin bulk operation")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("preview Module Version Pin bulk operation", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var ExecuteModuleVersionPinBulk = &cobra.Command{
	Use: "module-version-pin-bulk", Args: cobra.NoArgs, Short: "Execute an atomic frozen Environment Pin set",
	Long: fmt.Sprintf("Execute a Module Pin bulk operation. Fields: %s.", generateTopLevelSetFields(cp.ModuleVersionPinBulkCommandBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleVersionPinBulkCommandBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).ExecuteEnvironmentModuleVersionPinBulkOperationWithResponse(cmd.Context(), orgID,
			&cp.ExecuteEnvironmentModuleVersionPinBulkOperationParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to execute Module Version Pin bulk operation")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("execute Module Version Pin bulk operation", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var CreateModuleVersionPin = &cobra.Command{
	Use: "module-version-pin", Args: cobra.NoArgs, Short: "Pin an Environment to its exact active Module Version",
	Long: fmt.Sprintf("Create an exact Environment Pin. Fields: %s.", generateTopLevelSetFields(cp.EnvironmentModuleVersionPinCreateBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := readSetFlagsIntoType[cp.EnvironmentModuleVersionPinCreateBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).CreateEnvironmentModuleVersionPinWithResponse(cmd.Context(), orgID,
			&cp.CreateEnvironmentModuleVersionPinParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create Module Version Pin")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("create Module Version Pin", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var ListModuleVersionPins = &cobra.Command{
	Use: "module-version-pins", Args: cobra.NoArgs, Short: "List authorised Environment Module Version Pins",
	RunE: func(cmd *cobra.Command, _ []string) error {
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		includeRemoved, _ := cmd.Flags().GetBool("include-removed")
		environmentValue, _ := cmd.Flags().GetString("environment-uuid")
		moduleValue, _ := cmd.Flags().GetString("module-uuid")
		var environmentUUID, moduleUUID *uuid.UUID
		if environmentValue != "" {
			parsed, parseErr := uuid.Parse(environmentValue)
			if parseErr != nil {
				return errors.Wrap(parseErr, "invalid Environment UUID")
			}
			environmentUUID = &parsed
		}
		if moduleValue != "" {
			parsed, parseErr := uuid.Parse(moduleValue)
			if parseErr != nil {
				return errors.Wrap(parseErr, "invalid Module UUID")
			}
			moduleUUID = &parsed
		}
		response, err := MustCpClient(cmd.Context()).ListEnvironmentModuleVersionPinsWithResponse(cmd.Context(), orgID,
			&cp.ListEnvironmentModuleVersionPinsParams{EnvironmentUuid: environmentUUID, ModuleUuid: moduleUUID, IncludeRemoved: ref.Ref(includeRemoved)})
		if err != nil {
			return errors.Wrap(err, "failed to list Module Version Pins")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("list Module Version Pins", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var GetModuleVersionPin = &cobra.Command{
	Use: "module-version-pin <pin-id>", Args: cobra.ExactArgs(1), Short: "Get one Environment Module Version Pin",
	RunE: func(cmd *cobra.Command, args []string) error {
		pinID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "invalid Pin UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).GetEnvironmentModuleVersionPinWithResponse(cmd.Context(), orgID, pinID)
		if err != nil {
			return errors.Wrap(err, "failed to get Module Version Pin")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("get Module Version Pin", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var GetModuleVersionPinHistory = &cobra.Command{
	Use: "module-version-pin-history <pin-id>", Args: cobra.ExactArgs(1), Short: "List append-only Pin lifecycle events",
	RunE: func(cmd *cobra.Command, args []string) error {
		pinID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "invalid Pin UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).ListEnvironmentModuleVersionPinEventsWithResponse(cmd.Context(), orgID, pinID)
		if err != nil {
			return errors.Wrap(err, "failed to list Module Version Pin history")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("list Module Version Pin history", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var UpdateModuleVersionPin = &cobra.Command{
	Use: "module-version-pin <pin-id>", Args: cobra.ExactArgs(1), Short: "Unpin or permanently discard an overridden Pin",
	RunE: func(cmd *cobra.Command, args []string) error {
		pinID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "invalid Pin UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		action, _ := cmd.Flags().GetString(moduleVersionActionFlag)
		reason, _ := cmd.Flags().GetString(moduleVersionReasonFlag)
		expected, _ := cmd.Flags().GetInt64(moduleVersionExpectedFlag)
		response, err := MustCpClient(cmd.Context()).TransitionEnvironmentModuleVersionPinWithResponse(cmd.Context(), orgID, pinID,
			cp.TransitionEnvironmentModuleVersionPinParamsPinAction(action),
			&cp.TransitionEnvironmentModuleVersionPinParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)},
			cp.ModuleReasonedCommand{ExpectedResourceVersion: expected, Reason: reason})
		if err != nil {
			return errors.Wrap(err, "failed to transition Module Version Pin")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("transition Module Version Pin", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var CreateModuleVersionPinNote = &cobra.Command{
	Use: "module-version-pin-note <pin-id>", Args: cobra.ExactArgs(1),
	Short: "Append an immutable note to a Module Version Pin",
	RunE: func(cmd *cobra.Command, args []string) error {
		pinID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "invalid Pin UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		note, _ := cmd.Flags().GetString(moduleVersionNoteFlag)
		response, err := MustCpClient(cmd.Context()).AppendEnvironmentModuleVersionPinNoteWithResponse(cmd.Context(), orgID, pinID,
			&cp.AppendEnvironmentModuleVersionPinNoteParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)},
			cp.ModuleVersionPinNoteBody{Note: note})
		if err != nil {
			return errors.Wrap(err, "failed to append Module Version Pin note")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("append Module Version Pin note", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

func init() {
	CreateManagedModuleVersion.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	CreateModuleCatalogueEntry.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	CreateCmd.AddCommand(CreateManagedModuleVersion, CreateModuleCatalogueEntry)

	ListManagedModuleVersions.Flags().Bool("include-deprecated", false, "Include Deprecated versions")
	ListManagedModuleVersions.Flags().Bool("include-defective", false, "Include Defective versions")
	ListModuleCatalogueEntries.Flags().Bool("include-archived", false, "Include archived Modules")
	PreviewModuleVersionPinBulk.Flags().String(createUpdateCmdSetJsonFlag, "", "Set JSON input as a raw string, stdin '-', or an @-prefixed file path")
	PreviewModuleVersionPinBulk.Flags().String(createUpdateCmdSetYamlFlag, "", "Set YAML input as a raw string, stdin '-', or an @-prefixed file path")
	PreviewModuleVersionPinBulk.Flags().StringArray(createUpdateCmdSetFlag, []string{}, "Set key=value pairs")
	PreviewModuleVersionPinBulk.MarkFlagsMutuallyExclusive(createUpdateCmdSetYamlFlag, createUpdateCmdSetJsonFlag)
	GetCmd.AddCommand(GetManagedModuleVersion, ListManagedModuleVersions, GetModuleVersionHistory, GetModuleVersionComparison, GetModuleVersionUsage,
		PreviewModuleVersionPinBulk, GetModuleCatalogueEntry, ListModuleCatalogueEntries)

	PublishStableModuleVersion.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	TransactModuleVersions.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	CreateCmd.AddCommand(PublishStableModuleVersion, TransactModuleVersions)

	UpdateManagedModuleVersion.Flags().String(moduleVersionActionFlag, "", "Lifecycle action: promote, deprecate, mark-defective or restore")
	UpdateManagedModuleVersion.Flags().String(moduleVersionReasonFlag, "", "Mandatory audited reason")
	UpdateManagedModuleVersion.Flags().Int64(moduleVersionExpectedFlag, 0, "Expected Module Version resource version")
	UpdateManagedModuleVersion.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	_ = UpdateManagedModuleVersion.MarkFlagRequired(moduleVersionActionFlag)
	_ = UpdateManagedModuleVersion.MarkFlagRequired(moduleVersionReasonFlag)
	_ = UpdateManagedModuleVersion.MarkFlagRequired(moduleVersionExpectedFlag)
	UpdateCmd.AddCommand(UpdateManagedModuleVersion)
	UpdateModuleCatalogueStatus.Flags().String(moduleVersionActionFlag, "", "Catalogue action: archive or unarchive")
	UpdateModuleCatalogueStatus.Flags().String(moduleVersionReasonFlag, "", "Mandatory audited reason")
	UpdateModuleCatalogueStatus.Flags().Int64(moduleVersionExpectedFlag, 0, "Expected Module catalogue resource version")
	UpdateModuleCatalogueStatus.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	_ = UpdateModuleCatalogueStatus.MarkFlagRequired(moduleVersionActionFlag)
	_ = UpdateModuleCatalogueStatus.MarkFlagRequired(moduleVersionReasonFlag)
	_ = UpdateModuleCatalogueStatus.MarkFlagRequired(moduleVersionExpectedFlag)
	UpdateCmd.AddCommand(UpdateModuleCatalogueEntry, UpdateModuleCatalogueStatus)

	CreateModuleVersionPin.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	ExecuteModuleVersionPinBulk.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	CreateModuleVersionPinNote.Flags().String(moduleVersionNoteFlag, "", "Immutable note text")
	CreateModuleVersionPinNote.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	_ = CreateModuleVersionPinNote.MarkFlagRequired(moduleVersionNoteFlag)
	CreateCmd.AddCommand(CreateModuleVersionPin, CreateModuleVersionPinNote, ExecuteModuleVersionPinBulk)
	ListModuleVersionPins.Flags().Bool("include-removed", false, "Include removed historical Pins")
	ListModuleVersionPins.Flags().String("environment-uuid", "", "Filter to one exact Environment UUID")
	ListModuleVersionPins.Flags().String("module-uuid", "", "Filter to one exact Module UUID")
	GetCmd.AddCommand(ListModuleVersionPins, GetModuleVersionPin, GetModuleVersionPinHistory)
	UpdateModuleVersionPin.Flags().String(moduleVersionActionFlag, "", "Pin action: unpin or discard")
	UpdateModuleVersionPin.Flags().String(moduleVersionReasonFlag, "", "Mandatory audited reason")
	UpdateModuleVersionPin.Flags().Int64(moduleVersionExpectedFlag, 0, "Expected Pin resource version")
	UpdateModuleVersionPin.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	_ = UpdateModuleVersionPin.MarkFlagRequired(moduleVersionActionFlag)
	_ = UpdateModuleVersionPin.MarkFlagRequired(moduleVersionReasonFlag)
	_ = UpdateModuleVersionPin.MarkFlagRequired(moduleVersionExpectedFlag)
	UpdateCmd.AddCommand(UpdateModuleVersionPin)
}
