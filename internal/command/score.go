package command

import (
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filippo.io/age"
	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	"github.com/score-spec/score-go/loader"
	"github.com/score-spec/score-go/schema"
	"github.com/score-spec/score-go/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	scoreDeployCmdDefaultImageFlag = "default-image"
	scorePrintManifestFlag         = "print-manifest"
	scoreDeployDefaultResourceType = "score-workload"
	scoreAnnotationResType         = "platform-orchestrator.dev/resType"
	scoreAnnotationResClass        = "platform-orchestrator.dev/resClass"
	scoreAnnotationResId           = "platform-orchestrator.dev/resId"
)

var ScoreCmd = &cobra.Command{
	Use:   "score",
	Short: "Deploy Score-based workloads to an environment",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

var ScoreDeployCmd = &cobra.Command{
	Use:   "deploy <project-id> <environment-id> <score-file>...",
	Short: "Deploy one or more Score files to the environment",
	Long: fmt.Sprintf(`Deploy one or more Score files to the target environment.

This works by converting each Score workload into a new workload entry and adding it to the deployment manifest. The
workload contains a resource of type "%[1]s" which is provisioned to deploy the workload.

This requires that the "%[1]s" resource type is available in your target environment. You can check this with
the "octl get available-resource-types <project-id> <environment-id>" command.

The resource-type, class, and id can be overridden by adding the '%[2]s', '%[3]s', or '%[4]s' annotations to the
Score workload metadata.

The command accepts multiple Score files. Each provided Score workload will be added or updated in the deployment manifest
but never removed. Use other commands to remove workloads from the deployment manifest.
`, scoreDeployDefaultResourceType, scoreAnnotationResType, scoreAnnotationResClass, scoreAnnotationResId),
	Example: `  # a single score file with default image
  octl score deploy my-project my-env score.yaml --default-image my-image:latest

  # multiple score files at once
  octl score deploy my-project my-env score-1.yaml score-2.yaml score-3.yaml

  # deploy in plan mode
  octl score deploy my-project my-env score.yaml --plan-only

  # deploy in dry run mode
  octl score deploy my-project my-env score.yaml --dry-run

  # print the deployment manifest after conversion to standard output and exit
  octl score deploy my-project my-env score.yaml --print-manifest=-

  # print the deployment manifest after conversion to a file output and exit
  octl score deploy my-project my-env score.yaml --print-manifest=manifest.yaml
`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		projectId, envId, scoreSources := args[0], args[1], args[2:]

		out, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), out, []string{printer.JsonPrinterType, printer.YamlPrinterType, printer.TablePrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)

		var scoreWorkloads []types.Workload
		for _, source := range scoreSources {
			infoMessageF("Loading score file '%s'", source)
			raw, err := os.ReadFile(source) //nolint:gosec
			if err != nil {
				return errors.Wrapf(err, "failed to read score file '%s'", source)
			}
			var rawMap map[string]interface{}
			if err := yaml.Unmarshal(raw, &rawMap); err != nil {
				return errors.Wrapf(err, "failed to parse score file as yaml '%s'", source)
			}
			if changes, err := schema.ApplyCommonUpgradeTransforms(rawMap); err != nil {
				return errors.Wrapf(err, "failed to apply common upgrade transforms to score file '%s'", source)
			} else {
				for _, change := range changes {
					infoMessageF("Applied upgrade transform to score file: %s", change)
				}
			}
			if err = schema.Validate(rawMap); err != nil {
				return errors.Wrapf(err, "score file '%s' is invalid", source)
			}
			var workload types.Workload
			if err = loader.MapSpec(&workload, rawMap); err != nil {
				return errors.Wrapf(err, "failed to load score file '%s'", source)
			}
			// apply container image defaulting
			for containerName, container := range workload.Containers {
				if container.Image == "." {
					if v, _ := cmd.Flags().GetString(scoreDeployCmdDefaultImageFlag); v != "" {
						container.Image = v
						infoMessageF("Set container image for container '%s' to %s", containerName, v)
						workload.Containers[containerName] = container
					} else {
						return errors.Errorf("failed to convert '%s' because container '%s' has no image and --%s was not provided", source, containerName, scoreDeployCmdDefaultImageFlag)
					}
				}
			}
			// normalize the spec by embedding file contents
			if err = loader.Normalize(&workload, filepath.Dir(source)); err != nil {
				return errors.Wrapf(err, "failed to normalize score file '%s'", source)
			}
			scoreWorkloads = append(scoreWorkloads, workload)
		}

		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		if err := checkEnvironmentExistsForDeployment(cmd.Context(), orgId, projectId, envId, cmd); err != nil {
			return err
		}

		_, lastDeploymentManifest, err := getLastDeploymentManifestForEnvironment(cmd.Context(), orgId, projectId, envId)
		if err != nil {
			return err
		}

		manifest := lastDeploymentManifest
		manifest.Workloads = maps.Clone(manifest.Workloads)
		if manifest.Workloads == nil {
			manifest.Workloads = make(map[string]dp.DeploymentManifestWorkload, max(len(scoreWorkloads), len(manifest.Workloads)))
		}

		for _, workload := range scoreWorkloads {
			workloadResources := make(map[string]dp.DeploymentManifestResource, 1+len(workload.Resources))

			if err := rewritePlaceholders(workload); err != nil {
				return errors.Wrapf(err, "failed to rewrite placeholders in workload '%s'", workload.Metadata["name"].(string))
			}

			for alias, r := range workload.Resources {
				dmr := dp.DeploymentManifestResource{
					Type:   r.Type,
					Class:  r.Class,
					Params: r.Params,
				}
				// If this resource IN A SCORE FILE has an id then it is denoted as a shared resource. We therefore
				// add the shared resource id prefix onto the id. This allows it to live with the other shared resources
				// and so can be shared between workloads with ownership passing along and the definition can be extracted
				// from the workloads as well into the shared section.
				if r.Id != nil {
					dmr.Id = ref.Ref("shared." + *r.Id)
				}
				workloadResources[alias] = dmr
			}

			// Allowing the resource type, resource class, and id to be overridden based on annotations allows the developer to opt in
			// to slightly different behavior or conversions supported by the module without the platform engineer
			// needing to be involved. The PE can still validate these values from within the TF module.
			workloadResourceType := scoreDeployDefaultResourceType
			var workloadClass, workloadId *string
			if annotations, ok := workload.Metadata["annotations"].(map[string]interface{}); ok {
				if v, ok := annotations[scoreAnnotationResType].(string); ok && v != "" {
					workloadResourceType = v
				}
				if v, ok := annotations[scoreAnnotationResClass].(string); ok && v != "" {
					workloadClass = &v
				}
				if v, ok := annotations[scoreAnnotationResId].(string); ok && v != "" {
					workloadId = &v
				}
			}

			workloadResources[scoreDeployDefaultResourceType] = dp.DeploymentManifestResource{
				Type:  workloadResourceType,
				Class: workloadClass,
				Id:    workloadId,
				Params: map[string]interface{}{
					"metadata":   workload.Metadata,
					"containers": workload.Containers,
					"service":    ref.DerefOr(workload.Service, types.WorkloadService{}),
				},
			}
			manifest.Workloads[workload.Metadata["name"].(string)] = dp.DeploymentManifestWorkload{Resources: workloadResources}
		}

		if v, _ := cmd.Flags().GetString(scorePrintManifestFlag); v != "" {
			rawManifest, _ := yaml.Marshal(manifest)
			successMessageF("Deployment manifest produced by score manifest[s], writing to destination '%s'", v)
			if v == "-" {
				_, err = cmd.OutOrStdout().Write(rawManifest)
				return err
			} else {
				if _, err := filepath.Abs(v); err != nil {
					return errors.Wrapf(err, "deployment manifest file path '%s' was invalid", v)
				} else {
					if err := os.WriteFile(v+".tmp", rawManifest, 0600); err != nil {
						return errors.Wrap(err, "failed to write temporary deployment manifest file")
					} else if err = os.Rename(v+".tmp", v); err != nil {
						return errors.Wrap(err, "failed to rename temporary deployment manifest file to output target")
					}
				}
			}
			return nil
		}

		body := dp.DeploymentCreateBody{
			ProjectId: projectId,
			EnvId:     envId,
			Mode:      dp.DeploymentCreateBodyModeDeploy,
			Manifest:  &manifest,
		}

		if v, _ := cmd.Flags().GetBool(deployCmdPlanOnlyFlag); v {
			body.Mode = dp.DeploymentCreateBodyModePlanOnly
		}
		if v, _ := cmd.Flags().GetBool(deployCmdDryRunFlag); v {
			body.IsDryRun = v
		}

		var logsAgeKey *age.X25519Identity
		if !body.IsDryRun {
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
		}

		if err := commonDeployCmdDiff(manifest, lastDeploymentManifest); err != nil {
			return err
		}

		_, err = commonDeployCmdInner(cmd, orgId, body, lastDeploymentManifest, logsAgeKey)
		if err != nil {
			return err
		}

		return nil
	},
}

