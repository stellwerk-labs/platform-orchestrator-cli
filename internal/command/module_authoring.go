package command

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var CreateResourceTypeVersion = &cobra.Command{
	Use: "resource-type-version <resource-type-id>", Args: cobra.ExactArgs(1),
	Short: "Publish the next immutable Resource Type interface",
	Long:  fmt.Sprintf("Publish an explicit interface version without changing version 1 or existing Modules. Fields: %s.", generateTopLevelSetFields(cp.ResourceTypeVersionPublishBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := readSetFlagsIntoType[cp.ResourceTypeVersionPublishBody](cmd)
		if err != nil {
			return err
		}
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).PublishResourceTypeVersionWithResponse(cmd.Context(), org, args[0],
			&cp.PublishResourceTypeVersionParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to publish Resource Type version")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("publish Resource Type version", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var GetResourceTypeVersion = &cobra.Command{
	Use: "resource-type-version <resource-type-id> <version>", Args: cobra.ExactArgs(2),
	Short: "Read an exact immutable Resource Type interface",
	RunE: func(cmd *cobra.Command, args []string) error {
		version, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || version < 1 {
			return errors.New("Resource Type version must be a positive integer")
		}
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).GetResourceTypeVersionWithResponse(cmd.Context(), org, args[0], version)
		if err != nil {
			return errors.Wrap(err, "failed to read Resource Type version")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("read Resource Type version", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var ListResourceTypeVersions = &cobra.Command{
	Use: "resource-type-versions <resource-type-id>", Args: cobra.ExactArgs(1),
	Short: "List immutable Resource Type interface history",
	RunE: func(cmd *cobra.Command, args []string) error {
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		items, err := clients.CollectAll(func(page string) (*cp.ListResourceTypeVersionsResponse, error) {
			return MustCpClient(cmd.Context()).ListResourceTypeVersionsWithResponse(cmd.Context(), org, args[0], &cp.ListResourceTypeVersionsParams{Page: ref.RefStringEmptyNil(page)})
		}, func(response *cp.ListResourceTypeVersionsResponse) ([]cp.ResourceTypeVersion, *string) {
			return response.JSON200.Items, response.JSON200.NextPageToken
		})
		if err != nil {
			return err
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), items)
	},
}

var CreateModuleDraft = &cobra.Command{
	Use: "module-draft <module-id>", Args: cobra.ExactArgs(1), Short: "Start editable Module authoring without publishing a Version",
	Long: fmt.Sprintf("Save an editable Draft. Edits do not reserve SemVer or execute a deployment. Fields: %s.", generateTopLevelSetFields(cp.ModuleDraftCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := readSetFlagsIntoType[cp.ModuleDraftCreateBody](cmd)
		if err != nil {
			return err
		}
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).CreateModuleDraftWithResponse(cmd.Context(), org, args[0],
			&cp.CreateModuleDraftParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create Module Draft")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("create Module Draft", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var UpdateModuleDraft = &cobra.Command{
	Use: "module-draft <module-id> <draft-uuid>", Args: cobra.ExactArgs(2), Short: "Save a new authoring revision of an open Draft",
	Long: fmt.Sprintf("Save with the exact expected_resource_version. Earlier revisions remain immutable. Fields: %s.", generateTopLevelSetFields(cp.ModuleDraftUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[1])
		if err != nil {
			return errors.Wrap(err, "invalid Draft UUID")
		}
		body, err := readSetFlagsIntoType[cp.ModuleDraftUpdateBody](cmd)
		if err != nil {
			return err
		}
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).UpdateModuleDraftWithResponse(cmd.Context(), org, args[0], id,
			&cp.UpdateModuleDraftParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, *body)
		if err != nil {
			return errors.Wrap(err, "failed to save Module Draft")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("save Module Draft", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var GetModuleDraft = &cobra.Command{
	Use: "module-draft <module-id> <draft-uuid>", Args: cobra.ExactArgs(2), Short: "Read a Module Draft or an exact authoring revision",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[1])
		if err != nil {
			return errors.Wrap(err, "invalid Draft UUID")
		}
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		revision, _ := cmd.Flags().GetInt64("revision")
		if cmd.Flags().Changed("revision") {
			if revision < 1 {
				return errors.New("Draft revision must be a positive integer")
			}
			response, err := MustCpClient(cmd.Context()).GetModuleDraftRevisionWithResponse(cmd.Context(), org, args[0], id, revision)
			if err != nil {
				return errors.Wrap(err, "failed to read Draft revision")
			}
			if response.StatusCode() != http.StatusOK {
				return moduleCommandError("read Draft revision", response.StatusCode(), response.Body)
			}
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
		}
		response, err := MustCpClient(cmd.Context()).GetModuleDraftWithResponse(cmd.Context(), org, args[0], id)
		if err != nil {
			return errors.Wrap(err, "failed to read Module Draft")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("read Module Draft", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

var ListModuleDrafts = &cobra.Command{
	Use: "module-drafts <module-id>", Args: cobra.ExactArgs(1), Short: "List open and retained Module Drafts",
	RunE: func(cmd *cobra.Command, args []string) error {
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		items, err := clients.CollectAll(func(page string) (*cp.ListModuleDraftsResponse, error) {
			return MustCpClient(cmd.Context()).ListModuleDraftsWithResponse(cmd.Context(), org, args[0], &cp.ListModuleDraftsParams{Page: ref.RefStringEmptyNil(page)})
		}, func(response *cp.ListModuleDraftsResponse) ([]cp.ModuleDraft, *string) {
			return response.JSON200.Items, response.JSON200.NextPageToken
		})
		if err != nil {
			return err
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), items)
	},
}

var UpdateModuleDraftStatus = &cobra.Command{
	Use: "module-draft-status <module-id> <draft-uuid>", Args: cobra.ExactArgs(2),
	Short: "Publish an exact reviewed Draft revision or discard authoring",
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, flag := range []string{createUpdateCmdSetJsonFlag, createUpdateCmdSetYamlFlag, createUpdateCmdSetFlag} {
			if cmd.Flags().Changed(flag) {
				return errors.New("Draft status commands use the reviewed stored revision; save edits separately with octl update module-draft")
			}
		}

		id, err := uuid.Parse(args[1])
		if err != nil {
			return errors.Wrap(err, "invalid Draft UUID")
		}
		action, _ := cmd.Flags().GetString(moduleVersionActionFlag)
		if action != "publish" && action != "discard" {
			return errors.New("Draft action must be publish or discard")
		}
		expected, _ := cmd.Flags().GetInt64(moduleVersionExpectedFlag)
		if expected < 1 {
			return errors.New("expected Draft resource version must be positive")
		}
		reason, _ := cmd.Flags().GetString(moduleVersionReasonFlag)
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		body := cp.ModuleReasonedCommand{ExpectedResourceVersion: expected, Reason: reason}
		key := moduleCommandIdempotencyKey(cmd)
		if action == "publish" {
			response, err := MustCpClient(cmd.Context()).PublishModuleDraftWithResponse(cmd.Context(), org, args[0], id, &cp.PublishModuleDraftParams{IdempotencyKey: key}, body)
			if err != nil {
				return errors.Wrap(err, "failed to publish reviewed Draft")
			}
			if response.StatusCode() != http.StatusCreated {
				return moduleCommandError("publish reviewed Draft", response.StatusCode(), response.Body)
			}
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
		}
		response, err := MustCpClient(cmd.Context()).DiscardModuleDraftWithResponse(cmd.Context(), org, args[0], id, &cp.DiscardModuleDraftParams{IdempotencyKey: key}, body)
		if err != nil {
			return errors.Wrap(err, "failed to discard Draft")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("discard Draft", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

func init() {
	for _, command := range []*cobra.Command{CreateResourceTypeVersion, CreateModuleDraft, UpdateModuleDraft, UpdateModuleDraftStatus} {
		command.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	}
	CreateCmd.AddCommand(CreateResourceTypeVersion, CreateModuleDraft)
	UpdateCmd.AddCommand(UpdateModuleDraft, UpdateModuleDraftStatus)
	GetModuleDraft.Flags().Int64("revision", 0, "Read an exact immutable authoring revision instead of the current Draft")
	GetCmd.AddCommand(GetResourceTypeVersion, ListResourceTypeVersions, GetModuleDraft, ListModuleDrafts)
	UpdateModuleDraftStatus.Flags().String(moduleVersionActionFlag, "", "Draft action: publish or discard")
	UpdateModuleDraftStatus.Flags().String(moduleVersionReasonFlag, "", "Mandatory audited reason")
	UpdateModuleDraftStatus.Flags().Int64(moduleVersionExpectedFlag, 0, "Exact reviewed Draft resource version")
	for _, flag := range []string{moduleVersionActionFlag, moduleVersionReasonFlag, moduleVersionExpectedFlag} {
		_ = UpdateModuleDraftStatus.MarkFlagRequired(flag)
	}
}
