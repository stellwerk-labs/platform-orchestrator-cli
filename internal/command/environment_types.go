package command

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const environmentTypeUse = "environment-type <environment-type-id>"

var ObjectTypeEnvironmentTypeAliasesSingular = []string{
	"et",
	"env-type",
	"envtype",
	"environmenttype",
}

var ObjectTypeEnvironmentTypeAliasesPlural = []string{
	"ets",
	"env-types",
	"envtypes",
	"environmenttypes",
}

var CreateEnvironmentType = &cobra.Command{
	Use:     environmentTypeUse,
	Aliases: ObjectTypeEnvironmentTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Create an environment type",
	Long: fmt.Sprintf(`Create a new environment type in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.EnvironmentTypeCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.EnvironmentTypeCreateBody](cmd)
		if err != nil {
			return err
		}
		x.Id = args[0]

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating environment type", slog.String("org_id", orgId), slog.String("id", x.Id))
		if res, err := cpc.CreateEnvironmentTypeWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create environment type")
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("conflict: %s", res.JSON409.Message)
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating environment type: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Environment type %s created in organization %s.", x.Id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteEnvironmentType = &cobra.Command{
	Use:     environmentTypeUse,
	Aliases: ObjectTypeEnvironmentTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete an environment type",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		slog.Debug("Deleting environment type", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteEnvironmentTypeWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to delete environment type")
		} else if res.StatusCode() == http.StatusNotFound {
			return SuggestHintByCause(cmd.Context(), HintCauseEnvTypeNotFound, cmd, envTypeNotFoundError(args[0], orgId))
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("environment type '%s' cannot be deleted: %s", args[0], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting environment type: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Environment type %s deleted from organization %s.", args[0], orgId)
		return nil
	},
}

var GetEnvironmentType = &cobra.Command{
	Use:     environmentTypeUse,
	Aliases: ObjectTypeEnvironmentTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get an environment type",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if r, err := cpc.GetEnvironmentTypeWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to get environment type")
		} else if r.StatusCode() == http.StatusNotFound {
			return SuggestHintByCause(cmd.Context(), HintCauseEnvTypeNotFound, cmd, envTypeNotFoundError(args[0], orgId))
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting environment type: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListEnvironmentTypes = &cobra.Command{
	Use:     "environment-types",
	Aliases: ObjectTypeEnvironmentTypeAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List environment types",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		allEnvironmentTypes, err := clients.CollectAll(
			func(pt string) (*cp.ListEnvironmentTypesResponse, error) {
				return cpc.ListEnvironmentTypesWithResponse(cmd.Context(), orgId, &cp.ListEnvironmentTypesParams{Page: ref.RefStringEmptyNil(pt)})
			},
			func(r *cp.ListEnvironmentTypesResponse) ([]cp.EnvironmentType, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allEnvironmentTypes)
	},
}

var UpdateEnvironmentType = &cobra.Command{
	Use:     environmentTypeUse,
	Aliases: ObjectTypeEnvironmentTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Update an environment type",
	Long: fmt.Sprintf(`Update an existing environment type in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.EnvironmentTypeUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.EnvironmentTypeUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Updating environment type", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.UpdateEnvironmentTypeWithResponse(cmd.Context(), orgId, args[0], *x); err != nil {
			return errors.Wrap(err, "failed to update environment type")
		} else if res.StatusCode() == http.StatusNotFound {
			return SuggestHintByCause(cmd.Context(), HintCauseEnvTypeNotFound, cmd, envTypeNotFoundError(args[0], orgId))
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating environment type: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Environment type %s updated in organization %s.", args[0], orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func envTypeNotFoundError(envTypeId, orgId string) error {
	return errors.Errorf("environment type %q not found in org %q.", envTypeId, orgId)
}

func init() {
	CreateCmd.AddCommand(CreateEnvironmentType)
	DeleteCmd.AddCommand(DeleteEnvironmentType)
	GetCmd.AddCommand(GetEnvironmentType)
	GetCmd.AddCommand(ListEnvironmentTypes)
	UpdateCmd.AddCommand(UpdateEnvironmentType)
}
