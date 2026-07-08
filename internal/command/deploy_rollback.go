package command

import (
	"fmt"
	"net/http"
	"slices"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

var RollbackCmd = &cobra.Command{
	Use:   "rollback <project-id> <environment-id> <deployment-id>",
	Args:  cobra.ExactArgs(3),
	Short: "Rollback an environment to the resource graph of a previous deployment.",
	Long: `Rollback an environment to the resource graph of a previous deployment.

$ octl rollback my-project my-env 01234567-89ab-cdef-0123-456789abcdef

This will start a new deployment to roll back the environment to the same manifest and resource graph, including pinned
module versions, as the deployment with the given id. This will ignore any changes to the modules or module rules that
may affect a normal deploy to this environment.

Rollback does not revert changes to the runner or provider configurations.

Like 'deploy', this command supports --plan-only, --dry-run, and flags for controlling deployment output and logs.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		projectId := args[0]
		envId := args[1]

		targetDeploymentId, err := uuid.Parse(args[2])
		if err != nil {
			return fmt.Errorf("invalid deployment id '%s'", args[2])
		}

		var targetDeployment dp.Deployment
		infoMessageF("Looking up rollback target deployment '%s'...", targetDeploymentId)
		if r, err := MustDpClient(cmd.Context()).GetDeploymentWithResponse(cmd.Context(), orgId, targetDeploymentId); err != nil {
			return err
		} else if r.StatusCode() == http.StatusNotFound {
			return fmt.Errorf("deployment '%s' not found in org '%s'", targetDeploymentId, orgId)
		} else if r.StatusCode() != http.StatusOK {
			return fmt.Errorf("failed to get deployment '%s'", targetDeploymentId)
		} else {
			targetDeployment = *r.JSON200
		}
		if targetDeployment.ProjectId != projectId || targetDeployment.EnvId != envId {
			return fmt.Errorf("deployment '%s' not found in project '%s' environment '%s'", targetDeploymentId, projectId, envId)
		}

		_, lastDeploymentManifest, err := getLastDeploymentManifestForEnvironment(cmd.Context(), orgId, projectId, envId)
		if err != nil {
			return err
		}

		body := dp.DeploymentCreateBody{
			ProjectId:              projectId,
			EnvId:                  envId,
			Mode:                   dp.DeploymentCreateBodyModeRollback,
			RollbackToDeploymentId: ref.Ref(targetDeploymentId),
		}

		if v, _ := cmd.Flags().GetBool(deployCmdPlanOnlyFlag); v {
			body.PlanOnly = ref.Ref(true)
		}

		if v, _ := cmd.Flags().GetBool(deployCmdDryRunFlag); v {
			body.IsDryRun = v
		}

		var outputsAgeKey *age.X25519Identity
		if e, ok := cmd.Context().Value("ageKey").(*age.X25519Identity); ok {
			outputsAgeKey = e
		} else if outputsAgeKey, err = age.GenerateX25519Identity(); err != nil {
			return fmt.Errorf("failed to generate age key for outputs encryption: %w", err)
		}
		body.EncryptedOutputsRecipient = ref.Ref(outputsAgeKey.Recipient().String())

		var logsAgeKey *age.X25519Identity
		if v, _ := cmd.Flags().GetBool(deployCmdSkipLogsFlag); !v {
			if e, ok := cmd.Context().Value("logsAgeKey").(*age.X25519Identity); ok {
				logsAgeKey = e
			} else if logsAgeKey, err = age.GenerateX25519Identity(); err != nil {
				return fmt.Errorf("failed to generate age key for logs encryption: %w", err)
			}
			body.EncryptedLogsRecipient = ref.Ref(logsAgeKey.Recipient().String())
		}

		if v, _ := cmd.Flags().GetString(deployCmdRunnerLogsLevelFlag); !slices.Contains(validRunnerLogsLevels, v) {
			return fmt.Errorf("unsupported value %s for %s flag", v, deployCmdRunnerLogsLevelFlag)
		} else {
			body.RunnerLogLevel = ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel(v))
		}

		if err := commonDeployCmdDiff(targetDeployment.Manifest, lastDeploymentManifest); err != nil {
			return err
		}

		deployment, err := commonDeployCmdInner(cmd, orgId, body, lastDeploymentManifest, logsAgeKey)
		if err != nil {
			return err
		}

		if body.IsDryRun {
			return nil
		}

		return commonDeploymentOutputHandler(cmd, deployment, outputsAgeKey)
	},
}

func init() {
	printer.SetupListOutputFormatFlag(RollbackCmd.PersistentFlags())

	RollbackCmd.Flags().Bool(deployCmdDryRunFlag, false, "Dry run - validate the request but do not execute the deployment")
	RollbackCmd.Flags().Bool(deployCmdNoPromptFlag, false, "Do not prompt for confirmation")
	RollbackCmd.Flags().Bool(deployCmdPlanOnlyFlag, false, "Set deployment mode to plan only")
	RollbackCmd.Flags().String(deployCmdOutputFlag, "", "Write the results to the file at the given path. Use '-' to write to stdout. By default, results will not be shown.")

	RollbackCmd.Flags().StringP(deployCmdFormatFlag, "o", "yaml", "Output format (yaml|json)")

	RollbackCmd.Flags().Bool(deployCmdSkipLogsFlag, false, "Logs produced by the runner will not be stored")
	RollbackCmd.Flags().String(deployCmdRunnerLogsLevelFlag, "info", "Set the logging level for the runner (debug|info|warn|error)")
	RollbackCmd.Flags().Bool(deployCmdShowLogsFlag, false, "Show runner logs in the output of this command")

	RootCmd.AddCommand(RollbackCmd)
}
