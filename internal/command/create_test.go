package command

import (
	"bytes"
	"log/slog"
	"net/http"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const createTestProjectObjectType = "project"

func TestGenerateFieldDoc(t *testing.T) {
	assert.Equal(t, "description (string), id (string), runner_configuration (map), state_storage_configuration (map)", generateTopLevelSetFields(cp.RunnerCreateBody{}))
	assert.Equal(t, "configuration (map), description (string), id (string), provider_type (string), source (string), version_constraint (string)", generateTopLevelSetFields(cp.ModuleProviderCreateBody{}))
}

func TestInputFieldDebugLogKeepsAcceptedAndRejectedValuesPrivate(t *testing.T) {
	previous := slog.Default()
	defer slog.SetDefault(previous)
	for _, tc := range []struct {
		name  string
		input string
		valid bool
	}{
		{name: "accepted", input: `{"description":"accepted-value-canary","runner_configuration":{"type":"unknown-kind-canary","key":"credential-value-canary"}}`, valid: true},
		{name: "rejected", input: `{"description":"rejected-value-canary","unknown-field-canary":"credential-value-canary"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			cmd := &cobra.Command{}
			cmd.Flags().String(createUpdateCmdSetJsonFlag, tc.input, "")
			cmd.Flags().String(createUpdateCmdSetYamlFlag, "", "")
			cmd.Flags().StringArray(createUpdateCmdSetFlag, nil, "")
			body, err := readSetFlagsIntoType[cp.RunnerCreateBody](cmd)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, body.Description)
				require.Equal(t, "accepted-value-canary", *body.Description)
				configuration, err := body.RunnerConfiguration.MarshalJSON()
				require.NoError(t, err)
				require.Contains(t, string(configuration), "credential-value-canary", "logging must not alter the parsed input")
			} else {
				require.Error(t, err)
				require.Nil(t, body)
			}
			require.Contains(t, logs.String(), `"msg":"read input fields"`)
			for _, marker := range []string{"accepted-value-canary", "rejected-value-canary", "credential-value-canary", "unknown-kind-canary", "unknown-field-canary"} {
				require.NotContains(t, logs.String(), marker)
			}
			require.NotContains(t, logs.String(), `"input":`)
			require.Contains(t, logs.String(), `"field_count":2`)
		})
	}
}

func TestSetJsonYaml(t *testing.T) {
	for i, tc := range []string{
		`--set-json={"display_name": "` + testProjectName + `"}`,
		`--set-yaml={display_name: "` + testProjectName + `"}`,
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			orgId, cpc, _, ctx, fin := setupTestContext(t)
			defer fin()

			cpc.EXPECT().CreateProjectWithResponse(gomock.Any(), orgId, cp.ProjectCreateBody{
				Id:          testProjectId,
				DisplayName: ref.RefStringEmptyNil(testProjectName),
			}).Return(&cp.CreateProjectResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
				JSON201:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
			}, nil)

			_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testCreateCmd, createTestProjectObjectType, testProjectId, tc})
			require.NoError(t, err)
		})

	}
}
