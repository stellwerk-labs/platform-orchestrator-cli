package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
)

const moduleOutputDeclaration = `{"type":"object","properties":{"endpoint":{"type":"string"}},"required":["endpoint"],"additionalProperties":false}`

func modulePublicationCommand(t *testing.T, body string) *cobra.Command {
	t.Helper()
	command := &cobra.Command{}
	command.Flags().String(createUpdateCmdSetJsonFlag, body, "")
	command.Flags().String(createUpdateCmdSetYamlFlag, "", "")
	command.Flags().StringArray(createUpdateCmdSetFlag, nil, "")
	command.Flags().String(moduleVersionIdempotencyFlag, "module-output-contract", "")
	return command
}

func TestModuleVersionPublicationOutputSchemaHTTPRoundTrip(t *testing.T) {
	for _, declaration := range []string{"", `{}`, moduleOutputDeclaration} {
		t.Run(declaration, func(t *testing.T) {
			input := `{"semantic_version":"1.0.0","module_source":"inline","module_source_code":"output \"endpoint\" {value = \"example\"}","module_params":{},"module_inputs":{},"provider_mapping":{},"dependencies":{},"coprovisioned":[]}`
			var wanted map[string]any
			require.NoError(t, json.Unmarshal([]byte(input), &wanted))
			if declaration != "" {
				var schema map[string]any
				require.NoError(t, json.Unmarshal([]byte(declaration), &schema))
				wanted["output_schema"] = schema
			}
			encoded, err := json.Marshal(wanted)
			require.NoError(t, err)
			requests := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if !assert.Equal(t, http.MethodPost, request.Method) ||
					!assert.Equal(t, "/orgs/example/modules/volume/versions", request.URL.Path) ||
					!assert.Equal(t, "module-output-contract", request.Header.Get("Idempotency-Key")) {
					http.Error(w, "request contract mismatch", http.StatusBadRequest)
					return
				}
				var received cp.ModuleVersionPublishBody
				decoder := json.NewDecoder(request.Body)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&received); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				raw, err := json.Marshal(received)
				if !assert.NoError(t, err) {
					http.Error(w, "encoding failed", http.StatusInternalServerError)
					return
				}
				var recorded map[string]any
				if !assert.NoError(t, json.Unmarshal(raw, &recorded)) {
					http.Error(w, "decoding failed", http.StatusInternalServerError)
					return
				}
				requests <- recorded
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprint(w, `{"semantic_version":"1.0.0","lifecycle_status":"proposed"}`)
			}))
			t.Cleanup(server.Close)
			client, err := cp.NewClientWithResponses(server.URL)
			require.NoError(t, err)
			command := modulePublicationCommand(t, string(encoded))
			ctx := context.WithValue(t.Context(), ConfigContextKey, config.Config{DefaultOrg: "example"})
			ctx = context.WithValue(ctx, CpClientContextKey, client)
			ctx = context.WithValue(ctx, PrinterContextKey, &printer.JsonPrinter{})
			command.SetContext(ctx)
			command.SetOut(&bytes.Buffer{})
			require.NoError(t, CreateManagedModuleVersion.RunE(command, []string{"volume"}))
			received := <-requests
			require.Equal(t, wanted, received)
			_, declared := received["output_schema"]
			require.Equal(t, declaration != "", declared, "absence differs from a declared empty object")
		})
	}
}

func TestModuleVersionAuthoringRejectsUnknownFields(t *testing.T) {
	for _, field := range []string{"resource_type", "output_scehma", "unexpected"} {
		command := modulePublicationCommand(t, fmt.Sprintf(`{"%s":{}}`, field))
		_, err := readSetFlagsIntoType[cp.ModuleVersionPublishBody](command)
		require.ErrorContains(t, err, "unknown field")
	}
	command := modulePublicationCommand(t, `{"version":{"resource_type":"volume"}}`)
	_, err := readSetFlagsIntoType[cp.StableModuleVersionSuccessorBody](command)
	require.ErrorContains(t, err, `unknown field "resource_type"`)
	command = modulePublicationCommand(t, `{"version":{"output_schema":`+moduleOutputDeclaration+`}}`)
	body, err := readSetFlagsIntoType[cp.StableModuleVersionSuccessorBody](command)
	require.NoError(t, err)
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	var recorded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &recorded))
	require.Contains(t, recorded["version"].(map[string]any), "output_schema")
	require.Contains(t, CreateManagedModuleVersion.Long, "output_schema")
}

func TestModuleVersionDefinitionOutputSchemaDecodeRoundTrip(t *testing.T) {
	for _, declaration := range []string{"", `{}`, moduleOutputDeclaration} {
		t.Run(declaration, func(t *testing.T) {
			definition := map[string]any{"module_source": "inline"}
			if declaration != "" {
				var schema map[string]any
				require.NoError(t, json.Unmarshal([]byte(declaration), &schema))
				definition["output_schema"] = schema
			}
			raw, err := json.Marshal(definition)
			require.NoError(t, err)
			var version cp.ModuleVersion
			require.NoError(t, json.Unmarshal(raw, &version))
			encoded, err := json.Marshal(version)
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			value, declared := decoded["output_schema"]
			require.Equal(t, declaration != "", declared)
			require.Equal(t, definition["output_schema"], value)
			var detail cp.CoreModuleVersionDetail
			require.NoError(t, json.Unmarshal([]byte(`{"definition":`+string(raw)+`}`), &detail))
			encoded, err = json.Marshal(detail.Definition)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, definition["output_schema"], decoded["output_schema"])
			var snapshot cp.ModuleVersionComparisonSnapshot
			require.NoError(t, json.Unmarshal(raw, &snapshot))
			encoded, err = json.Marshal(snapshot)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			_, declared = decoded["output_schema"]
			require.Equal(t, declaration != "", declared)
			require.Equal(t, definition["output_schema"], decoded["output_schema"])
		})
	}
}
