package command

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var ObjectTypeDeploymentAliasesSingular = []string{
	"dep",
}

var ObjectTypeDeploymentAliasesPlural = []string{
	"deps",
}

var GetDeployment = &cobra.Command{
	Use:     "deployment <deployment-id>",
	Aliases: ObjectTypeDeploymentAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a deployment",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		depId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrapf(err, "invalid deployment id '%s'", args[0])
		}

		if r, err := dpc.GetDeploymentWithResponse(cmd.Context(), orgId, depId); err != nil {
			return errors.Wrap(err, "failed to get deployment")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("deployment '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListDeployments = &cobra.Command{
	Use:     "deployments [project-id] [environment-id]",
	Aliases: ObjectTypeDeploymentAliasesPlural,
	Args:    cobra.RangeArgs(0, 2),
	Short:   "List deployments",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		allDeployments, err := clients.CollectAll(
			func(pt string) (*dp.ListDeploymentsResponse, error) {
				var projectId *string
				if len(args) > 0 {
					projectId = ref.RefStringEmptyNil(args[0])
				}
				var envId *string
				if len(args) > 1 {
					envId = ref.RefStringEmptyNil(args[1])
				}
				return dpc.ListDeploymentsWithResponse(cmd.Context(), orgId, &dp.ListDeploymentsParams{
					Page:      ref.RefStringEmptyNil(pt),
					ProjectId: projectId,
					EnvId:     envId,
				})
			},
			func(r *dp.ListDeploymentsResponse) ([]dp.DeploymentSummary, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allDeployments)
	},
}

func init() {
	GetCmd.AddCommand(GetDeployment)
	GetCmd.AddCommand(ListDeployments)
}
