package command

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	listModuleRulesTypeFlag      = "type"
	listModuleRulesModuleFlag    = "module"
	createModuleRuleNoPromptFlag = "no-prompt"
)

var (
	ObjectTypeRuleAliasesSingular = []string{
		"rule",
		"rl",
	}
	ObjectTypeRuleAliasesPlural = []string{
		"rules",
		"rls",
	}
)

var CreateModuleRule = &cobra.Command{
	Use:     "module-rule",
	Aliases: ObjectTypeRuleAliasesSingular,
	Args:    cobra.NoArgs,
	Short:   "Create a module rule",
	Long: fmt.Sprintf(`Create a new module rule for a module in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.RuleCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.RuleCreateBody](cmd)
		if err != nil {
			return err
		}
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		ctx := cmd.Context()

		if x.ProjectId != nil {
			if res, err := cpc.GetProjectWithResponse(ctx, orgId, *x.ProjectId); err != nil {
				return errors.Wrap(err, "failed to validate project id")
			} else if res.StatusCode() == http.StatusNotFound {
				if noPrompt, _ := cmd.Flags().GetBool(createModuleRuleNoPromptFlag); !noPrompt {
					changedMessageF("Project %q not found in organization %q. Are you sure you want to create a module rule on it?", *x.ProjectId, orgId)
					if err := PromptTextAndEnterToContinue(cmd.Context(), os.Stdin, ""); err != nil {
						return err
					}
				}
			} else if res.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when validating project id: %s", res.StatusCode(), string(res.Body))
			}

			if x.EnvId != nil {
				if res, err := cpc.GetEnvironmentWithResponse(ctx, orgId, *x.ProjectId, *x.EnvId); err != nil {
					return errors.Wrap(err, "failed to validate environment id")
				} else if res.StatusCode() == http.StatusNotFound {
					if noPrompt, _ := cmd.Flags().GetBool(createModuleRuleNoPromptFlag); !noPrompt {
						changedMessageF("Environment %q not found in organization %q and project %q. Are you sure you want to create a module rule on it?", *x.EnvId, orgId, *x.ProjectId)
						if err := PromptTextAndEnterToContinue(cmd.Context(), os.Stdin, ""); err != nil {
							return err
						}
					}
				} else if res.StatusCode() != http.StatusOK {
					return errors.Errorf("unexpected status code %d when validating environment id: %s", res.StatusCode(), string(res.Body))
				}
			}
		}

		slog.Debug("Creating module rule", slog.String("org_id", orgId), slog.String("module_id", x.ModuleId))
		if res, err := cpc.CreateModuleRuleInOrgWithResponse(ctx, orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create module rule")
		} else if res.StatusCode() == http.StatusConflict {
			return errorToHint(cmd, errors.Errorf("conflict: %s.", res.JSON409.Message))
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating module rule: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Module rule %s created for module %s in organization %s.", res.JSON201.Id, x.ModuleId, orgId)
			printer := MustPrinter(ctx)
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteModuleRule = &cobra.Command{
	Use:     "module-rule <rule-id>",
	Aliases: ObjectTypeRuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete a module rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		ruleId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrapf(err, "supplied rule id '%s' is not a valid uuid", args[0])
		}
		slog.Debug("Deleting module rule", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteModuleRuleInOrgWithResponse(cmd.Context(), orgId, ruleId); err != nil {
			return errors.Wrap(err, "failed to delete module rule")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module rule '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("module rule '%s' cannot be deleted: %s", args[0], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting module rule: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Module rule %q deleted from organization %q.", ruleId, orgId)
		return nil
	},
}

var GetModuleRule = &cobra.Command{
	Use:     "module-rule <rule-id>",
	Aliases: ObjectTypeRuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a module rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		ruleId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "supplied rule id is not a valid uuid")
		}
		if r, err := cpc.GetModuleRuleInOrgWithResponse(cmd.Context(), orgId, ruleId); err != nil {
			return errors.Wrap(err, "failed to get module rule")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("module rule '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting module rule: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListModuleRules = &cobra.Command{
	Use:     "module-rules",
	Aliases: ObjectTypeRuleAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List module rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		byType, _ := cmd.Flags().GetString(listModuleRulesTypeFlag)
		byModuleId, _ := cmd.Flags().GetString(listModuleRulesModuleFlag)

		allModuleRules, err := clients.CollectAll(
			func(pt string) (*cp.ListModuleRulesInOrgResponse, error) {
				return cpc.ListModuleRulesInOrgWithResponse(cmd.Context(), orgId, &cp.ListModuleRulesInOrgParams{
					Page:           ref.RefStringEmptyNil(pt),
					ByResourceType: ref.RefStringEmptyNil(byType),
					ByModuleId:     ref.RefStringEmptyNil(byModuleId),
				})
			},
			func(r *cp.ListModuleRulesInOrgResponse) ([]cp.RuleSummary, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allModuleRules)
	},
}

func init() {
	CreateModuleRule.Flags().Bool(createModuleRuleNoPromptFlag, false, "Do not prompt for confirmation when project or environment not found")
	ListModuleRules.Flags().String(listModuleRulesTypeFlag, "", "Filter by the resource type of the module rules (eg: 's3')")
	ListModuleRules.Flags().String(listModuleRulesModuleFlag, "", "Filter by the module id of the module rules (eg: 'sample-dev-s3')")

	CreateCmd.AddCommand(CreateModuleRule)
	DeleteCmd.AddCommand(DeleteModuleRule)
	GetCmd.AddCommand(GetModuleRule)
	GetCmd.AddCommand(ListModuleRules)
}
