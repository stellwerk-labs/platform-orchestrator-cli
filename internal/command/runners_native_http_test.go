package command

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const nativeRunnerID = "native-pool"
const nativeRunnerOrg = "native-http-org"
const nativeRunnerCollectionPath = "/orgs/" + nativeRunnerOrg + "/runners"
const nativeRunnerUUID = "719870ca-5494-4ac2-8850-fef3b3472111"
const nativeRunnerKey = "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAc5dgCx4ano39JT0XgTsHnts3jej+5xl7ZAwSIrKpef0=\n-----END PUBLIC KEY-----"

func nativeRunnerCreateInput(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"description": "accepted-native-value-canary",
		"runner_configuration": map[string]any{
			"type": "native-agent", "key": nativeRunnerKey, "max_parallel_jobs": 2,
			"supported_protocols": []string{"stellwerk.pipeline/v2", "stellwerk.pipeline.plan-apply/v2"},
			"capabilities":        []string{"local.exec"},
			"native_execution":    map[string]any{"contract_version": "stellwerk.runner.catalogue/v1", "runtime_version": "stellwerk.runner.rollout/v2", "tools_digest": strings.Repeat("a", 64), "tools": []any{}},
		},
		"state_storage_configuration": map[string]any{"type": "s3", "bucket": "retained-state"},
	})
	require.NoError(t, err)
	return raw
}

// This owned wire fixture qualifies CLI binding, printing and errors. Core's
// validation, authorization and persistence are tested by the Core suite.
func nativeRunnerResponse(t *testing.T) cp.Runner {
	t.Helper()
	var runner cp.Runner
	require.NoError(t, json.Unmarshal([]byte(`{"id":"native-pool","org_id":"example","uuid":"`+nativeRunnerUUID+`","created_at":"2026-11-20T10:00:00Z","updated_at":"2026-11-20T10:00:00Z","runner_configuration":{"type":"native-agent","max_parallel_jobs":2},"state_storage_configuration":{"type":"s3","bucket":"retained-state"}}`), &runner))
	runner.Id = nativeRunnerID
	runner.OrgId = nativeRunnerOrg
	native, err := runner.RunnerConfiguration.AsNativeAgentRunnerConfiguration()
	require.NoError(t, err)
	native.Key = nativeRunnerKey
	require.NoError(t, runner.RunnerConfiguration.FromNativeAgentRunnerConfiguration(native))
	return runner
}

type runnerWireRequest struct {
	method, path, authorization string
	body                        []byte
}

func nativeRunnerHTTPCommand(t *testing.T, status int, response any) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, <-chan runnerWireRequest) {
	t.Helper()
	requests := make(chan runnerWireRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read request", http.StatusBadRequest)
			return
		}
		requests <- runnerWireRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), raw}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	ctx := withConfiguration(t.Context(), config.Config{DefaultOrg: nativeRunnerOrg})
	ctx, err := withClients(ctx, server.URL, nativeRunnerOrg, "fixture-token")
	require.NoError(t, err)
	ctx, err = withPrinter(ctx, printer.JsonPrinterType, []string{printer.JsonPrinterType})
	require.NoError(t, err)
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.Flags().String(createUpdateCmdSetJsonFlag, "", "")
	cmd.Flags().String(createUpdateCmdSetYamlFlag, "", "")
	cmd.Flags().StringArray(createUpdateCmdSetFlag, nil, "")
	output, messages := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(messages)
	previous := color.Output
	color.Output = messages
	t.Cleanup(func() { color.Output = previous })
	return cmd, output, messages, requests
}

