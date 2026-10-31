package command

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
)

const deployCmdDraftTestFlag = "draft-test"

var CreateModuleDraftTest = &cobra.Command{
	Use:   "module-draft-test <project-id> <env-id> <module-id> <draft-id>",
	Args:  cobra.ExactArgs(4),
	Short: "Capture an immutable test snapshot of a reviewed Draft revision",
	Long:  "Capture the exact reviewed Draft revision for one Environment. This freezes test material without publishing a Version or reserving its intended SemVer. Execute the returned snapshot with octl deploy --draft-test MODULE_UUID=SNAPSHOT_UUID.",
	RunE: func(cmd *cobra.Command, args []string) error {
		draft, err := uuid.Parse(args[3])
		if err != nil || draft == uuid.Nil {
			return errors.New("Draft must be a nonzero UUID")
		}
		expected, _ := cmd.Flags().GetInt64(moduleVersionExpectedFlag)
		if expected < 1 {
			return errors.New("expected Draft resource version must be positive")
		}
		reason, _ := cmd.Flags().GetString(moduleVersionReasonFlag)
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).CaptureModuleDraftTestSnapshotWithResponse(cmd.Context(), org, args[0], args[1], args[2], draft, &cp.CaptureModuleDraftTestSnapshotParams{IdempotencyKey: moduleCommandIdempotencyKey(cmd)}, cp.ModuleDraftTestCaptureBody{ExpectedResourceVersion: expected, Reason: reason})
		if err != nil {
			return errors.Wrap(err, "failed to capture reviewed Draft test")
		}
		if response.StatusCode() != http.StatusCreated {
			return moduleCommandError("capture reviewed Draft test", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON201)
	},
}

var GetModuleDraftTest = &cobra.Command{
	Use:   "module-draft-test <project-id> <env-id> <module-id> <snapshot-id>",
	Args:  cobra.ExactArgs(4),
	Short: "Read an exact immutable Draft test snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		snapshot, err := uuid.Parse(args[3])
		if err != nil || snapshot == uuid.Nil {
			return errors.New("Snapshot must be a nonzero UUID")
		}
		org, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		response, err := MustCpClient(cmd.Context()).GetModuleDraftTestSnapshotWithResponse(cmd.Context(), org, args[0], args[1], args[2], snapshot)
		if err != nil {
			return errors.Wrap(err, "failed to read Draft test snapshot")
		}
		if response.StatusCode() != http.StatusOK {
			return moduleCommandError("read Draft test snapshot", response.StatusCode(), response.Body)
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *response.JSON200)
	},
}

func parseDraftTestSelections(values []string) (map[string]uuid.UUID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	const maxSelections = 1000
	if len(values) > maxSelections {
		return nil, errors.New("select at most 1000 Draft test snapshots")
	}
	selected := make(map[string]uuid.UUID, len(values))
	for _, value := range values {
		moduleText, snapshotText, found := strings.Cut(value, "=")
		module, err := uuid.Parse(moduleText)
		if !found || err != nil || module == uuid.Nil || module.String() != moduleText {
			return nil, fmt.Errorf("draft test selection %q must use MODULE_UUID=SNAPSHOT_UUID with canonical UUIDs", value)
		}
		snapshot, err := uuid.Parse(snapshotText)
		if err != nil || snapshot == uuid.Nil || snapshot.String() != snapshotText {
			return nil, fmt.Errorf("draft test selection %q requires a canonical nonzero snapshot UUID", value)
		}
		if _, duplicate := selected[moduleText]; duplicate {
			return nil, fmt.Errorf("module %s has more than one Draft test selection", moduleText)
		}
		selected[moduleText] = snapshot
	}
	return selected, nil
}

func createDeploymentWithDraftSelection(ctx context.Context, client dp.ClientWithResponsesInterface, org string, params *dp.CreateDeploymentParams, body dp.DeploymentCreateBody, selected map[string]uuid.UUID) (*dp.CreateDeploymentResponse, error) {
	if len(selected) == 0 {
		return client.CreateDeploymentWithResponse(ctx, org, params, body)
	}
	testParams := dp.CreateModuleTestDeploymentParams(*params)
	response, err := client.CreateModuleTestDeploymentWithResponse(ctx, org, &testParams, dp.ModuleTestDeploymentCreateBody{Deployment: body, DraftTestSnapshots: selected})
	if err != nil {
		return nil, err
	}
	// Both operations deliberately share the normal deployment response contract.
	// The separate route prevents older servers from ignoring a test selection.
	return (*dp.CreateDeploymentResponse)(response), nil
}

func init() {
	CreateCmd.AddCommand(CreateModuleDraftTest)
	GetCmd.AddCommand(GetModuleDraftTest)
	CreateModuleDraftTest.Flags().String(moduleVersionIdempotencyFlag, "", "Stable idempotency key")
	CreateModuleDraftTest.Flags().String(moduleVersionReasonFlag, "", "Mandatory audited reason")
	CreateModuleDraftTest.Flags().Int64(moduleVersionExpectedFlag, 0, "Exact reviewed Draft resource version")
	_ = CreateModuleDraftTest.MarkFlagRequired(moduleVersionReasonFlag)
	_ = CreateModuleDraftTest.MarkFlagRequired(moduleVersionExpectedFlag)
	DeployCmd.Flags().StringArray(deployCmdDraftTestFlag, nil, "Execute an exact captured test for this Environment: MODULE_UUID=SNAPSHOT_UUID (repeat per Module). Does not select the moving Draft head or publish a Version.")
}
