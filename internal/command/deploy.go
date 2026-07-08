package command

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/wI2L/jsondiff"
	"gopkg.in/yaml.v3"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	deployCmdMergeFlag           = "merge"
	deployCmdDropWorkload        = "drop-workload"
	deployCmdDropShared          = "drop-shared"
	deployCmdSetFlag             = "set"
	deployCmdDryRunFlag          = "dry-run"
	deployCmdNoPromptFlag        = "no-prompt"
	deployCmdPlanOnlyFlag        = "plan-only"
	deployCmdOutputFlag          = "result"
	deployCmdFormatFlag          = "out"
	deployCmdSkipLogsFlag        = "skip-logs"
	deployCmdRunnerLogsLevelFlag = "runner-logs-level"
	deployCmdShowLogsFlag        = "show-logs"

	// Deprecated flags
	deprecatedDeployCmdOutputFlag = "output"
	deprecatedDeployCmdFormatFlag = "format"

	getDeploymentLogsFirstTimeout = 2 * time.Second
	getDeploymentLogsNumOfRetries = 5
)

var validRunnerLogsLevels = []string{"debug", "info", "warn", "error"}

var DeployCmd = &cobra.Command{
	Use:   "deploy <project-id> <environment-id> <manifest-source>",
	Short: "Deploy to an environment",
	Long: `Deploy to an environment.

This command can be used to deploy a manifest file including workloads and resources to an environment, wait for completion, and return any outputs.

$ octl deploy my-project my-env manifest.yaml

This will, by default, delete any workloads or resources that are not in the manifest. To merge the manifest into the current environment state, use the --merge flag:

$ octl deploy my-project my-env partial-manifest.yaml --merge

When deploying partial manifests, it is possible to drop workloads or resources from the manifest using the --drop-workload and --drop-shared flags. This is particularly useful when cloning a previous deployment or environment:

$ octl deploy my-project my-env deployment://HEAD --drop-workload deprecated-workload
$ octl deploy my-project my-env deployment://HEAD --drop-shared unused-database-resource

You can use the --plan-only flag to plan a deployment, which will plan the resource graph and module execution without actually changing the environment:

$ octl deploy my-project my-env --plan-only

Various manifest sources are supported for the default mode or when --plan-only is used:

- ./manifest.yaml                                   A local file
- '-'                                               Read from stdin
- deployment://01234567-89ab-cdef-0123-456789abcdef A deployment id in the same org
- deployment://HEAD                                 The last stateful deployment for the environment, this acts like a redeployment
- environment://staging                             The last stateful deployment for an environment in the same project

The form of a manifest is a YAML or JSON file with the following structure:

workloads:
  WORKLOAD-NAME:
    outputs:
      KEY: VALUE EXPRESSION
    resources:
      ALIAS:
        type: TYPE
        ...
shared:
  ALIAS:
    type: TYPE
	...

By default, the command will prompt for confirmation of the diff before proceeding. This can be disabled with the --no-prompt flag and may be needed for non-interactive use.

Once a deployment is created, the CLI shows a URL which can be used to download the runner logs when the deployment completes. The level of the log produced by the runner might be defined with the --runner-logs-level flag, while the CLI will not produce any URL and logs will not be downloadable if the flag --skip-logs is set.
`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType, printer.TablePrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)

		projectId, envId := args[0], args[1]
		outputFlag := GetFlagWithFallback(cmd, "result", "output")
		if outputFlag != "" && outputFlag != "-" {
			if _, err := filepath.Abs(outputFlag); err != nil {
				return errors.Wrapf(err, "output file path '%s' was invalid", outputFlag)
			}
		}

		formatFlag := GetFlagWithFallback(cmd, deployCmdFormatFlag, deprecatedDeployCmdFormatFlag)
		if formatFlag != "yaml" && formatFlag != "json" {
			return errors.Errorf("unsupported output format %s", formatFlag)
		}

		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		rawManifest, err := loadRawManifest(cmd.Context(), cmd.InOrStdin(), args[2], MustDpClient(cmd.Context()), orgId, projectId, envId)
		if err != nil {
			return err
		}
		dec := yaml.NewDecoder(bytes.NewReader(rawManifest))
		dec.KnownFields(true)
		var manifest dp.DeploymentManifest
		if err := dec.Decode(&manifest); err != nil {
			return errors.Wrap(err, "failed to decode manifest")
		}
		if manifest.Workloads == nil {
			manifest.Workloads = make(map[string]dp.DeploymentManifestWorkload)
		}
		successMessageF("Loaded manifest.")

		if err := checkEnvironmentExistsForDeployment(cmd.Context(), orgId, projectId, envId, cmd); err != nil {
			return err
		}

		_, lastDeploymentManifest, err := getLastDeploymentManifestForEnvironment(cmd.Context(), orgId, projectId, envId)
		if err != nil {
			return err
		}

		if v, _ := cmd.Flags().GetBool(deployCmdMergeFlag); v {
			manifest = dp.DeploymentManifest{
				Workloads: maps.Collect(internal.ConcatSeq2(maps.All(lastDeploymentManifest.Workloads), maps.All(manifest.Workloads))),
				Shared:    maps.Collect(internal.ConcatSeq2(maps.All(lastDeploymentManifest.Shared), maps.All(manifest.Shared))),
			}
		}
		if v, _ := cmd.Flags().GetStringArray(deployCmdDropWorkload); len(v) > 0 {
			for _, workloadName := range v {
				delete(manifest.Workloads, workloadName)
			}
		}
		if v, _ := cmd.Flags().GetStringArray(deployCmdDropShared); len(v) > 0 {
			for _, sharedName := range v {
				delete(manifest.Shared, sharedName)
			}
		}
		if overrides, _ := cmd.Flags().GetStringArray(deployCmdSetFlag); len(overrides) > 0 {
			if err := applyManifestOverrides(&manifest, overrides); err != nil {
				return err
			}
		}

		body := dp.DeploymentCreateBody{
			ProjectId: projectId,
			EnvId:     envId,
			Mode:      dp.DeploymentCreateBodyModeDeploy,
			Manifest:  &manifest,
		}

		if v, _ := cmd.Flags().GetBool(deployCmdPlanOnlyFlag); v {
			body.PlanOnly = ref.Ref(true)
		}

		if v, _ := cmd.Flags().GetBool(deployCmdDryRunFlag); v {
			body.IsDryRun = v
		}

		var ageKey *age.X25519Identity
		if e, ok := cmd.Context().Value("ageKey").(*age.X25519Identity); ok {
			ageKey = e
		} else if ageKey, err = age.GenerateX25519Identity(); err != nil {
			return errors.Wrap(err, "failed to generate age key for outputs encryption")
		}
		body.EncryptedOutputsRecipient = ref.Ref(ageKey.Recipient().String())
		slog.Debug("generated outputs encryption identity", slog.String("public recipient", ageKey.Recipient().String()))

		var logsAgeKey *age.X25519Identity
		if v, _ := cmd.Flags().GetBool(deployCmdSkipLogsFlag); !v {
			if e, ok := cmd.Context().Value("logsAgeKey").(*age.X25519Identity); ok {
				logsAgeKey = e
			} else if logsAgeKey, err = age.GenerateX25519Identity(); err != nil {
				return errors.Wrap(err, "failed to generate age key for logs encryption")
			}
			body.EncryptedLogsRecipient = ref.Ref(logsAgeKey.Recipient().String())
			slog.Debug("generated logs encryption identity", slog.String("public recipient", logsAgeKey.Recipient().String()))
		}

		if v, _ := cmd.Flags().GetString(deployCmdRunnerLogsLevelFlag); !slices.Contains(validRunnerLogsLevels, v) {
			return errors.Errorf("unsupported value %s for %s flag", v, deployCmdRunnerLogsLevelFlag)
		} else {
			body.RunnerLogLevel = ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel(v))
		}

		if err := commonDeployCmdDiff(manifest, lastDeploymentManifest); err != nil {
			return err
		}

		deployment, err := commonDeployCmdInner(cmd, orgId, body, lastDeploymentManifest, logsAgeKey)
		if err != nil {
			return err
		}

		if body.IsDryRun {
			return nil
		}

		return commonDeploymentOutputHandler(cmd, deployment, ageKey)
	},
}

