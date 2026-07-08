package command

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	testUpdatedProjectName     = "Updated Project"
	projectsTestObjectType     = "project"
	projectsTestListObjectType = "projects"
	projectsTestNotFound       = "not found"
	projectsTestDisplayName    = "DisplayName"
	projectsTestCreatedAt      = "CreatedAt"
)

func TestCreate_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateProjectWithResponse(gomock.Any(), orgId, cp.ProjectCreateBody{
		Id:          testProjectId,
		DisplayName: ref.RefStringEmptyNil(testProjectName),
	}).Return(&cp.CreateProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, projectsTestObjectType, testProjectId, `--set-json={"display_name": "` + testProjectName + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"id": "my-project",
	"display_name": "`+testProjectName+`",
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"status": ""
}`, stdout)
	}
}

func TestDelete_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect listing of module rules
	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
	}).Return(&cp.ListModuleRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RulePage{Items: []cp.RuleSummary{}},
	}, nil)

	// Expect listing of runner rules
	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
	}).Return(&cp.ListRunnerRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RunnerRulePage{Items: []cp.RunnerRuleSummary{}},
	}, nil)

	cpc.EXPECT().DeleteProjectWithResponse(gomock.Any(), orgId, testProjectId, &cp.DeleteProjectParams{
		DeleteRules: ref.Ref(false),
	}).Return(&cp.DeleteProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, projectsTestObjectType, testProjectId})
	assert.NoError(t, err)
}

func TestDelete_project_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect listing of module rules
	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
	}).Return(&cp.ListModuleRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RulePage{Items: []cp.RuleSummary{}},
	}, nil)

	// Expect listing of runner rules
	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
	}).Return(&cp.ListRunnerRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RunnerRulePage{Items: []cp.RunnerRuleSummary{}},
	}, nil)

	cpc.EXPECT().DeleteProjectWithResponse(gomock.Any(), orgId, testProjectId, &cp.DeleteProjectParams{
		DeleteRules: ref.Ref(false),
	}).Return(&cp.DeleteProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: projectsTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, projectsTestObjectType, testProjectId})
	assert.EqualError(t, err, fmt.Sprintf("project 'my-project' not found in org '%s'", orgId))
}

func TestGet_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Project{
			Id:          testProjectId,
			DisplayName: testProjectName,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, projectsTestObjectType, testProjectId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"id": "my-project",
	"display_name": "My Project",
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"status": ""
}`, stdout)
	}
}

func TestGet_project_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: projectsTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, projectsTestObjectType, testProjectId})
	assert.EqualError(t, err, fmt.Sprintf("project 'my-project' not found in org '%s'", orgId))
}

func TestList_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListProjectsWithResponse(gomock.Any(), orgId, &cp.ListProjectsParams{}).Return(&cp.ListProjectsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ProjectPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListProjectsWithResponse(gomock.Any(), orgId, &cp.ListProjectsParams{Page: ref.Ref("next-page")}).Return(&cp.ListProjectsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ProjectPage{Items: []cp.Project{{Id: testProjectId, DisplayName: testProjectName}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, projectsTestListObjectType})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
	"id": "my-project",
	"display_name": "My Project",
	"created_at": "0001-01-01T00:00:00Z",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"status": "",
	"updated_at": "0001-01-01T00:00:00Z"
}]`, stdout)
	}
}

func TestGet_project_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Project{
			Id:          testProjectId,
			DisplayName: testProjectName,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, projectsTestObjectType, testProjectId})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, projectsTestDisplayName)
		assert.Contains(t, stdout, "Uuid")
		assert.Contains(t, stdout, projectsTestCreatedAt)
		assert.Contains(t, stdout, testProjectId)
		assert.Contains(t, stdout, testProjectName)
	}
}

func TestList_project_table(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListProjectsWithResponse(gomock.Any(), orgId, &cp.ListProjectsParams{}).Return(&cp.ListProjectsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ProjectPage{NextPageToken: ref.Ref("next-page")},
	}, nil)

	cpc.EXPECT().ListProjectsWithResponse(gomock.Any(), orgId, &cp.ListProjectsParams{Page: ref.Ref("next-page")}).Return(&cp.ListProjectsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ProjectPage{Items: []cp.Project{{Id: testProjectId, DisplayName: testProjectName}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, "table", testGetCmd, projectsTestListObjectType})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, projectsTestDisplayName)
		assert.Contains(t, stdout, "Uuid")
		assert.Contains(t, stdout, projectsTestCreatedAt)
		assert.Contains(t, stdout, testProjectId)
		assert.Contains(t, stdout, testProjectName)
	}
}

func TestUpdate_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateProjectWithResponse(gomock.Any(), orgId, testProjectId, cp.ProjectUpdateBody{
		DisplayName: testUpdatedProjectName,
	}).Return(&cp.UpdateProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Project{
			Id:          testProjectId,
			DisplayName: testUpdatedProjectName,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, projectsTestObjectType, testProjectId, `--set-json={"display_name": "` + testUpdatedProjectName + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"id": "my-project",
	"display_name": "`+testUpdatedProjectName+`",
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"status": ""
}`, stdout)
	}
}

func TestUpdate_project_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateProjectWithResponse(gomock.Any(), orgId, testProjectId, cp.ProjectUpdateBody{
		DisplayName: testUpdatedProjectName,
	}).Return(&cp.UpdateProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: projectsTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, projectsTestObjectType, testProjectId, `--set-json={"display_name": "` + testUpdatedProjectName + `"}`})
	assert.EqualError(t, err, fmt.Sprintf("project 'my-project' not found in org '%s'", orgId))
}

func TestDelete_project_with_delete_rules_flag(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// When --delete-rules is set, no listing should occur
	cpc.EXPECT().DeleteProjectWithResponse(gomock.Any(), orgId, testProjectId, &cp.DeleteProjectParams{
		DeleteRules: ref.Ref(true),
	}).Return(&cp.DeleteProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, projectsTestObjectType, testProjectId, "--delete-rules"})
	assert.NoError(t, err)
}

func TestDelete_project_with_rules_prompt_error(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	moduleRuleId := uuid.New()

	// Expect listing of module rules
	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
	}).Return(&cp.ListModuleRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.RulePage{Items: []cp.RuleSummary{
			{Id: moduleRuleId, ModuleId: "s3-module", ResourceType: "s3"},
		}},
	}, nil)

	// Expect listing of runner rules
	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
	}).Return(&cp.ListRunnerRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RunnerRulePage{Items: []cp.RunnerRuleSummary{}},
	}, nil)

	// Should fail when trying to prompt without --no-prompt
	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, projectsTestObjectType, testProjectId})
	assert.Error(t, err)
}
