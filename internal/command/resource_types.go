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

const resourceTypeUse = "resource-type <resource-type-id>"

var ObjectTypeResourceTypeAliasesSingular = []string{
	"rt",
	"resourcetype",
}

var ObjectTypeResourceTypeAliasesPlural = []string{
	"rts",
	"resourcetypes",
}

var CreateResourceType = &cobra.Command{
	Use:     resourceTypeUse,
	Aliases: ObjectTypeResourceTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Create a resource type",
	Long: fmt.Sprintf(`Create a new resource type in your organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ResourceTypeCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ResourceTypeCreateBody](cmd)
		if err != nil {
			return err
		}
		x.Id = args[0]

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating resource type", slog.String("org_id", orgId), slog.String("id", x.Id))
		if res, err := cpc.CreateResourceTypeWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create resource type")
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("conflict: %s", res.JSON409.Message)
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating resource type: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Resource type %s created in organization %s.", x.Id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteResourceType = &cobra.Command{
	Use:     resourceTypeUse,
	Aliases: ObjectTypeResourceTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete a resource type",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		slog.Debug("Deleting resource type", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteResourceTypeWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to delete resource type")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("resource type '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("resource type '%s' cannot be deleted: %s", args[0], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting resource type: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Resource type %s deleted from organization %s.", args[0], orgId)
		return nil
	},
}

var GetResourceType = &cobra.Command{
	Use:     resourceTypeUse,
	Aliases: ObjectTypeResourceTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a resource type",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if r, err := cpc.GetResourceTypeWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to get resource type")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("resource type '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting resource type: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListResourceTypes = &cobra.Command{
	Use:     "resource-types",
	Aliases: ObjectTypeResourceTypeAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List resource types",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		allResourceTypes, err := clients.CollectAll(
			func(pt string) (*cp.ListResourceTypesResponse, error) {
				return cpc.ListResourceTypesWithResponse(cmd.Context(), orgId, &cp.ListResourceTypesParams{Page: ref.RefStringEmptyNil(pt)})
			},
			func(r *cp.ListResourceTypesResponse) ([]cp.ResourceType, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allResourceTypes)
	},
}

var UpdateResourceType = &cobra.Command{
	Use:     resourceTypeUse,
	Aliases: ObjectTypeResourceTypeAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Update a resource type",
	Long: fmt.Sprintf(`Update a resource type in your organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ResourceTypeUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ResourceTypeUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Updating resource type", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.UpdateResourceTypeWithResponse(cmd.Context(), orgId, args[0], *x); err != nil {
			return errors.Wrap(err, "failed to update resource type")
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("resource type '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating resource type: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Resource type %s updated in organization %s.", args[0], orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateResourceType)
	DeleteCmd.AddCommand(DeleteResourceType)
	GetCmd.AddCommand(GetResourceType)
	GetCmd.AddCommand(ListResourceTypes)
	UpdateCmd.AddCommand(UpdateResourceType)
}
