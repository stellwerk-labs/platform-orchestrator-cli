package command

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	logsCmdKey = "key"
)

var LogsCmd = &cobra.Command{
	Use:   "logs <deployment-id>",
	Short: "Get logs for a deployment",
	Long: `Get logs for a deployment.

This command can be used to show deployment logs for a given deployment. Logs are encrypted, so a private key needs to be passed to this command in the 'key' argument. The key is generated during the deployment and can be found in the output of the 'deploy' command.

$ octl logs 01234567-89ab-cdef-0123-456789abcdef --key=AGE-SECRET-KEY-1SGKT64UHNGURNZUQSAJ9J0C65R2F2PXDCZQVVGRUPN98L33G2HFSUC24EJ
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "deployment ID must be a valid UUID")
		}

		secretKey, _ := cmd.Flags().GetString(logsCmdKey)

		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		params := &dp.GetDeploymentLogsParams{}
		if secretKey != "" {
			params.DecryptKey = &secretKey
		}
		dpClient := MustDpClient(cmd.Context())
		if outRes, err := dpClient.GetDeploymentLogsWithResponse(cmd.Context(), orgId, deploymentId, params); err != nil {
			return errors.Wrap(err, "failed to get deployment logs")
		} else if outRes.StatusCode() != http.StatusOK {
			var msg string
			if outRes.JSON400 != nil {
				msg = outRes.JSON400.Message
			} else if outRes.JSON404 != nil {
				msg = outRes.JSON404.Message
			} else {
				msg = string(outRes.Body)
			}
			return errors.Errorf("unexpected status code %d when getting deployment logs: %s", outRes.StatusCode(), msg)
		} else {
			_, err = cmd.OutOrStdout().Write(outRes.Body)
			return err
		}
	},
}

func init() {
	LogsCmd.Flags().String(logsCmdKey, "", "The private secret key needed to decrypt the logs for the given deployment. You can find this key in the output of the `deploy` command.")
	oldHelp := LogsCmd.HelpFunc()
	LogsCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		_ = command.Flags().MarkHidden(printer.OutputFormatFlag)
		oldHelp(command, strings)
	})
	RootCmd.AddCommand(LogsCmd)
}
