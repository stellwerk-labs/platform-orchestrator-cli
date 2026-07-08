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
	listModuleProvidersCmdTypeFlag = "type"
	moduleProviderUse              = "provider <provider-type> <provider-id>"
)

var (
	ObjectTypeProviderAliasesSingular = []string{
		"mp",
	}
	ObjectTypeProviderAliasesPlural = []string{
		"mps",
	}
)

var CreateModuleProvider = &cobra.Command{
	Use:     moduleProviderUse,
	Aliases: ObjectTypeProviderAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Create a module provider",
	Long: fmt.Sprintf(`Create a new module provider in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ModuleProviderCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ModuleProviderCreateBody](cmd)
		if err != nil {
			return err
		}
		x.ProviderType = args[0]
		x.Id = args[1]

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating module provider", slog.String("org_id", orgId), slog.String("id", x.Id))
		if res, err := cpc.CreateModuleProviderWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create module provider")
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("conflict: %s", res.JSON409.Message)
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating module provider: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Module provider %s created in organization %s.", x.Id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteModuleProvider = &cobra.Command{
	Use:     moduleProviderUse,
	Aliases: ObjectTypeProviderAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Delete a module provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		slog.Debug("Deleting module provider", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteModuleProviderWithResponse(cmd.Context(), orgId, args[0], args[1]); err != nil {
			return errors.Wrap(err, "failed to delete module provider")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module provider '%s' '%s' not found in org '%s'", args[0], args[1], orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("module provider '%s' '%s' cannot be deleted: %s", args[0], args[1], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting module provider: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Module provider %s deleted from organization %s.", args[0], orgId)
		return nil
	},
}

var GetModuleProvider = &cobra.Command{
	Use:     moduleProviderUse,
	Aliases: ObjectTypeProviderAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Get a module provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if r, err := cpc.GetModuleProviderWithResponse(cmd.Context(), orgId, args[0], args[1]); err != nil {
			return errors.Wrap(err, "failed to get module provider")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module provider '%s' '%s' not found in org '%s'", args[0], args[1], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting module provider: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListModuleProviders = &cobra.Command{
	Use:     "providers",
	Aliases: ObjectTypeProviderAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List module providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		byType, _ := cmd.Flags().GetString(listModuleProvidersCmdTypeFlag)

		allModuleProviders, err := clients.CollectAll(
			func(pt string) (*cp.ListModuleProvidersResponse, error) {
				return cpc.ListModuleProvidersWithResponse(cmd.Context(), orgId, &cp.ListModuleProvidersParams{
					Page:           ref.RefStringEmptyNil(pt),
					ByProviderType: ref.RefStringEmptyNil(byType),
				})
			},
			func(r *cp.ListModuleProvidersResponse) ([]cp.ModuleProviderSummary, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allModuleProviders)
	},
}

var UpdateModuleProvider = &cobra.Command{
	Use:     moduleProviderUse,
	Aliases: ObjectTypeProviderAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Update a module provider",
	Long: fmt.Sprintf(`Update a module provider in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ModuleProviderUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ModuleProviderUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Updating module provider", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.UpdateModuleProviderWithResponse(cmd.Context(), orgId, args[0], args[1], *x); err != nil {
			return errors.Wrap(err, "failed to update module provider")
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module provider '%s' '%s' not found in org '%s'", args[0], args[1], orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating module provider: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Module provider %s updated in organization %s.", args[0], orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func init() {
	ListModuleProviders.Flags().String(listModuleProvidersCmdTypeFlag, "", "Filter by the type of the module provider (eg: 'aws')")

	CreateCmd.AddCommand(CreateModuleProvider)
	DeleteCmd.AddCommand(DeleteModuleProvider)
	GetCmd.AddCommand(GetModuleProvider)
	GetCmd.AddCommand(ListModuleProviders)
	UpdateCmd.AddCommand(UpdateModuleProvider)
}
