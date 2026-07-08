package command

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var GetManifest = &cobra.Command{
	Use:   "manifest <manifest-source>",
	Args:  cobra.RangeArgs(1, 2),
	Short: "Get the deployment manifest for an environment or deployment",
	Long: `Get the deployment manifest for an environment or deployment.

This returns the full manifest content and prints it to stdout.
`,
	Example: `  # Get the manifest for a deployment by uuid.
  octl get manifest 01234567-89ab-cdef-0123-456789abcdef

  # Get the manifest for the latest stateful deployment for an environment.
  octl get manifest my-project my-env
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		var depId uuid.UUID
		if len(args) == 1 {
			depId, err = uuid.Parse(args[0])
			if err != nil {
				return errors.Wrapf(err, "invalid deployment id '%s'", args[0])
			}
		} else if len(args) == 2 {

			if depsResp, err := dpc.ListLastDeploymentsWithResponse(cmd.Context(), orgId, &dp.ListLastDeploymentsParams{
				ProjectId:       ref.Ref(args[0]),
				EnvId:           ref.Ref(args[1]),
				StateChangeOnly: ref.Ref(true),
				PerPage:         ref.Ref(1),
			}); err != nil {
				return errors.Wrap(err, "failed to list last deployments")
			} else if depsResp.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when listing deployments: %s", depsResp.StatusCode(), string(depsResp.Body))
			} else if len(depsResp.JSON200.Items) == 0 {
				return errors.Errorf("no deployments found for environment '%s' - does it exist?", args[1])
			} else {
				depId = depsResp.JSON200.Items[0].Id
			}
		} else {
			return errors.Errorf("expected 1 or 2 arguments, got %d", len(args))
		}

		if r, err := dpc.GetDeploymentWithResponse(cmd.Context(), orgId, depId); err != nil {
			return errors.Wrap(err, "failed to get deployment")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("deployment '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), r.JSON200.Manifest)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetManifest)
}
