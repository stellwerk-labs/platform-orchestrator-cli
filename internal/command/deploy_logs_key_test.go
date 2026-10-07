package command

import (
	"net/http"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/require"
)

const ageSecretKeyPrefix = "AGE-SECRET-KEY-" //nolint:gosec // the prefix every age secret key starts with, not a credential

// logsKeyFixture serves a deployment that is created at once and succeeds.
func logsKeyFixture(t *testing.T) (*deployQueueServer, string, func(args ...string) (string, error)) {
	t.Helper()
	fixture, ctx := newDeployQueueServer(t)
	fixture.createStatus, fixture.createBody = http.StatusCreated, fixture.deployment
	key := ctx.Value("logsAgeKey").(*age.X25519Identity).String()
	run := func(args ...string) (string, error) {
		stdout, stderr, err := executeAndResetCommand(ctx, RootCmd, args)
		return stdout + stderr, err
	}
	return fixture, key, run
}

func TestRunnerLogsKeyIsNeverPrintedByDefault(t *testing.T) {
	for name, args := range map[string]func(t *testing.T, f *deployQueueServer) []string{
		testDeployCmd: func(t *testing.T, _ *deployQueueServer) []string {
			return deployQueueArgs(t, noPromptFlag)
		},
		queueTestRollback: func(_ *testing.T, f *deployQueueServer) []string {
			f.withBase(deployTestStatusSucceeded)
			return []string{orgFlag, queueTestOrg, queueTestRollback, testProjectId, testEnvId, f.deployment.Id.String(), noPromptFlag}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, key, run := logsKeyFixture(t)
			output, err := run(args(t, fixture)...)
			require.NoError(t, err)
			require.NotEmpty(t, fixture.createdBodies()[0].EncryptedLogsRecipient, "runner logs are still encrypted for the key")
			require.NotContains(t, output, key)
			require.NotContains(t, output, ageSecretKeyPrefix)
			require.Contains(t, output, "Runner logs are encrypted with a key that is not printed; pass --print-logs-key to print it for 'octl logs'.")
		})
	}
}

func TestRunnerLogsKeyIsPrintedOnlyOnRequestWithAWarning(t *testing.T) {
	fixture, key, run := logsKeyFixture(t)
	output, err := run(deployQueueArgs(t, noPromptFlag, "--"+deployCmdPrintLogsKeyFlag)...)
	require.NoError(t, err)
	id := fixture.deployment.Id.String()
	warning := "Warning: the key below decrypts the runner logs of deployment " + id + ". Anyone who can read this output, including CI logs and terminal scrollback, can read those logs."
	require.Contains(t, output, warning+"\nRunner logs key: "+key+"\nOnce the deployment is complete, read its logs with 'octl logs "+id+" --key <runner logs key>'.\n")
	require.Equal(t, 1, strings.Count(output, ageSecretKeyPrefix), "the key is printed once")
}

func TestPrintLogsKeyNeedsRunnerLogs(t *testing.T) {
	fixture, _, run := logsKeyFixture(t)
	_, err := run(deployQueueArgs(t, noPromptFlag, "--"+deployCmdPrintLogsKeyFlag, "--"+deployCmdSkipLogsFlag)...)
	require.ErrorContains(t, err, "none of the others can be")
	require.Empty(t, fixture.createdBodies())
}