func TestNativeRunnerCreateHTTPInputFormats(t *testing.T) {
	input := nativeRunnerCreateInput(t)
	yamlInput, err := yaml.JSONToYAML(input)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, flag string
		source     byte
		body       []byte
	}{
		{"raw JSON", createUpdateCmdSetJsonFlag, 0, input},
		{"JSON file", createUpdateCmdSetJsonFlag, '@', input},
		{"JSON stdin", createUpdateCmdSetJsonFlag, '-', input},
		{"raw YAML", createUpdateCmdSetYamlFlag, 0, yamlInput},
		{"YAML file", createUpdateCmdSetYamlFlag, '@', yamlInput},
		{"YAML stdin", createUpdateCmdSetYamlFlag, '-', yamlInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := nativeRunnerResponse(t)
			require.NoError(t, json.Unmarshal(input, &response))
			response.Id = nativeRunnerID
			cmd, out, messages, requests := nativeRunnerHTTPCommand(t, http.StatusCreated, response)
			value := string(tc.body)
			switch tc.source {
			case '@':
				path := filepath.Join(t.TempDir(), "runner-input")
				require.NoError(t, os.WriteFile(path, tc.body, 0600))
				value = "@" + path
			case '-':
				value = "-"
				cmd.SetIn(bytes.NewReader(tc.body))
			}
			require.NoError(t, cmd.Flags().Set(tc.flag, value))
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			require.NoError(t, CreateRunner.RunE(cmd, []string{nativeRunnerID}))
			request := <-requests
			require.Equal(t, http.MethodPost, request.method)
			require.Equal(t, nativeRunnerCollectionPath, request.path)
			require.Equal(t, "Bearer fixture-token", request.authorization)
			var body cp.RunnerCreateBody
			require.NoError(t, json.Unmarshal(request.body, &body))
			require.Equal(t, nativeRunnerID, body.Id)
			native, err := body.RunnerConfiguration.AsNativeAgentRunnerConfiguration()
			require.NoError(t, err)
			require.Equal(t, cp.RunnerType("native-agent"), native.Type)
			require.Equal(t, 2, native.MaxParallelJobs)
			require.Equal(t, nativeRunnerKey, string(native.Key))
			require.Equal(t, []string{"local.exec"}, *native.Capabilities)
			require.Equal(t, []string{"stellwerk.pipeline/v2", "stellwerk.pipeline.plan-apply/v2"}, *native.SupportedProtocols)
			require.Equal(t, strings.Repeat("a", 64), native.NativeExecution.ToolsDigest)
			require.NotContains(t, string(request.body), `"uuid"`)
			require.Contains(t, out.String(), nativeRunnerUUID)
			require.Contains(t, messages.String(), "created in organization")
			require.NotContains(t, logs.String(), "accepted-native-value-canary")
			require.NotContains(t, logs.String(), "MCowBQYDK2VwAyEAc5dgCx4ano39JT0XgTsHnts3jej+5xl7ZAwSIrKpef0=")
			require.NotContains(t, logs.String(), "fixture-token")
		})
	}
}

func TestNativeRunnerReadHTTPPreservesDisabledAbsence(t *testing.T) {
	runner := nativeRunnerResponse(t)
	cmd, out, _, requests := nativeRunnerHTTPCommand(t, http.StatusOK, runner)
	require.NoError(t, GetRunner.RunE(cmd, []string{nativeRunnerID}))
	require.Equal(t, nativeRunnerCollectionPath+"/"+nativeRunnerID, (<-requests).path)
	var read cp.Runner
	require.NoError(t, json.Unmarshal(out.Bytes(), &read))
	require.Equal(t, nativeRunnerUUID, read.Uuid.String())
	native, err := read.RunnerConfiguration.AsNativeAgentRunnerConfiguration()
	require.NoError(t, err)
	require.Nil(t, native.NativeExecution)
	require.Nil(t, native.Capabilities)
	require.Nil(t, native.SupportedProtocols)
	require.Equal(t, nativeRunnerKey, string(native.Key))
	require.Equal(t, 2, native.MaxParallelJobs)
	require.NotContains(t, out.String(), "native_execution")

	var summary cp.RunnerSummary
	raw, err := json.Marshal(runner)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &summary))
	cmd, out, _, requests = nativeRunnerHTTPCommand(t, http.StatusOK, map[string]any{"items": []cp.RunnerSummary{summary}})
	require.NoError(t, ListRunners.RunE(cmd, nil))
	require.Equal(t, nativeRunnerCollectionPath, (<-requests).path)
	require.Contains(t, out.String(), nativeRunnerUUID)
	require.Contains(t, out.String(), "native-agent")
	require.NotContains(t, out.String(), "native_execution")
}

