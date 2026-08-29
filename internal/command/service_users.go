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

const (
	serviceUserUse      = "service-user <service-user-id>"
	expiryInDaysFlag    = "expiry-in-days"
	defaultExpiryInDays = 30
	minimumExpiryInDays = 1
	maximumExpiryInDays = 3660
)

func listAllServiceUsers(cmd *cobra.Command, orgID string) ([]iam.ServiceUserSummary, error) {
	return clients.CollectAll(
		func(page string) (*iam.ListServiceUsersResponse, error) {
			return MustIamClient(cmd.Context()).ListServiceUsersWithResponse(cmd.Context(), orgID, &iam.ListServiceUsersParams{Page: ref.RefStringEmptyNil(page)})
		},
		func(res *iam.ListServiceUsersResponse) ([]iam.ServiceUserSummary, *string) {
			return res.JSON200.Items, res.JSON200.NextPageToken
		},
	)
}

var CreateServiceUser = &cobra.Command{
	Use:   "service-user",
	Args:  cobra.NoArgs,
	Short: "Create a service user and print its one-time bearer token",
	Long: fmt.Sprintf(`Create a service user and print its bearer token.

The token is returned only once. Redirect structured output directly to a secure destination.
The following fields can be set using --set, --set-json, or --set-yaml: %s.
`, generateTopLevelSetFields(iam.ServiceUserCreateBody{})),
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		body, err := readSetFlagsIntoType[iam.ServiceUserCreateBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).CreateServiceUserWithResponse(cmd.Context(), orgID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to create service user")
		}
		switch res.StatusCode() {
		case http.StatusCreated:
			successMessageF("Service user %s created in organization %s. Its token will not be shown again.", res.JSON201.Id, orgID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON201)
		case http.StatusBadRequest:
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		case http.StatusConflict:
			return errors.Errorf("service user conflicts with existing state: %s", res.JSON409.Message)
		default:
			return errors.Errorf("unexpected status code %d when creating service user: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var GetServiceUser = &cobra.Command{
	Use:   serviceUserUse,
	Args:  cobra.ExactArgs(1),
	Short: "Get a service user",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		serviceUserID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "service-user ID must be a UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		serviceUsers, err := listAllServiceUsers(cmd, orgID)
		if err != nil {
			return errors.Wrap(err, "failed to list service users")
		}
		for _, serviceUser := range serviceUsers {
			if serviceUser.Id == serviceUserID {
				return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), serviceUser)
			}
		}
		return errors.Errorf("service user '%s' not found in organization '%s'", serviceUserID, orgID)
	},
}

var ListServiceUsers = &cobra.Command{
	Use:   "service-users",
	Args:  cobra.NoArgs,
	Short: "List service users",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		serviceUsers, err := listAllServiceUsers(cmd, orgID)
		if err != nil {
			return errors.Wrap(err, "failed to list service users")
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), serviceUsers)
	},
}

var UpdateServiceUser = &cobra.Command{
	Use:   serviceUserUse,
	Args:  cobra.ExactArgs(1),
	Short: "Replace all role assignments for a service user",
	Long: fmt.Sprintf(`Replace all role assignments for a service user.

This is an authoritative replacement. The following fields can be set using --set, --set-json, or --set-yaml: %s.
`, generateTopLevelSetFields(iam.ReplaceServiceUserRolesBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		serviceUserID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "service-user ID must be a UUID")
		}
		body, err := readSetFlagsIntoType[iam.ReplaceServiceUserRolesBody](cmd)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).ReplaceServiceUserRolesWithResponse(cmd.Context(), orgID, serviceUserID, *body)
		if err != nil {
			return errors.Wrap(err, "failed to replace service-user roles")
		}
		switch res.StatusCode() {
		case http.StatusOK:
			changedMessageF("Role assignments for service user %s replaced in organization %s.", serviceUserID, orgID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
		case http.StatusBadRequest:
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		case http.StatusNotFound:
			return errors.Errorf("service user or role not found in organization '%s'", orgID)
		case http.StatusConflict:
			return errors.Errorf("service-user roles cannot be replaced: %s", res.JSON409.Message)
		default:
			return errors.Errorf("unexpected status code %d when replacing service-user roles: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var DeleteServiceUser = &cobra.Command{
	Use:   serviceUserUse,
	Args:  cobra.ExactArgs(1),
	Short: "Delete a service user and revoke its credential",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		serviceUserID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "service-user ID must be a UUID")
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).DeleteServiceUserWithResponse(cmd.Context(), orgID, serviceUserID)
		if err != nil {
			return errors.Wrap(err, "failed to delete service user")
		}
		switch res.StatusCode() {
		case http.StatusNoContent:
			changedMessageF("Service user %s deleted from organization %s.", serviceUserID, orgID)
			return nil
		case http.StatusNotFound:
			return errors.Errorf("service user '%s' not found in organization '%s'", serviceUserID, orgID)
		default:
			return errors.Errorf("unexpected status code %d when deleting service user: %s", res.StatusCode(), string(res.Body))
		}
	},
}

var RegenerateServiceUser = &cobra.Command{
	Use:   serviceUserUse,
	Args:  cobra.ExactArgs(1),
	Short: "Regenerate a service-user credential and print its one-time token",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		serviceUserID, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "service-user ID must be a UUID")
		}
		expiryInDays, err := cmd.Flags().GetInt(expiryInDaysFlag)
		if err != nil {
			return err
		}
		orgID, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		res, err := MustIamClient(cmd.Context()).RegenerateServiceUserWithResponse(cmd.Context(), orgID, serviceUserID, iam.ServiceUserRegenerateBody{ExpiryInDays: expiryInDays})
		if err != nil {
			return errors.Wrap(err, "failed to regenerate service-user credential")
		}
		switch res.StatusCode() {
		case http.StatusOK:
			changedMessageF("Credential for service user %s regenerated. Its token will not be shown again.", serviceUserID)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *res.JSON200)
		case http.StatusNotFound:
			return errors.Errorf("service user '%s' not found in organization '%s'", serviceUserID, orgID)
		default:
			return errors.Errorf("unexpected status code %d when regenerating service-user credential: %s", res.StatusCode(), string(res.Body))
		}
	},
}

func init() {
	RegenerateServiceUser.Flags().Int(expiryInDaysFlag, defaultExpiryInDays, "Credential lifetime in days")
	RegenerateServiceUser.PreRunE = func(cmd *cobra.Command, _ []string) error {
		days, err := cmd.Flags().GetInt(expiryInDaysFlag)
		if err != nil {
			return err
		}
		if days < minimumExpiryInDays || days > maximumExpiryInDays {
			return errors.Errorf("--%s must be between %d and %d", expiryInDaysFlag, minimumExpiryInDays, maximumExpiryInDays)
		}
		return nil
	}

	CreateCmd.AddCommand(CreateServiceUser)
	GetCmd.AddCommand(GetServiceUser, ListServiceUsers)
	UpdateCmd.AddCommand(UpdateServiceUser)
	DeleteCmd.AddCommand(DeleteServiceUser)
	RegenerateCmd.AddCommand(RegenerateServiceUser)
}