func rewritePlaceholders(w types.Workload) error {

	resourceOutputFuncs := make(map[string]framework.OutputLookupFunc)
	for alias := range w.Resources {
		resourceOutputFuncs[alias] = func(keys ...string) (interface{}, error) {
			return "${resources." + alias + ".outputs." + strings.Join(keys, ".") + "}", nil
		}
	}
	sf := framework.BuildSubstitutionFunction(w.Metadata, resourceOutputFuncs)

	for containerName, c := range w.Containers {
		for key, value := range c.Variables {
			out, err := framework.SubstituteString(value, sf)
			if err != nil {
				return errors.Wrapf(err, "container '%s', variable '%s'", containerName, key)
			}
			c.Variables[key] = out
		}
		for path, f := range c.Files {
			if f.Content != nil && !ref.DerefOr(f.NoExpand, false) {
				out, err := framework.SubstituteString(*f.Content, sf)
				if err != nil {
					return errors.Wrapf(err, "container '%s', files '%s'", containerName, path)
				}
				f.Content = &out
				c.Files[path] = f
			}
		}
		for path, v := range c.Volumes {
			out, err := framework.SubstituteString(v.Source, sf)
			if err != nil {
				return errors.Wrapf(err, "container '%s', volumes '%s'", containerName, path)
			}
			v.Source = out
			c.Volumes[path] = v
		}
		w.Containers[containerName] = c
	}

	for alias, r := range w.Resources {
		if r.Params != nil {
			rawParams := maps.Clone[map[string]interface{}](r.Params)
			if p, err := framework.Substitute(rawParams, sf); err != nil {
				return errors.Wrapf(err, "resources '%s', params", alias)
			} else {
				r.Params = p.(map[string]interface{})
				w.Resources[alias] = r
			}
		}
	}

	return nil
}

