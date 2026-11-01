package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testListProjects = "list_projects"
	testOrgKey       = "org_id"
	testProjectKey   = "project_id"
	testOrg          = "acme"
	testProject      = "project"
	testModule       = "bucket"
	testVersion      = "1.2.0"
)

func sessionFor(t *testing.T, handler http.HandlerFunc) *mcp.ClientSession {
	t.Helper()
	api := httptest.NewServer(handler)
	t.Cleanup(api.Close)
	cpclient, err := NewClient(api.URL, "secret-auth-token", "")
	require.NoError(t, err)
	server := New(cpclient, "test")
	st, ct := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "tests", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestToolCatalogueAndMetadataWire(t *testing.T) {
	fixtures := map[string]string{
		"/orgs/acme/projects":                      `{"items":[{"id":"project","display_name":"Project","status":"active"}],"next_page_token":"next"}`,
		"/orgs/acme/projects/project":              `{"id":"project","display_name":"Project","status":"active"}`,
		"/orgs/acme/projects/project/envs":         `{"items":[{"id":"dev","project_id":"project","display_name":"Dev","env_type_id":"development","status":"active","labels":{"secret":"sensitive-value"},"runner_id":"private-runner","status_message":"sensitive-value"}]}`,
		"/orgs/acme/projects/project/envs/dev":     `{"id":"dev","project_id":"project","display_name":"Dev","env_type_id":"development","status":"active","labels":{"secret":"sensitive-value"},"runner_id":"private-runner"}`,
		"/orgs/acme/module-catalogue":              `[{"slug":"bucket","resource_type":"s3","status":"active","description":"sensitive-value","tags":{"secret":"sensitive-value"}}]`,
		"/orgs/acme/modules/bucket/versions":       `{"items":[{"version_id":"1.2.0","version":{"opaque_version_id":"1.2.0","lifecycle_status":"active","verification_status":"unverified"},"module_source_code":"sensitive-value","module_inputs":{"secret":"sensitive-value"},"provider_mapping":{"secret":"sensitive-value"},"definition":{"module_source_code":"sensitive-value"}}]}`,
		"/orgs/acme/modules/bucket/versions/1.2.0": `{"version_id":"1.2.0","version":{"opaque_version_id":"1.2.0","lifecycle_status":"active","verification_status":"unverified"},"module_source_code":"sensitive-value","module_inputs":{"secret":"sensitive-value"},"provider_mapping":{"secret":"sensitive-value"}}`,
		"/orgs/acme/resource-types/s3":             `{"id":"s3","catalogue_status":"active","resource_version":2,"built_in":true,"is_developer_accessible":true,"description":"sensitive-value","output_schema":{"type":"object","properties":{"value":{"type":"string","default":"sensitive-value"}},"additionalProperties":false},"module_contract":{"secret":"sensitive-value"}}`,
	}
	var requests atomic.Int32
	cs := sessionFor(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer secret-auth-token", r.Header.Get("Authorization"))
		body, ok := fixtures[r.URL.Path]
		assert.True(t, ok, "unexpected path %s", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	tools, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 8)
	for _, tool := range tools.Tools {
		require.True(t, tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
		require.True(t, *tool.Annotations.OpenWorldHint)
	}
	cases := []struct {
		name     string
		args     map[string]any
		expected string
	}{
		{testListProjects, map[string]any{testOrgKey: testOrg}, "Project"},
		{"get_project", map[string]any{testOrgKey: testOrg, testProjectKey: testProject}, "Project"},
		{"list_environments", map[string]any{testOrgKey: testOrg, testProjectKey: testProject}, "development"},
		{"get_environment", map[string]any{testOrgKey: testOrg, testProjectKey: testProject, "environment_id": "dev"}, "development"},
		{"list_modules", map[string]any{testOrgKey: testOrg}, testModule},
		{"list_module_versions", map[string]any{testOrgKey: testOrg, "module_id": testModule}, testVersion},
		{"get_module_version", map[string]any{testOrgKey: testOrg, "module_id": testModule, "version_id": testVersion}, testVersion},
		{"get_resource_type", map[string]any{testOrgKey: testOrg, "type_id": "s3"}, "output_field_count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			require.NoError(t, err)
			require.False(t, res.IsError, "%+v", res.Content)
			b, err := json.Marshal(res)
			require.NoError(t, err)
			require.Contains(t, string(b), tc.expected)
			for _, secret := range []string{"sensitive-value", "secret-auth-token", "private-runner", "module_source_code", "module_inputs", "provider_mapping", "module_contract", "output_schema"} {
				require.NotContains(t, string(b), secret)
			}
		})
	}
	require.Equal(t, int32(8), requests.Load())
}

func TestInvalidToolArgumentsNeverReachAPI(t *testing.T) {
	cs := sessionFor(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid arguments reached API") })
	cases := []map[string]any{
		{}, {testOrgKey: testOrg, "unexpected": "value"}, {testOrgKey: "../other"}, {testOrgKey: "acme%2fother"}, {testOrgKey: "acme/other"}, {testOrgKey: "acme?x=y"}, {testOrgKey: testOrg, "limit": 101}, {testOrgKey: testOrg, "limit": -1}, {testOrgKey: testOrg, "page": strings.Repeat("x", 1025)},
	}
	for _, args := range cases {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: testListProjects, Arguments: args})
		require.True(t, err != nil || res.IsError, "accepted %+v", args)
	}
	_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_project", Arguments: map[string]any{testOrgKey: testOrg}})
	require.Error(t, err)
}

func TestAPIErrorsAreSanitized(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			cs := sessionFor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"message":"secret-auth-token sensitive-value",broken`))
			})
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: testListProjects, Arguments: map[string]any{testOrgKey: testOrg}})
			require.NoError(t, err)
			require.True(t, res.IsError)
			b, err := json.Marshal(res)
			require.NoError(t, err)
			require.NotContains(t, string(b), "secret-auth-token")
			require.NotContains(t, string(b), "sensitive-value")
		})
	}
}

func TestToolCancellationPropagatesToAPI(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	cs := sessionFor(t, func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(cancelled) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cs.CallTool(ctx, &mcp.CallToolParams{Name: testListProjects, Arguments: map[string]any{testOrgKey: testOrg}})
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("API request did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled MCP call did not return")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("MCP cancellation did not cancel API request")
	}
}

func TestMetadataResultLimits(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		cs := sessionFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			items := make([]map[string]string, 101)
			for i := range items {
				items[i] = map[string]string{"slug": "module"}
			}
			assert.NoError(t, json.NewEncoder(w).Encode(items))
		})
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_modules", Arguments: map[string]any{testOrgKey: testOrg, "limit": 100}})
		require.NoError(t, err)
		require.False(t, res.IsError)
		b, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		var page Page[ModuleMetadata]
		require.NoError(t, json.Unmarshal(b, &page))
		require.Len(t, page.Items, 100)
		require.True(t, page.Truncated)
	})
	t.Run("bytes", func(t *testing.T) {
		cs := sessionFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"project","display_name":"` + strings.Repeat("x", 129<<10) + `"}]}`))
		})
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: testListProjects, Arguments: map[string]any{testOrgKey: testOrg}})
		require.NoError(t, err)
		require.True(t, res.IsError)
		b, err := json.Marshal(res)
		require.NoError(t, err)
		require.Less(t, len(b), 1024)
	})
}

func TestResourceTypeOutputFieldCountUsesJSONSchemaProperties(t *testing.T) {
	for _, tc := range []struct {
		name, schema string
		count        int
	}{
		{"one", `{"type":"object","properties":{"value":{"type":"string"}},"additionalProperties":false}`, 1},
		{"two", `{"type":"object","properties":{"value":{"type":"string"},"count":{"type":"integer"}},"required":["value"]}`, 2},
		{"missing", `{"type":"object","description":"sensitive-value"}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := sessionFor(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"s3","output_schema":` + tc.schema + `}`))
			})
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_resource_type", Arguments: map[string]any{testOrgKey: testOrg, "type_id": "s3"}})
			require.NoError(t, err)
			require.False(t, res.IsError)
			b, err := json.Marshal(res.StructuredContent)
			require.NoError(t, err)
			var metadata ResourceTypeMetadata
			require.NoError(t, json.Unmarshal(b, &metadata))
			require.Equal(t, tc.count, metadata.OutputFieldCount)
			require.NotContains(t, string(b), "properties")
			require.NotContains(t, string(b), "sensitive-value")
		})
	}
}
