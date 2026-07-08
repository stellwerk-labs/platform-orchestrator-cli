package command

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockcp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp/mocks"
	mockdp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp/mocks"
	mockiam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam/mocks"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
)

const testApiUrl = "https://api.example.com/"

// Common CLI flag constants
const (
	orgFlag      = "--org"
	outFlag      = "--out"
	jsonOutput   = "json"
	noPromptFlag = "--no-prompt"
	planOnlyFlag = "--plan-only"
	dryRunFlag   = "--dry-run"
	skipLogsFlag = "--skip-logs"
	noWaitFlag   = "--no-wait"
	formatFlag   = "--format"
	keyFlag      = "--key"
)

// Common test data constants
const (
	testCreateCmd = "create"
	testDeleteCmd = "delete"
	testDeployCmd = "deploy"
	testGetCmd    = "get"
	testUpdateCmd = "update"

	testProjectId   = "my-project"
	testProjectName = "My Project"
	testEnvId       = "my-env"
	testEnvName     = "My Env"
	testEnvTypeId   = "my-et"
	testEnvTypeName = "My Environment Type"
)

// executeAndResetCommand is a test helper that runs a cobra command and resets
// all flag state afterward so tests do not leak state into each other.
func executeAndResetCommand(ctx context.Context, cmd *cobra.Command, args []string) (string, string, error) {
	beforeOut, beforeErr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	defer func() {
		cmd.SetOut(beforeOut)
		cmd.SetErr(beforeErr)
	}()

	nowOut, nowErr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(nowOut)
	cmd.SetErr(nowErr)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	subCmd, err := cmd.ExecuteC()
	if subCmd != nil {
		subCmd.SetOut(nil)
		subCmd.SetErr(nil)
		//nolint
		subCmd.SetContext(nil)
		subCmd.SilenceUsage = false

		resetCommandTree(subCmd)
	}
	return nowOut.String(), nowErr.String(), err
}

// resetCommandTree resets all flag values on the given command and every
// ancestor up to the root, covering both local flags and persistent flags
// that may have been set during command execution.
func resetCommandTree(cmd *cobra.Command) {
	for c := cmd; c != nil; c = c.Parent() {
		resetFlags(c.Flags())
		resetFlags(c.PersistentFlags())
	}
}

func resetFlags(fs *pflag.FlagSet) {
	fs.VisitAll(func(f *pflag.Flag) {
		if slice, ok := f.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

// resetCommandFlags resets all flags on a single command. Use resetCommandTree
// to also reset all ancestor flags.
func resetCommandFlags(cmd *cobra.Command) {
	resetFlags(cmd.Flags())
	resetFlags(cmd.PersistentFlags())
}

type slogTestLogger struct {
	T *testing.T
}

func (s *slogTestLogger) Write(p []byte) (n int, err error) {
	s.T.Log(string(p))
	return len(p), nil
}

func setupTestContext(t *testing.T) (orgId string, cpc *mockcp.MockClientWithResponsesInterface, dpc *mockdp.MockClientWithResponsesInterface, ctx context.Context, fin func()) {
	ctrl := gomock.NewController(t)
	orgId = fmt.Sprintf("org-%s", strings.ToLower(rand.Text()))
	cpc = mockcp.NewMockClientWithResponsesInterface(ctrl)
	dpc = mockdp.NewMockClientWithResponsesInterface(ctrl)
	iamc := mockiam.NewMockClientWithResponsesInterface(ctrl)
	ctx = context.WithValue(context.Background(), ConfigContextKey, config.Config{
		DefaultOrg: orgId,
	})
	ctx = context.WithValue(ctx, CpClientContextKey, cpc)
	ctx = context.WithValue(ctx, DpClientContextKey, dpc)
	ctx = context.WithValue(ctx, IamClientContextKey, iamc)

	oldSlogDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&slogTestLogger{T: t}, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ageKey, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	//nolint
	ctx = context.WithValue(ctx, "ageKey", ageKey)
	//nolint
	ctx = context.WithValue(ctx, "logsAgeKey", ageKey)
	_ = os.Setenv("PO_API_URL", testApiUrl)
	cobra.EnableTraverseRunHooks = true

	return orgId, cpc, dpc, ctx, func() {
		slog.SetDefault(oldSlogDefault)
		ctrl.Finish()
	}
}