func init() {
	printer.SetupListOutputFormatFlag(DeployCmd.PersistentFlags())

	DeployCmd.Flags().Bool(deployCmdMergeFlag, false, "Merge the manifest into the current environment state. This will not destroy workloads or resources that are not in the manifest.")
	DeployCmd.Flags().StringArray(deployCmdDropWorkload, []string{}, "Delete the given workload from the manifest. Can be specified multiple times.")
	DeployCmd.Flags().StringArray(deployCmdDropShared, []string{}, "Delete the given shared resource alias from the manifest. Can be specified multiple times.")
	DeployCmd.Flags().StringArray(deployCmdSetFlag, []string{}, "Override a manifest property. Format: <path>=<value>. Can be specified multiple times. Supported paths: workloads.<name>.outputs.<key>, workloads.<name>.resources.<alias>.{type,class,id,params.<key>}, shared.<alias>.{type,class,id,params.<key>}.")

	DeployCmd.Flags().Bool(deployCmdDryRunFlag, false, "Dry run - validate the request but do not execute the deployment")
	DeployCmd.Flags().Bool(deployCmdNoPromptFlag, false, "Do not prompt for confirmation")
	DeployCmd.Flags().Bool(deployCmdPlanOnlyFlag, false, "Set deployment mode to plan only")
	DeployCmd.Flags().String(deployCmdOutputFlag, "", "Write the results to the file at the given path. Use '-' to write to stdout. By default, results will not be shown.")

	DeployCmd.Flags().StringP(deployCmdFormatFlag, "o", "yaml", "Output format (yaml|json)")

	DeployCmd.Flags().Bool(deployCmdSkipLogsFlag, false, "Logs produced by the runner will not be stored")
	DeployCmd.Flags().String(deployCmdRunnerLogsLevelFlag, "info", "Set the logging level for the runner (debug|info|warn|error)")
	DeployCmd.Flags().Bool(deployCmdShowLogsFlag, false, "Show runner logs in the output of this command")

	// Deprecated flags
	DeployCmd.Flags().String(deprecatedDeployCmdOutputFlag, "", "Write the results to the file at the given path. Use '-' to write to stdout. By default, results will not be shown.")
	DeployCmd.Flags().String(deprecatedDeployCmdFormatFlag, "yaml", "Output format (yaml|json)")

	_ = DeployCmd.Flags().MarkDeprecated(deprecatedDeployCmdOutputFlag, "use --"+deployCmdOutputFlag+" instead.")
	_ = DeployCmd.Flags().MarkDeprecated(deprecatedDeployCmdFormatFlag, "use --"+deployCmdFormatFlag+" instead.")

	RootCmd.AddCommand(DeployCmd)
}

