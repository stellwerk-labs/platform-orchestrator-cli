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
	listRunnerRulesRunnerFlag    = "runner"
	createRunnerRuleNoPromptFlag = "no-prompt"
)

var (
	ObjectTypeRunnerRuleAliasesSingular = []string{
		"rrl",
	}
	ObjectTypeRunnerRuleAliasesPlural = []string{
		"rrls",
	}
)

var CreateRunnerRule = &cobra.Command{
	Use:     "runner-rule",
	Aliases: ObjectTypeRunnerRuleAliasesSingular,
	Args:    cobra.NoArgs,
	Short:   "Create a runner rule",
	Long: fmt.Sprintf(`Create a new runner rule for a rule in the organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.RunnerRuleCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.RunnerRuleCreateBody](cmd)
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
				if noPrompt, _ := cmd.Flags().GetBool(createRunnerRuleNoPromptFlag); !noPrompt {
					changedMessageF("Project %q not found in organization %q. Are you sure you want to create a runner rule on it?", *x.ProjectId, orgId)
					if err := PromptTextAndEnterToContinue(cmd.Context(), os.Stdin, ""); err != nil {
						return err
					}
				}
			} else if res.StatusCode() != http.StatusOK {
				return errors.Errorf("unexpected status code %d when validating project id: %s", res.StatusCode(), string(res.Body))
			}
		}

		slog.Debug("Creating runner rule", slog.String("org_id", orgId), slog.String("runner_id", x.RunnerId))
		if res, err := cpc.CreateRunnerRuleInOrgWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create runner rule")
		} else if res.StatusCode() == http.StatusConflict {
			return errorToHint(cmd, errors.Errorf("conflict: %s.", res.JSON409.Message))
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating runner rule: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Runner rule %s created for runner %s in organization %s.", res.JSON201.Id, x.RunnerId, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteRunnerRule = &cobra.Command{
	Use:     "runner-rule <runner-rule-id>",
	Aliases: ObjectTypeRunnerRuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete a runner rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		ruleId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrapf(err, "supplied runner rule id '%s' is not a valid uuid", args[0])
		}
		slog.Debug("Deleting runner rule", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteRunnerRuleInOrgWithResponse(cmd.Context(), orgId, ruleId); err != nil {
			return errors.Wrap(err, "failed to delete runner rule")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("runner rule '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("runner rule '%s' cannot be deleted: %s", args[0], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting runner rule: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Runner rule %s deleted from organization %s.", args[0], orgId)
		return nil
	},
}

var GetRunnerRule = &cobra.Command{
	Use:     "runner-rule <runner-rule-id>",
	Aliases: ObjectTypeRunnerRuleAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a runner rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		ruleId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrap(err, "supplied runner rule id is not a valid uuid")
		}
		if r, err := cpc.GetRunnerRuleInOrgWithResponse(cmd.Context(), orgId, ruleId); err != nil {
			return errors.Wrap(err, "failed to get runner rule")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("runner rule '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting runner rule: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListRunnerRules = &cobra.Command{
	Use:     "runner-rules",
	Aliases: ObjectTypeRunnerRuleAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List runner rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		byRunnerId, _ := cmd.Flags().GetString(listRunnerRulesRunnerFlag)

		allRunnerRules, err := clients.CollectAll(
			func(pt string) (*cp.ListRunnerRulesInOrgResponse, error) {
				return cpc.ListRunnerRulesInOrgWithResponse(cmd.Context(), orgId, &cp.ListRunnerRulesInOrgParams{
					Page:       ref.RefStringEmptyNil(pt),
					ByRunnerId: ref.RefStringEmptyNil(byRunnerId),
				})
			},
			func(r *cp.ListRunnerRulesInOrgResponse) ([]cp.RunnerRuleSummary, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allRunnerRules)
	},
}

func init() {
	CreateRunnerRule.Flags().Bool(createRunnerRuleNoPromptFlag, false, "Do not prompt for confirmation when project not found")
	ListRunnerRules.Flags().String(listRunnerRulesRunnerFlag, "", "Filter by the runner id of the runner rules (eg: 'runner-1')")

	CreateCmd.AddCommand(CreateRunnerRule)
	DeleteCmd.AddCommand(DeleteRunnerRule)
	GetCmd.AddCommand(GetRunnerRule)
	GetCmd.AddCommand(ListRunnerRules)
}
