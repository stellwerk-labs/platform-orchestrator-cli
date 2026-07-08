package command

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

var GetTf = &cobra.Command{
	Use:   "tf <deployment-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Get the compiled TF code applied for the given deployment",
	Long: `Get the compiled TF code applied for the given deployment.

This OpenTofu/Terraform source is generated during the deployment by expanding the manifest into a resource graph and
compiling the result into TF. The resulting content includes all providers, variable and output declarations, and module
blocks but does not include the source code of any inline modules.

This command should be used by platform engineers to debug deployments and to understand the TF code that is applied.
There is not expectation that this code can be used outside of the Platform Orchestrator.
`,
	Example: `  # Get the TF code for a deployment by uuid.
  octl get tf 01234567-89ab-cdef-0123-456789abcdef 
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		depId, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid deployment id '%s'", args[0])
		}

		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		if r, err := dpc.GetDeploymentTfWithResponse(cmd.Context(), orgId, depId); err != nil {
			return fmt.Errorf("failed to get deployment tf: %w", err)
		} else if r.StatusCode() == http.StatusNotFound {
			return fmt.Errorf("deployment '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return fmt.Errorf("unexpected status code %d when getting deployment tf: %s", r.StatusCode(), string(r.Body))
		} else {
			_, err = cmd.OutOrStdout().Write(r.Body)
			return err
		}
	},
}

func init() {
	oldHelp := GetTf.HelpFunc()
	GetTf.SetHelpFunc(func(command *cobra.Command, strings []string) {
		_ = command.Flags().MarkHidden(printer.OutputFormatFlag)
		oldHelp(command, strings)
	})
	GetCmd.AddCommand(GetTf)
}
