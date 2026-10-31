package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPSubprocess(t *testing.T) {
	if os.Getenv("OCTL_MCP_TEST_PROCESS") != "1" {
		return
	}
	os.Args = append([]string{"octl", "mcp", "serve", "--debug"}, strings.Fields(os.Getenv("OCTL_MCP_TEST_EXTRA"))...)
	main()
	os.Exit(0)
}

func TestMCPStdio(t *testing.T) {
	var requests atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/orgs/explicit-org/projects", r.URL.Path)
		assert.Equal(t, "Bearer private-stdio-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer api.Close()
	var stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestMCPSubprocess$") //nolint:gosec // Re-executes this test binary.
	cmd.Stderr = &stderr
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "PO_") && !strings.HasPrefix(env, "HOME=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "OCTL_MCP_TEST_PROCESS=1", "HOME="+t.TempDir(), "PO_API_URL="+api.URL, "PO_AUTH_TOKEN=private-stdio-token", "PO_CLI_DEBUG=true", "PO_ORG_ID=unused-default")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-tests", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	require.Equal(t, int32(0), requests.Load(), "startup should not contact API or version services")
	tools, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 8)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{"org_id": "explicit-org"}})
	require.NoError(t, err)
	require.False(t, res.IsError)
	invalid, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	require.True(t, err != nil || invalid.IsError)
	require.NoError(t, cs.Close())
	require.Equal(t, int32(1), requests.Load())
	require.Empty(t, stderr.String(), "debug mode must not log metadata requests or credentials")
}

func TestMCPErrorsUseStderr(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestMCPSubprocess$") //nolint:gosec // Re-executes this test binary.
	cmd.Env = append(os.Environ(), "OCTL_MCP_TEST_PROCESS=1", "PO_API_URL=https://user:private-value@example.com")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.Error(t, err)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "invalid MCP API endpoint")
	require.NotContains(t, stderr.String(), "private-value")
	var value any
	require.Error(t, json.Unmarshal(stderr.Bytes(), &value))
}

func TestMCPInvalidCLIArguments(t *testing.T) {
	for _, args := range []string{"unexpected", "--listen=127.0.0.1:9000", "--transport=http"} {
		t.Run(args, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMCPSubprocess$") //nolint:gosec // Re-executes this test binary.
			cmd.Env = append(os.Environ(), "OCTL_MCP_TEST_PROCESS=1", "OCTL_MCP_TEST_EXTRA="+args)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.Error(t, cmd.Run())
			require.Empty(t, stdout.String())
			require.NotEmpty(t, stderr.String())
		})
	}
}