func checkEnvironmentExistsForDeployment(ctx context.Context, orgId, projectId, envId string, cmd *cobra.Command) error {
	cpClient := MustCpClient(ctx)
	infoMessageF("Checking project '%s' and environment '%s' exist...", projectId, envId)
	if r, err := cpClient.GetProjectWithResponse(ctx, orgId, projectId); err != nil {
		return errors.Wrap(err, "failed to get project")
	} else if r.StatusCode() == http.StatusNotFound {
		return errors.Errorf("project '%s' not found in org", projectId)
	} else if r.StatusCode() != http.StatusOK {
		return errors.Errorf("unexpected status code %d when getting project: %s", r.StatusCode(), string(r.Body))
	} else {
		successMessageF("Project '%s' (%s) exists.", r.JSON200.DisplayName, projectId)
		if r, err := cpClient.GetEnvironmentWithResponse(ctx, orgId, projectId, envId); err != nil {
			return errors.Wrap(err, "failed to get environment")
		} else if r.StatusCode() == http.StatusNotFound {
			return SuggestHintByCause(ctx, HintCauseEnvNotFound, cmd, errors.Errorf("environment %q not found in project %q", envId, projectId))
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting environment: %s", r.StatusCode(), string(r.Body))
		} else {
			successMessageF("Environment '%s' (%s type=%s) exists.", r.JSON200.DisplayName, envId, r.JSON200.EnvTypeId)
		}
	}
	return nil
}

