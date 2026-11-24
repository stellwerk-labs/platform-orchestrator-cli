package command

import (
	"io"
	"net/http"
	"os"
	"regexp"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const (
	logsCmdKeyFileFlag   = "key-file"
	logsCmdEncryptedFlag = "encrypted"
)

var LogsCmd = &cobra.Command{
	Use:   "logs <deployment-id>",
	Short: "Get logs for a deployment",
	Long: `Get logs for a deployment.

Runner logs are decrypted automatically with your authenticated account's deployment log permission.
Use --encrypted to download the encrypted bytes instead.

For deployments created with a caller-owned key, pass --key-file from a file or, with '-', from standard input.
'octl deploy --print-logs-key' opts into that mode and prints its secret key. Arguments never accept secret keys.
The input holds exactly one age secret key. Blank lines and lines starting with '#' are ignored.

Decrypted logs are formatted and syntax highlighted in a terminal. Pipes and --raw keep the original bytes. Use --pretty to format redirected output. NO_COLOR and TERM=dumb disable colors, retaining readable formatting.

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
		if err := selectDeploymentLogs(cmd, params); err != nil {
			return err
		}
		if keyFile, _ := cmd.Flags().GetString(logsCmdKeyFileFlag); keyFile != "" {
			secretKey, err := readLogsKey(cmd.InOrStdin(), keyFile)
			if err != nil {
				return err
			}
			params.DecryptKey = &secretKey
		} else if encrypted, _ := cmd.Flags().GetBool(logsCmdEncryptedFlag); !encrypted {
			decrypt := true
			params.Decrypt = &decrypt
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
			body := outRes.Body
			if params.DecryptKey != nil || params.Decrypt != nil {
				body = runnerLogsBody(cmd, body)
			}
			_, err = cmd.OutOrStdout().Write(body)
			return err
		}
	},
}

func selectDeploymentLogs(cmd *cobra.Command, params *dp.GetDeploymentLogsParams) error {
	phase, _ := cmd.Flags().GetString("phase")
	execution, _ := cmd.Flags().GetString("execution-id")
	capture, _ := cmd.Flags().GetInt64("capture-revision")
	subject, _ := cmd.Flags().GetString("subject")
	if phase != "" && phase != "plan" && phase != "apply" || cmd.Flags().Changed("phase") && phase == "" {
		return errors.New("phase must be plan or apply")
	}
	if cmd.Flags().Changed("execution-id") && (!regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`).MatchString(execution) || execution == "." || execution == "..") {
		return errors.New("execution-id must be a bounded captured execution identity")
	}
	if cmd.Flags().Changed("capture-revision") && capture < 1 {
		return errors.New("capture-revision must be positive")
	}
	if cmd.Flags().Changed("subject") && subject != "" && !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(subject) {
		return errors.New("subject must be the exact Plan subject")
	}
	if phase != "" {
		value := dp.GetDeploymentLogsParamsPhase(phase)
		params.Phase = &value
	}
	if execution != "" {
		params.ExecutionId = &execution
	}
	if cmd.Flags().Changed("capture-revision") {
		params.CaptureRevision = &capture
	}
	if cmd.Flags().Changed("subject") {
		params.Subject = &subject
	}
	return nil
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
	setupRunnerLogsFlags(LogsCmd)
	LogsCmd.Flags().String("phase", "", "Select Native plan or apply logs (defaults to the deployment's phase).")
	LogsCmd.Flags().String("execution-id", "", "Select an exact captured Native execution.")
	LogsCmd.Flags().Int64("capture-revision", 0, "Select an exact positive capture revision (defaults to greatest accepted).")
	LogsCmd.Flags().String("subject", "", "Bind Plan logs to the exact displayed subject.")
	LogsCmd.Flags().String(logsCmdKeyFileFlag, "", "Read the secret key that decrypts the logs from this file, or from standard input with '-'. 'octl deploy --print-logs-key' prints the key.")
	LogsCmd.Flags().Bool(logsCmdEncryptedFlag, false, "Download encrypted logs without decryption.")
	LogsCmd.MarkFlagsMutuallyExclusive(logsCmdKeyFileFlag, logsCmdEncryptedFlag)
	_ = LogsCmd.MarkFlagFilename(logsCmdKeyFileFlag)
	oldHelp := LogsCmd.HelpFunc()
	LogsCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		_ = command.Flags().MarkHidden(printer.OutputFormatFlag)
		oldHelp(command, strings)
	})
	RootCmd.AddCommand(LogsCmd)
}
