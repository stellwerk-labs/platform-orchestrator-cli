package command

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"filippo.io/age"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
)

const (
	logsTestCmd        = "logs"
	logsTestOrg        = "logs-org"
	logsTestDeployment = "00374e43-96b5-416f-977b-514b9e267f89"
	logsTestText       = `Deployment succeeded after 1s: it was successful!`
)

// logsServer is a Data Plane that answers the runner logs route of one
// deployment and records the DecryptKey header of every request.
type logsServer struct {
	mu     sync.Mutex
	status int
	body   any
	keys   []string
}

func newLogsServer(t *testing.T, status int, body any) (*logsServer, context.Context) {
	t.Helper()
	fixture := &logsServer{status: status, body: body}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		assert.Equal(t, "GET /orgs/"+logsTestOrg+"/deployments/"+logsTestDeployment+"/actions/get-logs", r.Method+" "+r.URL.Path)
		fixture.keys = append(fixture.keys, r.Header.Get("DecryptKey"))
		if text, ok := fixture.body.(string); ok {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(fixture.status)
			_, _ = w.Write([]byte(text))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fixture.status)
		assert.NoError(t, json.NewEncoder(w).Encode(fixture.body))
	}))
	t.Cleanup(server.Close)

	cpc, err := cp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	dpc, err := dp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), ConfigContextKey, config.Config{DefaultOrg: logsTestOrg})
	ctx = context.WithValue(ctx, CpClientContextKey, cpc)
	ctx = context.WithValue(ctx, DpClientContextKey, dpc)
	t.Setenv("PO_API_URL", server.URL)
	cobra.EnableTraverseRunHooks = true
	color.NoColor = true
	return fixture, ctx
}

// requestKeys returns the DecryptKey header of each request, "" when absent.
func (f *logsServer) requestKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keys...)
}

func newLogsKey(t *testing.T) string {
	t.Helper()
	key, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	return key.String()
}

// runLogs runs 'octl logs' with stdin as standard input.
func runLogs(ctx context.Context, t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	RootCmd.SetIn(strings.NewReader(stdin))
	t.Cleanup(func() { RootCmd.SetIn(nil) })
	return executeAndResetCommand(ctx, RootCmd, append([]string{orgFlag, logsTestOrg, logsTestCmd, logsTestDeployment}, args...))
}

func TestLogs(t *testing.T) {
	fixture, ctx := newLogsServer(t, http.StatusOK, logsTestText)
	secretKey := newLogsKey(t)

	stdout, _, err := runLogs(ctx, t, "", keyFileFlag, writeTestFile(t, "runner-logs.key", secretKey+"\n"))
	require.NoError(t, err)
	assert.Equal(t, logsTestText, stdout)
	assert.Equal(t, []string{secretKey}, fixture.requestKeys())
}

func TestLogs_without_decryption(t *testing.T) {
	fixture, ctx := newLogsServer(t, http.StatusOK, `<ENCRYPTED_LOGS>`)

	stdout, _, err := runLogs(ctx, t, "")
	require.NoError(t, err)
	assert.Equal(t, `<ENCRYPTED_LOGS>`, stdout)
	assert.Equal(t, []string{""}, fixture.requestKeys(), "no DecryptKey header without --key-file")
}

func TestLogs_error400(t *testing.T) {
	fixture, ctx := newLogsServer(t, http.StatusBadRequest, dp.N400BadRequest{Message: "Secret key not valid"})
	secretKey := newLogsKey(t)

	_, _, err := runLogs(ctx, t, "", keyFileFlag, writeTestFile(t, "runner-logs.key", secretKey))
	require.EqualError(t, err, "unexpected status code 400 when getting deployment logs: Secret key not valid")
	assert.Equal(t, []string{secretKey}, fixture.requestKeys())
}

func TestLogs_error404(t *testing.T) {
	_, ctx := newLogsServer(t, http.StatusNotFound, dp.N404NotFound{Message: "Deployment not found"})

	_, _, err := runLogs(ctx, t, "", keyFileFlag, writeTestFile(t, "runner-logs.key", newLogsKey(t)))
	require.EqualError(t, err, "unexpected status code 404 when getting deployment logs: Deployment not found")
}

func TestLogsReadsTheKeyFromStandardInputOrAnAgeIdentityFile(t *testing.T) {
	secretKey := newLogsKey(t)
	identityFile := "# created: 2026-10-08T09:00:00Z\n# public key: age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs3290gq\n\n" + secretKey + "\r\n"
	for name, input := range map[string]struct{ stdin, keyFile string }{
		"standard input":             {stdin: secretKey + "\n", keyFile: "-"},
		"standard input without EOL": {stdin: secretKey, keyFile: "-"},
		"age-keygen identity file":   {keyFile: identityFile},
		"identity on standard input": {stdin: identityFile, keyFile: "-"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ctx := newLogsServer(t, http.StatusOK, logsTestText)
			keyFile := input.keyFile
			if keyFile != "-" {
				keyFile = writeTestFile(t, "runner-logs.key", keyFile)
			}
			stdout, _, err := runLogs(ctx, t, input.stdin, keyFileFlag, keyFile)
			require.NoError(t, err)
			assert.Equal(t, logsTestText, stdout)
			assert.Equal(t, []string{secretKey}, fixture.requestKeys())
		})
	}
}

func TestLogsRefusesKeyInputThatIsNotExactlyOneKey(t *testing.T) {
	secretKey := newLogsKey(t)
	missing := filepath.Join(t.TempDir(), "missing.key")
	for name, tc := range map[string]struct {
		stdin, keyFile string
		err            string
	}{
		"missing file": {keyFile: missing, err: "failed to open the runner logs key file: open " + missing + ": no such file or directory"},
		"empty input":  {stdin: "", keyFile: "-", err: "failed to read the runner logs key from standard input: no secret keys found"},
		"only comments": {stdin: "# public key: age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs3290gq\n", keyFile: "-",
			err: "failed to read the runner logs key from standard input: no secret keys found"},
		"pasted output line": {stdin: "Runner logs key: " + secretKey + "\n", keyFile: "-",
			err: "failed to read the runner logs key from standard input: error at line 1: malformed secret key: mixed case"},
		"two keys": {stdin: secretKey + "\n" + newLogsKey(t) + "\n", keyFile: "-",
			err: "standard input must hold exactly one age secret key of the form AGE-SECRET-KEY-1..., found 2 keys"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ctx := newLogsServer(t, http.StatusOK, logsTestText)
			stdout, stderr, err := runLogs(ctx, t, tc.stdin, keyFileFlag, tc.keyFile)
			require.EqualError(t, err, tc.err)
			assert.Empty(t, fixture.requestKeys(), "no request without a usable key")
			assert.NotContains(t, stdout+stderr+err.Error(), secretKey, "key input is never echoed")
		})
	}
}

func TestLogsNeverTakesTheKeyAsAnArgument(t *testing.T) {
	fixture, ctx := newLogsServer(t, http.StatusOK, logsTestText)
	secretKey := newLogsKey(t)

	_, _, err := runLogs(ctx, t, "", "--key", secretKey)
	require.EqualError(t, err, "unknown flag: --key")
	assert.Empty(t, fixture.requestKeys())
	assert.Nil(t, LogsCmd.Flags().Lookup("key"))
}