func getLastDeploymentManifestForEnvironment(ctx context.Context, orgId, projectId, envId string) (*dp.DeploymentSummary, dp.DeploymentManifest, error) {
	dpClient := MustDpClient(ctx)
	var lastDeployment *dp.DeploymentSummary
	var lastDeploymentManifest dp.DeploymentManifest
	infoMessageF("Checking for last deployment...")
	if r, err := dpClient.ListLastDeploymentsWithResponse(ctx, orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(projectId),
		EnvId:           ref.Ref(envId),
		StateChangeOnly: ref.Ref(true),
	}); err != nil {
		return nil, lastDeploymentManifest, errors.Wrap(err, "failed to list last deployments")
	} else if r.StatusCode() != http.StatusOK {
		return nil, lastDeploymentManifest, errors.Errorf("unexpected status code %d when listing deployments: %s", r.StatusCode(), string(r.Body))
	} else if len(r.JSON200.Items) > 0 {
		lastDeployment = &r.JSON200.Items[0]
		successMessageF("Found previous stateful deployment %s.", lastDeployment.Id)
		if lastDeployment.Status == "executing" {
			return nil, lastDeploymentManifest, errors.Errorf("last deployment %s is still executing for %s", lastDeployment.Id, time.Since(lastDeployment.CreatedAt))
		}

		if r, err := dpClient.GetDeploymentWithResponse(ctx, orgId, lastDeployment.Id); err != nil {
			return nil, lastDeploymentManifest, errors.Wrap(err, "failed to get deployment")
		} else if r.StatusCode() != http.StatusOK {
			return nil, lastDeploymentManifest, errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
		} else {
			lastDeploymentManifest = r.JSON200.Manifest
		}
	} else {
		lastDeploymentManifest.Workloads = make(map[string]dp.DeploymentManifestWorkload)
		infoMessageF("No previous stateful deployment exists for this environment.")
	}
	return lastDeployment, lastDeploymentManifest, nil
}

func commonDeployCmdDiff(newManifest dp.DeploymentManifest, lastDeploymentManifest dp.DeploymentManifest) error {
	infoMessageF("Generating diff...")
	d, err := jsondiff.Compare(lastDeploymentManifest, newManifest, jsondiff.Factorize())
	if err != nil {
		return errors.Wrap(err, "failed to compare manifests")
	}
	if len(d) == 0 {
		infoMessageF("No manifest changes detected - this will be a re-deployment.")
	} else {
		infoMessageF("Manifest changes detected:")
		for _, operation := range d {
			switch operation.Type {
			case jsondiff.OperationAdd:
				successMessageF("Add     %s", operation.Path)
			case jsondiff.OperationRemove:
				failureMessageF("Remove  %s", operation.Path)
			case jsondiff.OperationReplace:
				changedMessageF("Replace %s", operation.Path)
			case jsondiff.OperationMove:
				changedMessageF("Move    %s -> %s", operation.From, operation.Path)
			case jsondiff.OperationCopy:
				successMessageF("Copy    %s -> %s", operation.From, operation.Path)
			}
		}
	}
	return nil
}

