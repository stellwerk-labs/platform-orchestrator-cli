package command

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const runnerUse = "runner <runner-id>"

var (
	ObjectTypeRunnerAliasesSingular []string
	ObjectTypeRunnerAliasesPlural   []string
)

var CreateRunner = &cobra.Command{
	Use:     runnerUse,
	Aliases: ObjectTypeRunnerAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Create a runner",
	Long: fmt.Sprintf(`Create a new runner in your organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.RunnerCreateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.RunnerCreateBody](cmd)
		if err != nil {
			return err
		}
		x.Id = args[0]

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Creating runner", slog.String("org_id", orgId), slog.String("id", x.Id))
		if res, err := cpc.CreateRunnerWithResponse(cmd.Context(), orgId, *x); err != nil {
			return errors.Wrap(err, "failed to create runner")
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("conflict: %s", res.JSON409.Message)
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() != http.StatusCreated {
			return errors.Errorf("unexpected status code %d when creating runner: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Runner %s created in organization %s.", x.Id, orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON201)
		}
	},
}

var DeleteRunner = &cobra.Command{
	Use:     runnerUse,
	Aliases: ObjectTypeRunnerAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Delete a runner by id",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		slog.Debug("Deleting runner", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.DeleteRunnerWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to delete runner")
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("runner '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() == http.StatusConflict {
			return errors.Errorf("runner '%s' cannot be deleted: %s", args[0], res.JSON409.Message)
		} else if res.StatusCode() != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d when deleting runner: %s", res.StatusCode(), string(res.Body))
		}
		changedMessageF("Runner %s deleted from organization %s.", args[0], orgId)
		return nil
	},
}

var GetRunner = &cobra.Command{
	Use:     runnerUse,
	Aliases: ObjectTypeRunnerAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Get a runner by id",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		if r, err := cpc.GetRunnerWithResponse(cmd.Context(), orgId, args[0]); err != nil {
			return errors.Wrap(err, "failed to get runner")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("runner '%s' not found in org '%s'", args[0], orgId)
		} else if r.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when getting runner: %s", r.StatusCode(), string(r.Body))
		} else {
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var ListRunners = &cobra.Command{
	Use:     "runners",
	Aliases: ObjectTypeRunnerAliasesPlural,
	Args:    cobra.NoArgs,
	Short:   "List runners",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		allRunners, err := clients.CollectAll(
			func(pt string) (*cp.ListRunnersResponse, error) {
				return cpc.ListRunnersWithResponse(cmd.Context(), orgId, &cp.ListRunnersParams{
					Page: ref.RefStringEmptyNil(pt),
				})
			},
			func(r *cp.ListRunnersResponse) ([]cp.RunnerSummary, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		printer := MustPrinter(cmd.Context())
		return printer.Write(cmd.OutOrStdout(), allRunners)
	},
}

var UpdateRunner = &cobra.Command{
	Use:     runnerUse,
	Aliases: ObjectTypeRunnerAliasesSingular,
	Args:    cobra.ExactArgs(1),
	Short:   "Update a runner by id",
	Long: fmt.Sprintf(`Update a runner in your organization.

The following fields can be set using --set or --set-json: %s.
`, generateTopLevelSetFields(cp.RunnerUpdateBody{})),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		x, err := readSetFlagsIntoType[cp.RunnerUpdateBody](cmd)
		if err != nil {
			return err
		}

		cpc := MustCpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}

		slog.Debug("Updating runner", slog.String("org_id", orgId), slog.String("id", args[0]))
		if res, err := cpc.UpdateRunnerWithResponse(cmd.Context(), orgId, args[0], *x); err != nil {
			return errors.Wrap(err, "failed to update runner")
		} else if res.StatusCode() == http.StatusBadRequest {
			return errors.Errorf("request is invalid: %s", res.JSON400.Message)
		} else if res.StatusCode() == http.StatusNotFound {
			return errors.Errorf("runner '%s' not found in org '%s'", args[0], orgId)
		} else if res.StatusCode() != http.StatusOK {
			return errors.Errorf("unexpected status code %d when updating runner: %s", res.StatusCode(), string(res.Body))
		} else {
			successMessageF("Runner %s updated in organization %s.", args[0], orgId)
			printer := MustPrinter(cmd.Context())
			return printer.Write(cmd.OutOrStdout(), *res.JSON200)
		}
	},
}

func init() {
	CreateCmd.AddCommand(CreateRunner)
	DeleteCmd.AddCommand(DeleteRunner)
	GetCmd.AddCommand(GetRunner)
	GetCmd.AddCommand(ListRunners)
	UpdateCmd.AddCommand(UpdateRunner)
}
