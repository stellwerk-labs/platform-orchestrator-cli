package command

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
)

func TestProviderRetirementAcceptedHTTPReportsPending(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodDelete || r.URL.Path != "/orgs/example/module-providers/aws/old-account" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	client, err := cp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	ctx := withConfiguration(t.Context(), config.Config{DefaultOrg: "example"})
	ctx = context.WithValue(ctx, CpClientContextKey, client)
	command := &cobra.Command{}
	command.SetContext(ctx)
	var output bytes.Buffer
	previousOutput := color.Output
	color.Output = &output
	t.Cleanup(func() { color.Output = previousOutput })

	require.NoError(t, DeleteModuleProvider.RunE(command, []string{"aws", "old-account"}))
	require.EqualValues(t, 1, requests.Load())
	require.Contains(t, output.String(), "Deletion requested for module provider aws.old-account")
	require.Contains(t, output.String(), "Configuration is retained until cleanup completes.")
	require.NotContains(t, output.String(), "deleted from organization")
}