func commonDeployCmdInner(cmd *cobra.Command, orgId string, body dp.DeploymentCreateBody, lastDeploymentManifest dp.DeploymentManifest, logsAgeKey *age.X25519Identity) (*dp.Deployment, error) {
	if b, _ := cmd.Flags().GetBool(deployCmdNoPromptFlag); !b {
		if err := PromptTextAndEnterToContinue(cmd.Context(), os.Stdin, ""); err != nil {
			return nil, err
		}
	}

	dpClient := MustDpClient(cmd.Context())
	var deployment dp.Deployment
	if body.PlanOnly != nil && *body.PlanOnly {
		infoMessageF("Creating %s deployment (plan only)...", body.Mode)
	} else {
		infoMessageF("Creating %s deployment...", body.Mode)
	}
	if r, err := dpClient.CreateDeploymentWithResponse(cmd.Context(), orgId, &dp.CreateDeploymentParams{
		IdempotencyKey: ref.Ref(uuid.New().String()),
	}, body); err != nil {
		return nil, errors.Wrap(err, "failed to create deployment")
	} else if r.StatusCode() == http.StatusConflict {
		return nil, errors.Errorf("conflict: %s", r.JSON409.Message)
	} else if r.StatusCode() == http.StatusBadRequest {
		return nil, errors.Errorf("request is invalid: %s", r.JSON400.Message)
	} else if body.IsDryRun {
		if r.StatusCode() != http.StatusOK {
			return nil, errors.Errorf("unexpected status code %d when creating deployment: %s", r.StatusCode(), string(r.Body))
		}
		successMessageF("Dry-run deployment is valid.")
		return nil, nil
	} else if r.StatusCode() != http.StatusCreated {
		return nil, errors.Errorf("unexpected status code %d when creating deployment: %s", r.StatusCode(), string(r.Body))
	} else {
		deployment = *r.JSON201
		successMessageF("Deployment %s created.", deployment.Id)
		if logsAgeKey != nil {
			infoMessageF("Once deployment is complete, logs will be available. To access logs use this secret key: %s, e.g. 'octl logs %s --key=%s'", logsAgeKey.String(), deployment.Id, logsAgeKey.String())
		}
	}

	for {
		infoMessageF("Waiting for deployment %s to complete (%s)...", deployment.Id, time.Since(deployment.CreatedAt).Round(time.Second))
		if r, err := dpClient.WaitForDeploymentCompleteWithResponse(cmd.Context(), orgId, deployment.Id, &dp.WaitForDeploymentCompleteParams{}); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && cmd.Context().Err() == nil {
				continue
			}
			return nil, errors.Wrap(err, "failed to wait for deployment to complete")
		} else if r.StatusCode() == http.StatusOK {
			deployment = *r.JSON200
			break
		} else if r.StatusCode() == http.StatusRequestTimeout {
			if err := cmd.Context().Err(); err != nil {
				return nil, err
			}
			continue
		} else {
			return nil, errors.Errorf("unexpected status code %d when waiting for deployment to complete: %s", r.StatusCode(), string(r.Body))
		}
	}

	if v, _ := cmd.Flags().GetBool(deployCmdShowLogsFlag); v && logsAgeKey != nil {
		infoMessageF("\nRunner logs:")
		params := &dp.GetDeploymentLogsParams{
			DecryptKey: ref.Ref(logsAgeKey.String()),
		}

		// Log availability depends on the internet connection and GCP ingestion latency,
		// so it may take some time for deployment logs to become available.
		var outRes *dp.GetDeploymentLogsResponse
		var err error
		var backoff = getDeploymentLogsFirstTimeout
		for range getDeploymentLogsNumOfRetries {
			outRes, err = dpClient.GetDeploymentLogsWithResponse(cmd.Context(), orgId, deployment.Id, params)
			if err != nil {
				break
			}
			if outRes.StatusCode() != http.StatusNotFound {
				break
			}
			time.Sleep(backoff)
			backoff *= 2
		}

		if err != nil {
			failureMessageF("failed to get logs: %s\n", err.Error())
		} else if outRes.StatusCode() != http.StatusOK {
			failureMessageF("failed to get logs: %s\n", string(outRes.Body))
		} else {
			infoMessageF(string(outRes.Body) + "\n")
		}
	}

	suffix := deployment.StatusMessage
	if suffix != "" {
		suffix = ": " + suffix
	}
	if deployment.Status != "succeeded" {
		return nil, errors.Errorf("deployment %s %s%s", deployment.Id, deployment.Status, suffix)
	}
	successMessageF("Deployment %s succeeded after %s%s", deployment.Id, deployment.CompletedAt.Sub(deployment.CreatedAt).Round(time.Second), suffix)

	return &deployment, nil
}

func commonDeploymentOutputHandler(cmd *cobra.Command, deployment *dp.Deployment, ageKey age.Identity) error {
	outputFile := GetFlagWithFallback(cmd, deployCmdOutputFlag, deprecatedDeployCmdOutputFlag)
	if outputFile == "" {
		infoMessageF("Outputs ignored due to unset --%s flag.", deployCmdOutputFlag)
		return nil
	}

	dpClient := MustDpClient(cmd.Context())
	if outRes, err := dpClient.GetDeploymentEncryptedOutputsWithResponse(cmd.Context(), deployment.OrgId, deployment.Id); err != nil {
		return errors.Wrap(err, "failed to get deployment outputs")
	} else if outRes.StatusCode() != http.StatusOK {
		return errors.Errorf("unexpected status code %d when getting deployment outputs: %s", outRes.StatusCode(), string(outRes.Body))
	} else {
		var out map[string]interface{}
		if r, err := age.Decrypt(base64.NewDecoder(base64.StdEncoding, strings.NewReader(outRes.JSON200.Raw)), ageKey); err != nil {
			return errors.Wrap(err, "failed to decrypt deployment outputs")
		} else if err := json.NewDecoder(r).Decode(&out); err != nil {
			return errors.Wrap(err, "failed to json decode deployment outputs")
		}
		var rawOut []byte
		outFormat := GetFlagWithFallback(cmd, deployCmdFormatFlag, deprecatedDeployCmdFormatFlag)
		switch outFormat {
		case "yaml":
			rawOut, _ = yaml.Marshal(out)
		case "json":
			rawOut, _ = json.Marshal(out)
		default:
			return errors.Errorf("unsupported output format %s", outFormat)
		}
		successMessageF("Retrieved outputs, writing to destination '%s'", outputFile)

		if outputFile == "-" {
			_, err = cmd.OutOrStdout().Write(rawOut)
			return err
		} else if err := os.WriteFile(outputFile+".tmp", rawOut, 0600); err != nil {
			return errors.Wrap(err, "failed to write temporary output file")
		} else if err = os.Rename(outputFile+".tmp", outputFile); err != nil {
			return errors.Wrap(err, "failed to rename temporary output file to output target")
		}
		return nil
	}
}

