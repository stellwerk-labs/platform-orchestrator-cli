package command

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stellwerk-labs/platform-orchestrator-cli/clients/pipelines"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineClientUsesConfiguredCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "Bearer fixture-token", req.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, certificate, 0o600))
	t.Setenv(CAFileEnvVar, caFile)
	ctx, err := withClients(t.Context(), server.URL, "org", "fixture-token")
	require.NoError(t, err)
	response, err := MustPipelineClient(ctx).GetProjectPipelineSchemaWithResponse(ctx, "org", "project")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode())
}

func TestReadPipelineDocument(t *testing.T) {
	for _, tc := range []struct {
		name, input, contentType string
		valid                    bool
	}{
		{"json", `{"slug":"example"}`, "application/json", true},
		{"yaml", "slug: example\n", "application/yaml", true},
		{"server rejects duplicate keys", "slug: a\nslug: b\n", "application/yaml", true},
		{"empty", " \n", "", false},
		{"over limit", strings.Repeat("x", pipelineDocumentLimit+1), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("file", "-", "")
			cmd.SetIn(strings.NewReader(tc.input))
			raw, kind, err := readPipelineDocument(cmd)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.input, string(raw))
			require.Equal(t, tc.contentType, kind)
		})
	}
}
func TestPipelineRevision(t *testing.T) {
	for _, version := range []int64{-1, 0, 1, 9223372036854775807} {
		cmd := &cobra.Command{}
		cmd.Flags().Int64("expected-version", version, "")
		revision, err := pipelineRevision(cmd)
		if version < 1 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, '"', rune(revision[0]))
			require.Equal(t, '"', rune(revision[len(revision)-1]))
		}
	}
}

// TestPipelineLiveLifecycle exercises the real command handlers against an isolated control plane.
func TestPipelineLiveLifecycle(t *testing.T) {
	project := os.Getenv("PIPELINE_TEST_PROJECT")
	if project == "" {
		t.Skip("PIPELINE_TEST_PROJECT must identify an isolated acceptance project")
	}
	org, apiURL, token := os.Getenv("PO_ORG_ID"), os.Getenv("PO_API_URL"), os.Getenv("PO_AUTH_TOKEN")
	require.NotEmpty(t, org)
	require.NotEmpty(t, apiURL)
	require.NotEmpty(t, token)
	ctx, err := withClients(t.Context(), apiURL, org, token)
	require.NoError(t, err)
	ctx = withConfiguration(ctx, config.Config{DefaultOrg: org})
	run := func(operation, id, input string, version int64, format string, details bool) (string, error) {
		cmd := &cobra.Command{}
		configurePipelineFlags(cmd, operation)
		if operation == "list" {
			cmd.Flags().String("out", format, "")
		}
		if operation != "delete" {
			require.NoError(t, cmd.Flags().Set("out", format))
		}
		if input != "" {
			require.NoError(t, cmd.Flags().Set("file", "-"))
			cmd.SetIn(strings.NewReader(input))
		}
		if version > 0 {
			require.NoError(t, cmd.Flags().Set("expected-version", strconv.FormatInt(version, 10)))
		}
		if details {
			require.NoError(t, cmd.Flags().Set("details", "true"))
		}
		output := new(bytes.Buffer)
		cmd.SetOut(output)
		cmd.SetContext(ctx)
		args := []string{project}
		if id != "" {
			args = append(args, id)
		}
		err := runPipelineCommand(cmd, args, operation) //nolint:contextcheck // The command carries the test context via SetContext above.
		return output.String(), err
	}
	slug := "cli-" + uuid.NewString()
	original := "slug: " + slug + "\nname: CLI acceptance\ndefinition:\n  strategy: apply\n  steps: []\n"
	output, err := run("create", "", original, 0, "json", false)
	require.NoError(t, err)
	var draft pipelines.PipelineDraft
	require.NoError(t, json.Unmarshal([]byte(output), &draft))
	revision := draft.ResourceVersion
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		response, err := MustPipelineClient(ctx).DeleteProjectPipelineDraftWithResponse(cleanupCtx, org, project, draft.Id, &pipelines.DeleteProjectPipelineDraftParams{IfMatch: strconv.Quote(strconv.FormatInt(revision, 10))})
		require.NoError(t, err)
		require.Equal(t, 204, response.StatusCode())
	})
	output, err = run("get", draft.Id.String(), "", 0, "yaml", false)
	require.NoError(t, err)
	require.Contains(t, output, "slug: "+slug)
	require.NotContains(t, output, "resource_version")
	output, err = run("get", draft.Id.String(), "", 0, "json", true)
	require.NoError(t, err)
	require.Contains(t, output, `"resource_version": 1`)
	document := pipelines.PipelineDraftCreate{Slug: draft.Slug, Name: "Updated", Definition: draft.Definition}
	raw, err := json.Marshal(document)
	require.NoError(t, err)
	output, err = run("update", draft.Id.String(), string(raw), revision, "json", false)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(output), &draft))
	revision = draft.ResourceVersion
	require.EqualValues(t, 2, revision)
	_, err = run("update", draft.Id.String(), original, 1, "json", false)
	require.ErrorContains(t, err, "conflict")
	output, err = run("validate", "", "slug: a\nslug: b\n", 0, "json", false)
	require.ErrorContains(t, err, "invalid")
	require.Contains(t, output, `"valid": false`)
	output, err = run("list", "", "", 0, "table", false)
	require.NoError(t, err)
	require.Contains(t, output, slug)
}

func TestPipelineDebugKeepsDocumentPrivate(t *testing.T) {
	const marker = "private-pipeline-document-marker"
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		received <- string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"valid":true,"errors":[],"document":{"slug":"private","name":"`+marker+`","definition":{"strategy":"apply","steps":[]}}}`)
	}))
	defer server.Close()
	logs := new(bytes.Buffer)
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	ctx, err := withClients(t.Context(), server.URL, "test", "test-token")
	require.NoError(t, err)
	source := "# " + marker + "\nslug: private\nname: Private\ndefinition: {strategy: apply, steps: []}\n"
	result, err := MustPipelineClient(ctx).ValidateProjectPipelineDocumentWithBodyWithResponse(ctx, "test", "project", "application/yaml", strings.NewReader(source))
	require.NoError(t, err)
	require.Equal(t, 200, result.StatusCode())
	require.Equal(t, source, <-received)
	require.NotContains(t, logs.String(), marker)
	require.NotContains(t, logs.String(), "test-token")
}
