package command

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const roleUse = "role <role-id>"

var CreateRole = &cobra.Command{
	Use:   "role",
	Args:  cobra.NoArgs,
	Short: "Create a configurable organization role",
	Long: fmt.Sprintf(`Create a configurable organization role.

The following fields can be set using --set, --set-json, or --set-yaml: %s.
`, generateTopLevelSetFields(iam.RoleWriteBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		body, err := readSetFlagsIntoType[iam.RoleWriteBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).CreateRoleWithResponse(cmd.Context(), orgID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create role")
		}
		switch res.StatusCode() {
		case http.StatusCreated:
			successMessageF("Role %s created in organization %s.", res.JSON201.Id, orgID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON201)
		case http.StatusBadRequest:
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		case http.StatusConflict:
			return errors.Errorf("role conflicts with existing state: %s", res.JSON409.Message)
		default:
			return errors.Errorf("unexpected status code %d when creating role: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var GetRole = &cobra.Command{
	Use:   roleUse,
	Args:  cobra.ExactArgs(1),
	Short: "Get an organization role",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		roleID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "role ID must be a UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).GetRoleWithResponse(cmd.Context(), orgID, roleID)
		if err != nil {
			return errors.Wrap(err, "failed to get role")
		}
		switch res.StatusCode() {
		case http.StatusOK:
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
		case http.StatusNotFound:
			return errors.Errorf("role '%s' not found in organization '%s'", roleID, orgID)
		default:
			return errors.Errorf("unexpected status code %d when getting role: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var ListRoles = &cobra.Command{
	Use:   "roles",
	Args:  cobra.NoArgs,
	Short: "List organization roles",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		items, err := clients.CollectAll(
			func(page string) (*iam.ListRolesResponse, error) {
				return MustIamClient(cmd.Context()).ListRolesWithResponse(cmd.Context(), orgID, &iam.ListRolesParams{Page: ref.RefStringEmptyNil(page)})
			},
			func(res *iam.ListRolesResponse) ([]iam.Role, *string) {
				return res.JSON200.Items, res.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), items)
	},
}

var ListPermissions = &cobra.Command{
	Use:   "permissions",
	Args:  cobra.NoArgs,
	Short: "List permissions available for configurable roles",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).ListPermissionsWithResponse(cmd.Context(), orgID)
		if err != nil {
			return errors.Wrap(err, "failed to list permissions")
		}
		if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when listing permissions: %s", res.StatusCode(), string(res.Body))
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), res.JSON200.Items)
	},
}

var UpdateRole = &cobra.Command{
	Use:   roleUse,
	Args:  cobra.ExactArgs(1),
	Short: "Replace a configurable role's display name and permissions",
	Long: fmt.Sprintf(`Replace a configurable role's complete state.

The following fields can be set using --set, --set-json, or --set-yaml: %s.
`, generateTopLevelSetFields(iam.RoleWriteBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		roleID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "role ID must be a UUID")
		}
		body, err := readSetFlagsIntoType[iam.RoleWriteBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).UpdateRoleWithResponse(cmd.Context(), orgID, roleID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to update role")
		}
		switch res.StatusCode() {
		case http.StatusOK:
			changedMessageF("Role %s updated in organization %s.", roleID, orgID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
		case http.StatusBadRequest:
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		case http.StatusNotFound:
			return errors.Errorf("role '%s' not found in organization '%s'", roleID, orgID)
		case http.StatusConflict:
			return errors.Errorf("role cannot be updated: %s", res.JSON409.Message)
		default:
			return errors.Errorf("unexpected status code %d when updating role: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var DeleteRole = &cobra.Command{
	Use:   roleUse,
	Args:  cobra.ExactArgs(1),
	Short: "Delete an unassigned configurable role",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		roleID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "role ID must be a UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).DeleteRoleWithResponse(cmd.Context(), orgID, roleID)
		if err != nil {
			return errors.Wrap(err, "failed to delete role")
		}
		switch res.StatusCode() {
		case http.StatusNoContent:
			changedMessageF("Role %s deleted from organization %s.", roleID, orgID)
			return nil
		case http.StatusNotFound:
			return errors.Errorf("role '%s' not found in organization '%s'", roleID, orgID)
		case http.StatusConflict:
			return errors.Errorf("role cannot be deleted: %s", res.JSON409.Message)
		default:
			return errors.Errorf("unexpected status code %d when deleting role: %s", res.StatusCode(), string(res.Body))
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateRole)
	GetCmd.AddCommand(GetRole, ListRoles, ListPermissions)
	UpdateCmd.AddCommand(UpdateRole)
	DeleteCmd.AddCommand(DeleteRole)
}