func init() {
	printer.SetupListOutputFormatFlag(ScoreDeployCmd.PersistentFlags())

	ScoreDeployCmd.Flags().String(scoreDeployCmdDefaultImageFlag, "", "The default container image to use for the Score deployment if a container has the image set to '.'")
	ScoreDeployCmd.Flags().Bool(deployCmdNoPromptFlag, false, "Do not prompt for confirmation")
	ScoreDeployCmd.Flags().Bool(deployCmdPlanOnlyFlag, false, "Set deployment mode to plan only")
	ScoreDeployCmd.Flags().Bool(deployCmdDryRunFlag, false, "Dry run - validate the request but do not execute the deployment")
	ScoreDeployCmd.Flags().Bool(deployCmdSkipLogsFlag, false, "Logs produced by the runner will not be stored")
	ScoreDeployCmd.Flags().String(deployCmdRunnerLogsLevelFlag, "info", "Set the logging level for the runner (debug|info|warn|error)")
	ScoreDeployCmd.Flags().String(scorePrintManifestFlag, "", "Write the deployment manifest produced by score manifest conversion to the file at the given path and exit. Use '-' to write to stdout.")

	ScoreCmd.AddCommand(ScoreDeployCmd)
	RootCmd.AddCommand(ScoreCmd)
}