func TestNativeRunnerPatchHTTPKeepsOmissionAndClearDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		enabled     bool
	}{
		{"description only", `{"description":"description-only"}`, false},
		{"clear inventory", `{"runner_configuration":{"type":"native-agent","native_execution":null}}`, false},
		{"explicit reenable", string(nativeRunnerCreateInput(t)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields map[string]any
			require.NoError(t, json.Unmarshal([]byte(tc.input), &fields))
			delete(fields, "state_storage_configuration")
			input, err := json.Marshal(fields)
			require.NoError(t, err)
			response := nativeRunnerResponse(t)
			if tc.enabled {
				require.NoError(t, json.Unmarshal(input, &response))
			}
			cmd, out, messages, requests := nativeRunnerHTTPCommand(t, http.StatusOK, response)
			require.NoError(t, cmd.Flags().Set(createUpdateCmdSetJsonFlag, string(input)))
			require.NoError(t, UpdateRunner.RunE(cmd, []string{nativeRunnerID}))
			request := <-requests
			require.Equal(t, http.MethodPatch, request.method)
			require.Equal(t, nativeRunnerCollectionPath+"/"+nativeRunnerID, request.path)
			require.JSONEq(t, string(input), string(request.body))
			require.Contains(t, out.String(), nativeRunnerUUID)
			var read cp.Runner
			require.NoError(t, json.Unmarshal(out.Bytes(), &read))
			native, err := read.RunnerConfiguration.AsNativeAgentRunnerConfiguration()
			require.NoError(t, err)
			require.Equal(t, tc.enabled, native.NativeExecution != nil)
			require.Contains(t, messages.String(), "updated in organization")
		})
	}
}

func TestNativeRunnerReadonlyAndPrivateInputRejectedBeforeHTTP(t *testing.T) {
	for _, operation := range []*cobra.Command{CreateRunner, UpdateRunner} {
		for _, tc := range []struct{ name, input string }{
			{name: "read-only UUID", input: `{"uuid":"` + nativeRunnerUUID + `"}`},
			{name: "private local configuration", input: `{"postgres":{"dsn":"rejected-private-value-canary"}}`},
		} {
			t.Run(operation.Short+tc.name, func(t *testing.T) {
				cmd, out, messages, requests := nativeRunnerHTTPCommand(t, http.StatusCreated, nativeRunnerResponse(t))
				require.NoError(t, cmd.Flags().Set(createUpdateCmdSetJsonFlag, tc.input))
				var logs bytes.Buffer
				previous := slog.Default()
				slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
				t.Cleanup(func() { slog.SetDefault(previous) })
				require.Error(t, operation.RunE(cmd, []string{nativeRunnerID}))
				require.Empty(t, requests)
				require.NotContains(t, logs.String(), "rejected-private-value-canary")
				require.Empty(t, out.String())
				require.Empty(t, messages.String())
			})
		}
	}
}

func TestNativeRunnerHTTPErrorsNeverReportSuccess(t *testing.T) {
	for _, operation := range []*cobra.Command{CreateRunner, UpdateRunner, DeleteRunner} {
		for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusConflict} {
			t.Run(operation.Short+http.StatusText(status), func(t *testing.T) {
				cmd, out, messages, requests := nativeRunnerHTTPCommand(t, status, cp.Error{Message: "registration retained"})
				require.NoError(t, cmd.Flags().Set(createUpdateCmdSetJsonFlag, string(nativeRunnerCreateInput(t))))
				err := operation.RunE(cmd, []string{nativeRunnerID})
				require.ErrorContains(t, err, "registration retained")
				require.Equal(t, nativeRunnerCollectionPath, strings.TrimSuffix((<-requests).path, "/"+nativeRunnerID))
				require.Empty(t, out.String())
				require.Empty(t, messages.String())
				if operation == DeleteRunner && status == http.StatusConflict {
					require.ErrorContains(t, err, "cannot be deleted")
				}
			})
		}
	}
}
