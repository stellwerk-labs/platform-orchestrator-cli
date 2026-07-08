package command

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var ObjectTypeAvailableResourceTypeAliasesSingular = []string{}

var ObjectTypeAvailableResourceTypeAliasesPlural = []string{}

var ListAvailableResourceType = &cobra.Command{
	Use:     "available-resource-types <project-id> <environment-id>",
	Aliases: ObjectTypeAvailableResourceTypeAliasesPlural,
	Args:    cobra.ExactArgs(2),
	Short:   "List available resource types",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		projectId := args[0]
		envId := args[1]

		allResourceTypes, err := clients.CollectAll(
			func(pt string) (*cp.ListAvailableResourceTypesResponse, error) {
				return cpc.ListAvailableResourceTypesWithResponse(cmd.Context(), orgId, projectId, envId, &cp.ListAvailableResourceTypesParams{Page: ref.RefStringEmptyNil(pt)})
			},
			func(r *cp.ListAvailableResourceTypesResponse) ([]cp.AvailableResourceType, *string) {
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

var GetAvailableResourceType = &cobra.Command{
	Use:     "available-resource-type <project-id> <environment-id> <resource-type-id>",
	Aliases: ObjectTypeAvailableResourceTypeAliasesSingular,
	Args:    cobra.ExactArgs(3),
	Short:   "Get an available resource type in the project for the environment",

	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		projectId := args[0]
		envId := args[1]
		resourceTypeId := args[2]

		resourceType, err := cpc.ListAvailableResourceTypesWithResponse(cmd.Context(), orgId, projectId, envId, &cp.ListAvailableResourceTypesParams{TypeId: ref.RefStringEmptyNil(resourceTypeId)})
		if err != nil {
			return err
		}
		if resourceType.JSON404 != nil || len(resourceType.JSON200.Items) == 0 {
			return errors.Errorf("available resource type '%s' not found in project '%s' for environment '%s'", resourceTypeId, projectId, envId)
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), resourceType.JSON200.Items[0])
		}

	},
}

func init() {
	GetCmd.AddCommand(GetAvailableResourceType)
	GetCmd.AddCommand(ListAvailableResourceType)
}
