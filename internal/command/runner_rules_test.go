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

var runnerRuleId = uuid.New()
var projectId = testProjectId

const (
	runnerRulesTestRunnerId       = "runner-1"
	runnerRulesTestRunnerSetFlag  = "--set=runner_id=runner-1"
	runnerRulesTestRuleAlias      = "rrl"
	runnerRulesTestListObjectType = "runner-rules"
	runnerRulesTestNotFound       = "not found"
	runnerRulesTestCreatedAt      = "CreatedAt"
)

func TestCreate_create_runner_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Project{Id: projectId},
	}, nil)

	cpc.EXPECT().CreateRunnerRuleInOrgWithResponse(gomock.Any(), orgId, cp.RunnerRuleCreateBody{
		RunnerId:  runnerRulesTestRunnerId,
		ProjectId: ref.Ref(projectId),
	}).Return(&cp.CreateRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.RunnerRule{
			OrgId:     orgId,
			Id:        runnerRuleId,
			RunnerId:  runnerRulesTestRunnerId,
			ProjectId: projectId,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, runnerRulesTestRuleAlias, runnerRulesTestRunnerSetFlag, "--set=project_id=" + projectId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
	"id": "%s",
	"runner_id": "runner-1",
	"project_id": "%s",
	"env_type_id": "",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, runnerRuleId, projectId), stdout)
	}
}

func TestCreate_create_runner_rule_with_nonexistent_project(t *testing.T) {
	projectId := "nonexistent-project"
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation to return NotFound
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: "project not found"},
	}, nil)

	cpc.EXPECT().CreateRunnerRuleInOrgWithResponse(gomock.Any(), orgId, cp.RunnerRuleCreateBody{
		RunnerId:  runnerRulesTestRunnerId,
		ProjectId: ref.Ref(projectId),
	}).Return(&cp.CreateRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.RunnerRule{
			OrgId:     orgId,
			Id:        runnerRuleId,
			RunnerId:  runnerRulesTestRunnerId,
			ProjectId: projectId,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, runnerRulesTestRuleAlias, runnerRulesTestRunnerSetFlag, "--set=project_id=" + projectId, noPromptFlag})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
	"id": "%s",
	"runner_id": "runner-1",
	"project_id": "nonexistent-project",
	"env_type_id": "",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, runnerRuleId), stdout)
	}
}

func TestCreate_create_runner_rule_project_validation_unexpected_error(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation to return an unexpected error
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
		Body:         []byte("internal server error"),
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testCreateCmd, runnerRulesTestRuleAlias, runnerRulesTestRunnerSetFlag, "--set=project_id=" + projectId})
	assert.EqualError(t, err, "unexpected status code 500 when validating project id: internal server error")
}

func TestDelete_runner_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteRunnerRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.DeleteRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, runnerRulesTestRuleAlias, ruleId.String()})
	assert.NoError(t, err)
}

func TestDelete_runner_rule_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteRunnerRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.DeleteRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: runnerRulesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, runnerRulesTestRuleAlias, ruleId.String()})
	assert.EqualError(t, err, fmt.Sprintf("runner rule '%s' not found in org '%s'", ruleId, orgId))
}

func TestGet_runner_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetRunnerRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.GetRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.RunnerRule{
			OrgId:    orgId,
			Id:       runnerRuleId,
			RunnerId: runnerRulesTestRunnerId,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, runnerRulesTestRuleAlias, ruleId.String()})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
    "id": "%s",
    "runner_id": "runner-1",
    "project_id": "",
    "env_type_id": "",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, runnerRuleId), stdout)
	}
}
func TestGet_runner_rule_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetRunnerRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.GetRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.RunnerRule{
			OrgId:    orgId,
			Id:       runnerRuleId,
			RunnerId: runnerRulesTestRunnerId,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, runnerRulesTestRuleAlias, ruleId.String()})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, "RunnerId")
		assert.Contains(t, stdout, runnerRulesTestRunnerId)
		assert.Contains(t, stdout, runnerRulesTestCreatedAt)
	}
}

func TestGet_runner_rule_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetRunnerRuleInOrgWithResponse(gomock.Any(), orgId, runnerRuleId).Return(&cp.GetRunnerRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: runnerRulesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, runnerRulesTestRuleAlias, runnerRuleId.String()})
	assert.EqualError(t, err, fmt.Sprintf("runner rule '%s' not found in org '%s'", runnerRuleId, orgId))
}

func TestList_runner_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{}).Return(&cp.ListRunnerRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RunnerRulePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{Page: ref.Ref("next-page")}).
		Return(&cp.ListRunnerRulesInOrgResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &cp.RunnerRulePage{Items: []cp.RunnerRuleSummary{
				{OrgId: orgId, Id: runnerRuleId, RunnerId: runnerRulesTestRunnerId, ProjectId: projectId, EnvTypeId: "env-type-1"},
			}},
		}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, runnerRulesTestListObjectType})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`[{
    "org_id": "%s",
    "id": "%s",
    "runner_id": "runner-1",
    "project_id": "%s",
    "env_type_id": "env-type-1",
    "created_at": "0001-01-01T00:00:00Z"
}]`, orgId, runnerRuleId, projectId), stdout)
	}
}

func TestList_runner_rule_with_filters(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{
		ByRunnerId: ref.Ref(runnerRulesTestRunnerId),
	}).Return(&cp.ListRunnerRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RunnerRulePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListRunnerRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListRunnerRulesInOrgParams{
		Page:       ref.Ref("next-page"),
		ByRunnerId: ref.Ref(runnerRulesTestRunnerId),
	}).
		Return(&cp.ListRunnerRulesInOrgResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &cp.RunnerRulePage{Items: []cp.RunnerRuleSummary{
				{OrgId: orgId, Id: runnerRuleId, RunnerId: runnerRulesTestRunnerId},
			}},
		}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, runnerRulesTestListObjectType, "--runner", runnerRulesTestRunnerId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`[{
    "org_id": "%s",
	"id": "%s",
    "runner_id": "runner-1",
    "project_id": "",
    "env_type_id": "",
    "created_at": "0001-01-01T00:00:00Z"
}]`, orgId, runnerRuleId), stdout)
	}
}
