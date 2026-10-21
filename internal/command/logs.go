package command

import (
	"io"
	"net/http"
	"os"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	logsCmdKeyFileFlag = "key-file"
)

var LogsCmd = &cobra.Command{
	Use:   "logs <deployment-id>",
	Short: "Get logs for a deployment",
	Long: `Get logs for a deployment.

Runner logs are encrypted. To read them decrypted, pass the age secret key that decrypts them with --key-file, from a
file or, with '-', from standard input. The key is created during the deployment; 'octl deploy --print-logs-key'
prints it. octl never takes the key itself as an argument: arguments end up in shell history and the process list.

The input holds exactly one age secret key. Blank lines and lines starting with '#' are ignored, so a key file written
by 'age-keygen -o' works as is. Without --key-file the logs are returned encrypted.

$ octl logs 01234567-89ab-cdef-0123-456789abcdef --key-file runner-logs.key
$ printenv RUNNER_LOGS_KEY | octl logs 01234567-89ab-cdef-0123-456789abcdef --key-file -
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deploymentId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "deployment ID must be a valid UUID")
		}

		params := &dp.GetDeploymentLogsParams{}
		if keyFile, _ := cmd.Flags().GetString(logsCmdKeyFileFlag); keyFile != "" {
			secretKey, err := readLogsKey(cmd.InOrStdin(), keyFile)
			if err != nil {
				return err
			}
			params.DecryptKey = &secretKey
		}

		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
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

// readLogsKey reads the age secret key that decrypts a deployment's runner
// logs from the file at path, or from stdin for "-". The input is an age
// identity file holding exactly one key. Errors never quote the input.
func readLogsKey(stdin io.Reader, path string) (string, error) {
	source, in := "standard input", stdin
	if path != "-" {
		f, err := os.Open(path) //nolint:gosec // reading the key file the user names is the point
		if err != nil {
			return "", errors.Wrap(err, "failed to open the runner logs key file")
		}
		defer func() { _ = f.Close() }()
		source, in = path, f
	}
	identities, err := age.ParseIdentities(in)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read the runner logs key from %s", source)
	}
	key, ok := identities[0].(*age.X25519Identity)
	if len(identities) != 1 || !ok {
		return "", errors.Errorf("%s must hold exactly one age secret key of the form AGE-SECRET-KEY-1..., found %d keys", source, len(identities))
	}
	return key.String(), nil
}

func init() {
	LogsCmd.Flags().String(logsCmdKeyFileFlag, "", "Read the secret key that decrypts the logs from this file, or from standard input with '-'. 'octl deploy --print-logs-key' prints the key.")
	_ = LogsCmd.MarkFlagFilename(logsCmdKeyFileFlag)
	oldHelp := LogsCmd.HelpFunc()
	LogsCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		_ = command.Flags().MarkHidden(printer.OutputFormatFlag)
		oldHelp(command, strings)
	})
	RootCmd.AddCommand(LogsCmd)
}
