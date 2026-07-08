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

const (
	listModulesTypeFlag = "type"
	moduleUse           = "module <module-id>"
)

var (
	ObjectTypeModuleAliasesSingular = []string{
		"mod",
	}
	ObjectTypeModuleAliasesPlural = []string{
		"mods",
	}
)

var CreateModule = &cobra.Command{
	Use:     moduleUse,
	Aliases: ObjectTypeModuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Create a module",
	Long: fmt.Sprintf(`Create a new module in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ModuleCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ModuleCreateBody](cmd)
		if err != nil {
			return err
		}
		x.Id = args[0]

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating module", slog.String("org_id", orgId), slog.String("id", x.Id))
		if res, err := cpc.CreateModuleWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create module")
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("conflict: %s", res.JSON409.Message)
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating module: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Module %s created in organization %s.", x.Id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteModule = &cobra.Command{
	Use:     moduleUse,
	Aliases: ObjectTypeModuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete a module",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		slog.Debug("Deleting module", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteModuleWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to delete module")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("module '%s' cannot be deleted: %s", args[0], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting module: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Module %s deleted from organization %s.", args[0], orgId)
		return nil
	},
}

var GetModule = &cobra.Command{
	Use:     moduleUse,
	Aliases: ObjectTypeModuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a module",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if r, err := cpc.GetModuleWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to get module")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting module: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListModules = &cobra.Command{
	Use:     "modules",
	Aliases: ObjectTypeModuleAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List modules",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		byType, _ := cmd.Flags().GetString(listModulesTypeFlag)

		allModules, err := clients.CollectAll(
			func(pt string) (*cp.ListModulesResponse, error) {
				return cpc.ListModulesWithResponse(cmd.Context(), orgId, &cp.ListModulesParams{
					Page:           ref.RefStringEmptyNil(pt),
					ByResourceType: ref.RefStringEmptyNil(byType),
				})
			},
			func(r *cp.ListModulesResponse) ([]cp.ModuleSummary, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allModules)
	},
}

var UpdateModule = &cobra.Command{
	Use:     moduleUse,
	Aliases: ObjectTypeModuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Update a module",
	Long: fmt.Sprintf(`Update a module in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ModuleUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ModuleUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Updating module", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.UpdateModuleWithResponse(cmd.Context(), orgId, args[0], *x); err != nil {
			return errors.Wrap(err, "failed to update module")
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating module: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Module %s updated to version %s in organization %s.", args[0], res.JSON200.VersionId, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func init() {
	ListModules.Flags().String(listModulesTypeFlag, "", "Filter by the resource type of the module (eg: 's3')")

	CreateCmd.AddCommand(CreateModule)
	DeleteCmd.AddCommand(DeleteModule)
	GetCmd.AddCommand(GetModule)
	GetCmd.AddCommand(ListModules)
	UpdateCmd.AddCommand(UpdateModule)
}