func loadRawManifest(ctx context.Context, stdin io.Reader, manifestSource string, dpClient dp.ClientWithResponsesInterface, orgId, projectId, envId string) ([]byte, error) {
	u, err := url.Parse(manifestSource)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "" {
		if u.Path == "-" {
			if isatty.IsTerminal(os.Stdin.Fd()) {
				return nil, errors.New("cannot read from stdin, it is a terminal")
			}
			return io.ReadAll(stdin)
		}
		//nolint
		return os.ReadFile(manifestSource)
	}

	// Translate the deployment://head path into environment://env
	if u.Scheme == "deployment" && u.Host == "HEAD" {
		if u.Path != "" || u.RawQuery != "" {
			return nil, errors.Errorf("invalid uri '%s' for deployment://HEAD", u)
		}
		u.Scheme = "environment"
		u.Host = envId
	}

	if u.Scheme == "deployment" {
		if u.Path != "" || u.RawQuery != "" {
			return nil, errors.Errorf("invalid uri '%s' for deployment://<id>", u)
		}
		infoMessageF("Looking up deployment '%s'...", u.Host)
		depId, err := uuid.Parse(u.Host)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse deployment id '%s' as uuid", u.Host)
		}
		if r, err := dpClient.GetDeploymentWithResponse(ctx, orgId, depId); err != nil {
			return nil, errors.Wrap(err, "failed to get deployment")
		} else if r.StatusCode() == http.StatusNotFound {
			return nil, errors.Errorf("deployment %s not found", depId)
		} else if r.StatusCode() != http.StatusOK {
			return nil, errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
		} else {
			successMessageF("Found deployment %s.", r.JSON200.Id)
			return json.Marshal(r.JSON200.Manifest)
		}
	}

	if u.Scheme == "environment" {
		if u.Path != "" || u.RawQuery != "" {
			return nil, errors.Errorf("invalid uri '%s' for environment://<id>", u)
		}
		infoMessageF("Looking up last stateful deployment in environment '%s'...", u.Host)
		if r, err := dpClient.ListLastDeploymentsWithResponse(ctx, orgId, &dp.ListLastDeploymentsParams{
			ProjectId:       ref.Ref(projectId),
			EnvId:           ref.Ref(u.Host),
			StateChangeOnly: ref.Ref(true),
		}); err != nil {
			return nil, errors.Wrap(err, "failed to list last deployments")
		} else if r.StatusCode() != http.StatusOK {
			return nil, errors.Errorf("unexpected status code %d when listing last deployments: %s", r.StatusCode(), string(r.Body))
		} else if len(r.JSON200.Items) == 0 {
			return nil, errors.Errorf("no deployments found for environment '%s' - does it exist?", u.Host)
		} else {
			if r, err := dpClient.GetDeploymentWithResponse(ctx, orgId, r.JSON200.Items[0].Id); err != nil {
				return nil, errors.Wrap(err, "failed to get deployment")
			} else if r.StatusCode() != http.StatusOK {
				return nil, errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
			} else {
				successMessageF("Found last deployment: %s.", r.JSON200.Id)
				return json.Marshal(r.JSON200.Manifest)
			}
		}
	}

	return nil, errors.Errorf("unsupported manifest source scheme '%s'", u.Scheme)
}
