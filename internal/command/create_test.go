package command

import (
	"net/http"
	"strconv"
	"testing"

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
