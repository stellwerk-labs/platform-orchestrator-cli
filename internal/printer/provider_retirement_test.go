package printer

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
)

func TestProviderTablesExposePendingRetirement(t *testing.T) {
	requested := time.Date(2026, time.November, 12, 1, 2, 3, 0, time.UTC)
	for _, item := range []any{
		cp.ModuleProvider{Id: "old-account", DeletionRequestedAt: &requested},
		cp.ModuleProviderSummary{Id: "old-account", DeletionRequestedAt: &requested},
	} {
		var output bytes.Buffer
		require.NoError(t, (&TablePrinter{}).Write(&output, item))
		require.Contains(t, output.String(), "DeletionRequestedAt")
		require.Contains(t, output.String(), "2026-11-12 01:02:03")
	}
}
