package command

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	deleteEnvNoWaitFlag = "no-wait"
	deleteEnvForceFlag  = "force"
	deleteEnvRulesFlag  = "delete-rules"
	environmentUse      = "environment <project-id> <environment-id>"
)

var ObjectTypeEnvironmentAliasesSingular = []string{
	"env",
}

var ObjectTypeEnvironmentAliasesPlural = []string{
	"envs",
}

var CreateEnvironment = &cobra.Command{
	Use:     "environment <project-id> <environment-id> --set env_type_id=<env_type_id>",
	Aliases: ObjectTypeEnvironmentAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Create an environment in a project",
	Long: fmt.Sprintf(`Create a new environment in a project.

The following fields can be set using --set or --set-json: %s.

Examples:

# The env_type_id is required
$ octl create environment my-project my-new-env --set env_type_id=production

# Set a display name
$ octl create environment my-project my-new-env --set env_type_id=production --set display_name='My New Environment'

`, generateTopLevelSetFields(cp.EnvironmentCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		x, err := readSetFlagsIntoType[cp.EnvironmentCreateBody](cmd)
		if err != nil {
			return err
		}

		projectId := args[0]
		x.Id = args[1]

		// Check if env_type_id is provided via --set
		if x.EnvTypeId == "" {
			return errors.New("env_type_id is required. Use --set env_type_id=<env_type_id>")
		}

		cmd.SilenceUsage = true

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating environment", slog.String("org_id", orgId), slog.String("project_id", projectId), slog.String("id", x.Id))
		if res, err := cpc.CreateEnvironmentWithResponse(cmd.Context(), orgId, projectId, *x); err != nil {
			return errors.Wrap(err, "failed to create environment")
		} else if res.StatusCode() == http.StatusConflict {
			return errorToHint(cmd, errors.Errorf("conflict: %s.", res.JSON409.Message))
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating environment: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Environment %s created in project %s.", x.Id, projectId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

var DeleteEnvironment = &cobra.Command{
	Use:     environmentUse,
	Aliases: ObjectTypeEnvironmentAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Delete an environment from the project. This will trigger a background destroy deployment.",
	Long: `This will trigger a background destroy deployment and remove all applications and resources from the environment.
If the deployment fails, the environment will be in a delete_failed state and the delete environment operation can be retried.

This command prompts for confirmation, but the prompt can be skipped using the --no-prompt flag.

If necessary, the --force flag can be used to delete the environment without running a destroy deployment first. This is not
recommended since this will leave all resources and applications behind and is not recoverable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		dpClient := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		projectId, envId := args[0], args[1]

		if res, err := cpc.GetEnvironmentWithResponse(cmd.Context(), orgId, projectId, envId); err != nil {
			return errors.Wrap(err, "failed to get environment")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("environment '%s' not found in project '%s' in org '%s'", envId, projectId, orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting environment: %s", res.StatusCode(), string(res.Body))
		} else if res.JSON200.Status == cp.EnvironmentStatusDeleting {
			return errors.Errorf("environment '%s' is already deleting", envId)
		}

		var manifest dp.DeploymentManifest
		if res, err := dpClient.ListLastDeploymentsWithResponse(cmd.Context(), orgId, &dp.ListLastDeploymentsParams{
			ProjectId:       &projectId,
			EnvId:           &envId,
			StateChangeOnly: ref.Ref(true),
			PerPage:         ref.Ref(1),
		}); err != nil {
			return errors.Wrap(err, "failed to list last deployments")
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when listing last deployments: %s", res.StatusCode(), string(res.Body))
		} else if len(res.JSON200.Items) > 0 {
			if res, err := dpClient.GetDeploymentWithResponse(cmd.Context(), orgId, res.JSON200.Items[0].Id); err != nil {
				return errors.Wrap(err, "failed to get deployment")
			} else if res.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when getting deployment: %s", res.StatusCode(), string(res.Body))
			} else {
				manifest = res.JSON200.Manifest
			}
		}
		infoMessageF("Environment '%s' in project '%s' contains %d workloads and %d shared resources", envId, projectId, len(manifest.Workloads), len(manifest.Shared))

		// Handle rules deletion
		var deleteRules bool
		if deleteRules, err = cmd.Flags().GetBool(deleteEnvRulesFlag); err != nil {
			return err
		}

		var noPrompt, _ = cmd.Flags().GetBool(deployCmdNoPromptFlag)

		// If --delete-rules is not set, check for existing rules and prompt user if necessary
		if !deleteRules && !noPrompt {
			// List module rules for the environment
			moduleRulesRes, err := cpc.ListModuleRulesInOrgWithResponse(cmd.Context(), orgId, &cp.ListModuleRulesInOrgParams{
				ByProjectId: &projectId,
				ByEnvId:     &envId,
			})
			if err != nil {
				return errors.Wrap(err, "failed to list module rules for environment")
			} else if moduleRulesRes.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when listing module rules for environment: %s", moduleRulesRes.StatusCode(), string(moduleRulesRes.Body))
			}

			moduleRulesCount := len(moduleRulesRes.JSON200.Items)

			// If there are rules, prompt the user
			if moduleRulesCount > 0 {
				infoMessageF("Environment %q in project %q has %d module rule(s).", envId, projectId, moduleRulesCount)

				infoMessageF("Module rules:")
				for _, rule := range moduleRulesRes.JSON200.Items {
					infoMessageF("  - %s (module: %s, type: %s)", rule.Id, rule.ModuleId, rule.ResourceType)
				}

				// Prompt the user to confirm deletion of rules
				confirmed, err := PromptYesNo(cmd.Context(), os.Stdin, "Do you want to delete these rules along with the environment?")
				if err != nil {
					return err
				}

				if confirmed {
					deleteRules = true
				} else {
					infoMessageF("Proceeding without deleting rules.")
				}
			}
		}

		forceDelete, _ := cmd.Flags().GetBool(deleteEnvForceFlag)
		if !noPrompt {
			if forceDelete {
				changedMessageF("This destroy cannot be undone and using --force may result in stranded and unrecoverable state. Are you sure you want to FORCE delete environment '%s' in project '%s'?", envId, projectId)
			} else {
				changedMessageF("This destroy cannot be undone. Are you sure you want to delete environment '%s' in project '%s'?", envId, projectId)
			}
			if err := PromptTextAndEnterToContinue(cmd.Context(), os.Stdin, fmt.Sprintf("%s/%s", projectId, envId)); err != nil {
				return err
			}
		}

		if res, err := cpc.DeleteEnvironmentWithResponse(cmd.Context(), orgId, projectId, envId, &cp.DeleteEnvironmentParams{Force: &forceDelete, DeleteRules: ref.Ref(deleteRules)}); err != nil {
			return errors.Wrap(err, "failed to delete environment")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("environment '%s' not found in project '%s' in org '%s'", envId, projectId, orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("environment '%s' cannot be deleted: %s", envId, res.JSON409.Message)
		} else if res.StatusCode() != http.StatusAccepted {
			return errors.Errorf("unexpected status code %d when deleting environment: %s", res.StatusCode(), string(res.Body))
		} else if res.JSON202.Status != cp.EnvironmentStatusDeleting {
			return errors.Errorf("unexpected status %s when deleting environment: %s", res.JSON202.Status, string(res.Body))
		} else {
			changedMessageF("Environment %s is deleting from project %s.", envId, projectId)
			if v, _ := cmd.Flags().GetBool(deleteEnvNoWaitFlag); v {
				return nil
			}
		}

		infoMessageF("Waiting for environment %s to be gone...", envId)
		completedDeps := make(map[uuid.UUID]bool)
		for {
			select {
			case <-time.After(2 * time.Second):
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			}

			exists, depId, err := isEnvironmentStillDeleting(cmd.Context(), cpc, orgId, projectId, envId)
			if err != nil {
				return err
			} else if !exists {
				successMessageF("Environment %s deleted from project %s.", envId, projectId)
				return nil
			} else if depId != uuid.Nil && !completedDeps[depId] {

				infoMessageF("Waiting for environment destroy deployment %s to complete...", depId)
				succeeded, err := hasDestroyDeploymentSucceeded(cmd.Context(), dpClient, orgId, depId)
				if err != nil {
					return err
				} else if succeeded {
					successMessageF("Destroy deployment %s succeeded.", depId)
					completedDeps[depId] = true
				}
			}
		}
	},
}

func isEnvironmentStillDeleting(ctx context.Context, cpc cp.ClientWithResponsesInterface, orgId string, projectId, envId string) (bool, uuid.UUID, error) {
	if res, err := cpc.GetEnvironmentWithResponse(ctx, orgId, projectId, envId); err != nil {
		return true, uuid.Nil, errors.Wrap(err, "failed to get environment")
	} else if res.StatusCode() == http.StatusNotFound {
		return false, uuid.Nil, nil
	} else if res.StatusCode() != http.StatusOK {
		return true, uuid.Nil, errors.Errorf("unexpected status code %d when getting environment: %s", res.StatusCode(), string(res.Body))
	} else if res.JSON200.Status == cp.EnvironmentStatusDeleting {
		depId, err := uuid.Parse(uuidRegex.FindString(ref.DerefOr(res.JSON200.StatusMessage, "")))
		if err != nil {
			slog.Debug("failed to parse uuid from status message", slog.String("message", ref.DerefOr(res.JSON200.StatusMessage, "")), slog.String("err", err.Error()))
			return true, uuid.Nil, nil
		} else {
			return true, depId, nil
		}
	} else {
		return true, uuid.Nil, errors.Errorf("environment %s not deleted from project %s: status is %s: %s", envId, projectId, res.JSON200.Status, ref.DerefOr(res.JSON200.StatusMessage, "<no message>"))
	}
}

func hasDestroyDeploymentSucceeded(ctx context.Context, dpClient dp.ClientWithResponsesInterface, orgId string, depId uuid.UUID) (bool, error) {
	if r, err := dpClient.WaitForDeploymentCompleteWithResponse(ctx, orgId, depId, &dp.WaitForDeploymentCompleteParams{}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to wait for destroy deployment to complete")
	} else if r.StatusCode() == http.StatusOK {
		if r.JSON200.Status != "succeeded" {
			return false, errors.Errorf("destroy deployment %s failed: %s", depId, r.JSON200.StatusMessage)
		}
		return true, nil
	} else if r.StatusCode() == http.StatusNotFound {
		return false, nil
	} else if r.StatusCode() == http.StatusRequestTimeout {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		return false, nil
	} else {
		return false, errors.Errorf("unexpected status code %d when waiting for deployment to complete: %s", r.StatusCode(), string(r.Body))
	}
}

var GetEnvironment = &cobra.Command{
	Use:     environmentUse,
	Aliases: ObjectTypeEnvironmentAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Get an environment in the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		projectId := args[0]

		if r, err := cpc.GetEnvironmentWithResponse(cmd.Context(), orgId, projectId, args[1]); err != nil {
			return errors.Wrap(err, "failed to get environment")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("environment '%s' not found in project '%s' in org '%s'", args[1], projectId, orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting environment: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListEnvironments = &cobra.Command{
	Use:     "environments <project-id>",
	Aliases: ObjectTypeEnvironmentAliasesPlural,
	Args:    cobra.ExactArgs(1),
	Short:   "List environments in the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		projectId := args[0]

		allEnvironments, err := clients.CollectAll(
			func(pt string) (*cp.ListEnvironmentsResponse, error) {
				return cpc.ListEnvironmentsWithResponse(cmd.Context(), orgId, projectId, &cp.ListEnvironmentsParams{Page: ref.RefStringEmptyNil(pt)})
			},
			func(r *cp.ListEnvironmentsResponse) ([]cp.Environment, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allEnvironments)
	},
}

var UpdateEnvironment = &cobra.Command{
	Use:     environmentUse,
	Aliases: ObjectTypeEnvironmentAliasesSingular,
	Args:    cobra.ExactArgs(2),
	Short:   "Update an environment",
	Long: fmt.Sprintf(`Update an existing environment in the project.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.EnvironmentUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.EnvironmentUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		projectId := args[0]
		id := args[1]

		slog.Debug("Updating environment", slog.String("org_id", orgId), slog.String("project_id", projectId), slog.String("id", id))
		if res, err := cpc.UpdateEnvironmentWithResponse(cmd.Context(), orgId, projectId, id, *x); err != nil {
			return errors.Wrap(err, "failed to update environment")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("environment '%s' not found in project '%s' in org '%s'", id, projectId, orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating environment: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Environment %s updated in project %s.", id, projectId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateEnvironment)
	DeleteEnvironment.Flags().Bool(deleteEnvForceFlag, false, "Force delete the environment even if it is not in a delete-able state.")
	DeleteEnvironment.Flags().Bool(deleteEnvNoWaitFlag, false, "Exit immediately after starting the delete environment operation.")
	DeleteEnvironment.Flags().Bool(deleteEnvRulesFlag, false, "Also delete all module rules associated with the environment")
	DeleteEnvironment.Flags().Bool(deployCmdNoPromptFlag, false, "Do not prompt for confirmation")
	DeleteCmd.AddCommand(DeleteEnvironment)
	GetCmd.AddCommand(GetEnvironment)
	GetCmd.AddCommand(ListEnvironments)
	UpdateCmd.AddCommand(UpdateEnvironment)
}
