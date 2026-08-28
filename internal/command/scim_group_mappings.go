package command

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
)

const scimGroupMappingUse = "scim-group-mapping <group-display-name>"

func upsertScimGroupMapping(cmd *cobra.Command, groupDisplayName string) error {
	body, err := readSetFlagsIntoType[iam.ScimGroupMappingWriteBody](cmd)
	if err != nil {
		return err
	}
	orgID, err := ShouldOrg(cmd.Context())
	if err != nil {
		return err
	}
	res, err := MustIamClient(cmd.Context()).UpsertScimGroupMappingWithResponse(cmd.Context(), orgID, groupDisplayName, *body)
	if err != nil {
		return errors.Wrap(err, "failed to create or update SCIM group mapping")
	}
	switch res.StatusCode() {
	case http.StatusOK:
		changedMessageF("SCIM group mapping for %q updated in organization %s.", groupDisplayName, orgID)
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
	case http.StatusBadRequest:
		return errors.Errorf("request is invalid: %s", res.JSON400.Message)
	case http.StatusNotFound:
		return errors.Errorf("role referenced by SCIM group mapping was not found in organization '%s'", orgID)
	default:
		return errors.Errorf("unexpected status code %d when updating SCIM group mapping: %s", res.StatusCode(), string(res.Body))
	}
}

func newUpsertScimGroupMappingCommand(short string) *cobra.Command {
	return &cobra.Command{
		Use:   scimGroupMappingUse,
		Args:  cobra.ExactArgs(1),
		Short: short,
		Long: fmt.Sprintf(`Create or replace the role mapping for a SCIM group display name.

The following fields can be set using --set, --set-json, or --set-yaml: %s.
`, generateTopLevelSetFields(iam.ScimGroupMappingWriteBody{})),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return upsertScimGroupMapping(cmd, args[0])
		},
	}
}

var CreateScimGroupMapping = newUpsertScimGroupMappingCommand("Create a SCIM group-to-role mapping")

var UpdateScimGroupMapping = newUpsertScimGroupMappingCommand("Replace a SCIM group-to-role mapping")

var GetScimGroupMapping = &cobra.Command{
	Use:   scimGroupMappingUse,
	Args:  cobra.ExactArgs(1),
	Short: "Get a SCIM group-to-role mapping",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).ListScimGroupMappingsWithResponse(cmd.Context(), orgID)
		if err != nil {
			return errors.Wrap(err, "failed to list SCIM group mappings")
		}
		if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when listing SCIM group mappings: %s", res.StatusCode(), string(res.Body))
		}
		for _, mapping := range res.JSON200.Items {
			if strings.EqualFold(mapping.GroupDisplayName, args[0]) {
				return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), mapping)
			}
		}
		return errors.Errorf("SCIM group mapping for %q not found in organization '%s'", args[0], orgID)
	},
}

var ListScimGroupMappings = &cobra.Command{
	Use:   "scim-group-mappings",
	Args:  cobra.NoArgs,
	Short: "List SCIM group-to-role mappings",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).ListScimGroupMappingsWithResponse(cmd.Context(), orgID)
		if err != nil {
			return errors.Wrap(err, "failed to list SCIM group mappings")
		}
		if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when listing SCIM group mappings: %s", res.StatusCode(), string(res.Body))
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), res.JSON200.Items)
	},
}

var DeleteScimGroupMapping = &cobra.Command{
	Use:   scimGroupMappingUse,
	Args:  cobra.ExactArgs(1),
	Short: "Delete a SCIM group-to-role mapping",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).DeleteScimGroupMappingWithResponse(cmd.Context(), orgID, args[0])
		if err != nil {
			return errors.Wrap(err, "failed to delete SCIM group mapping")
		}
		switch res.StatusCode() {
		case http.StatusNoContent:
			changedMessageF("SCIM group mapping for %q deleted from organization %s.", args[0], orgID)
			return nil
		case http.StatusNotFound:
			return errors.Errorf("SCIM group mapping for %q not found in organization '%s'", args[0], orgID)
		default:
			return errors.Errorf("unexpected status code %d when deleting SCIM group mapping: %s", res.StatusCode(), string(res.Body))
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateScimGroupMapping)
	GetCmd.AddCommand(GetScimGroupMapping, ListScimGroupMappings)
	UpdateCmd.AddCommand(UpdateScimGroupMapping)
	DeleteCmd.AddCommand(DeleteScimGroupMapping)
}
