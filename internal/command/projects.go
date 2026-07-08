package command

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	deleteRulesFlag = "delete-rules"
	projectUse      = "project <project-id>"
)

var ObjectTypeProjectAliasesSingular = []string{}

var ObjectTypeProjectAliasesPlural = []string{}

var CreateProject = &cobra.Command{
	Use:     projectUse,
	Aliases: ObjectTypeProjectAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Create a project",
	Long: fmt.Sprintf(`Create a new project in your organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ProjectCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ProjectCreateBody](cmd)
		if err != nil {
			return err
		}
		x.Id = args[0]

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating project", slog.String("org_id", orgId), slog.String("id", x.Id))
		if res, err := cpc.CreateProjectWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create project")
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("conflict: %s", res.JSON409.Message)
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating project: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Project %s created in organization %s.", x.Id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteProject = &cobra.Command{
	Use:     projectUse,
	Aliases: ObjectTypeProjectAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		projectId := args[0]

		var deleteRules bool
		if deleteRules, err = cmd.Flags().GetBool(deleteRulesFlag); err != nil {
			return err
		}
		slog.Debug("Deleting project", slog.String("org_id", orgId), slog.String("id", projectId), slog.Bool("delete_rules", deleteRules))

		// If --delete-rules is not set, check for existing rules and prompt user
		if !deleteRules {
			// List module rules for the project
			moduleRulesRes, err := cpc.ListModuleRulesInOrgWithResponse(cmd.Context(), orgId, &cp.ListModuleRulesInOrgParams{
				ByProjectId: &projectId,
			})
			if err != nil {
				return errors.Wrap(err, "failed to list module rules for project")
			} else if moduleRulesRes.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when listing module rules for project: %s", moduleRulesRes.StatusCode(), string(moduleRulesRes.Body))
			}

			// List runner rules for the project
			runnerRulesRes, err := cpc.ListRunnerRulesInOrgWithResponse(cmd.Context(), orgId, &cp.ListRunnerRulesInOrgParams{
				ByProjectId: &projectId,
			})
			if err != nil {
				return errors.Wrap(err, "failed to list runner rules for project")
			} else if runnerRulesRes.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when listing runner rules for project: %s", runnerRulesRes.StatusCode(), string(runnerRulesRes.Body))
			}

			moduleRulesCount := len(moduleRulesRes.JSON200.Items)
			runnerRulesCount := len(runnerRulesRes.JSON200.Items)

			// If there are rules, prompt the user
			if moduleRulesCount > 0 || runnerRulesCount > 0 {
				infoMessageF("Project %q has %d module rule(s) and %d runner rule(s).", projectId, moduleRulesCount, runnerRulesCount)

				// Prompt the user to confirm deletion of rules
				confirmed, err := PromptYesNo(cmd.Context(), os.Stdin, "Do you want to delete these rules along with the project?")
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

		if res, err := cpc.DeleteProjectWithResponse(cmd.Context(), orgId, projectId, &cp.DeleteProjectParams{DeleteRules: ref.Ref(deleteRules)}); err != nil {
			return errors.Wrap(err, "failed to delete project")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("project '%s' not found in org '%s'", projectId, orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("project '%s' cannot be deleted: %s", projectId, res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting project: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Project %s deleted from organization %s.", projectId, orgId)
		return nil
	},
}

var GetProject = &cobra.Command{
	Use:     projectUse,
	Aliases: ObjectTypeProjectAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if r, err := cpc.GetProjectWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to get project")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("project '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting project: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListProjects = &cobra.Command{
	Use:     "projects",
	Aliases: ObjectTypeProjectAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		allProjects, err := clients.CollectAll(
			func(pt string) (*cp.ListProjectsResponse, error) {
				return cpc.ListProjectsWithResponse(cmd.Context(), orgId, &cp.ListProjectsParams{Page: ref.RefStringEmptyNil(pt)})
			},
			func(r *cp.ListProjectsResponse) ([]cp.Project, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allProjects)
	},
}

var UpdateProject = &cobra.Command{
	Use:     projectUse,
	Aliases: ObjectTypeProjectAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Update a project",
	Long: fmt.Sprintf(`Update an existing project.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.ProjectUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.ProjectUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		id := args[0]

		slog.Debug("Updating project", slog.String("org_id", orgId), slog.String("id", id))
		if res, err := cpc.UpdateProjectWithResponse(cmd.Context(), orgId, id, *x); err != nil {
			return errors.Wrap(err, "failed to update project")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("project '%s' not found in org '%s'", id, orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating project: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Project %s updated in org %s.", id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func init() {
	DeleteProject.Flags().Bool(deleteRulesFlag, false, "Also delete all module rules and runner rules associated with the project")

	CreateCmd.AddCommand(CreateProject)
	DeleteCmd.AddCommand(DeleteProject)
	GetCmd.AddCommand(GetProject)
	GetCmd.AddCommand(ListProjects)
	UpdateCmd.AddCommand(UpdateProject)
}
