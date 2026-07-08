package command

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	testUpdatedEnvDisplayName = "Updated Environment"
	envsTestAlias             = "env"
	envsTestThingName         = "thing"
	envsTestStatusSucceeded   = "succeeded"
	envsTestStatusMessage     = "it was successful!"
	envsTestNotFound          = "not found"
)

func TestCreate_env(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, cp.EnvironmentCreateBody{
		Id:          testEnvId,
		DisplayName: ref.RefStringEmptyNil(testEnvName),
		EnvTypeId:   testEnvTypeId,
	}).Return(&cp.CreateEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId, ProjectId: testProjectId, Uuid: uuid.MustParse("00000000-0000-0000-0000-000000000000"), RunnerId: ref.Ref("test-runner")},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, envsTestAlias, testProjectId, testEnvId, `--set-json={"display_name": "` + testEnvName + `", "env_type_id": "` + testEnvTypeId + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"id": "`+testEnvId+`",
	"display_name": "`+testEnvName+`",
	"env_type_id": "`+testEnvTypeId+`",
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"project_id": "`+testProjectId+`",
	"runner_id": "test-runner",
	"status": ""
}`, stdout)
	}
}

func TestCreate_env_error(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, cp.EnvironmentCreateBody{
		Id:          testEnvId,
		DisplayName: ref.RefStringEmptyNil(testEnvName),
		EnvTypeId:   testEnvTypeId,
	}).Return(&cp.CreateEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusConflict},
		JSON409:      &cp.Error{Message: "uniq error message", Details: &map[string]interface{}{"source": "get_environment_type"}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, envsTestAlias, testProjectId, testEnvId, `--set-json={"display_name": "` + testEnvName + `", "env_type_id": "` + testEnvTypeId + `"}`})
	require.ErrorContains(t, err, "conflict: uniq error message.")
	assert.Empty(t, stdout)
}

func TestCreate_env_missing_env_type_id(t *testing.T) {
	orgId, _, _, ctx, fin := setupTestContext(t)
	defer fin()

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, envsTestAlias, testProjectId, testEnvId})
	assert.EqualError(t, err, "env_type_id is required. Use --set env_type_id=<env_type_id>")
}

func TestDelete_env_no_wait(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	depId := uuid.New()
	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: depId}}},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, depId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.Deployment{Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{envsTestThingName: {}}}},
	}, nil)

	// With --no-prompt, no listing of module rules should occur
	cpc.EXPECT().DeleteEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.DeleteEnvironmentParams{Force: ref.Ref(true), DeleteRules: ref.Ref(false)}).Return(&cp.DeleteEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
		JSON202:      &cp.Environment{Status: cp.EnvironmentStatusDeleting},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId, noWaitFlag, noPromptFlag, "--force"})
	assert.NoError(t, err)
}

func TestDelete_env_never_deployed_no_wait(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	// With --no-prompt, no listing of module rules should occur
	cpc.EXPECT().DeleteEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.DeleteEnvironmentParams{Force: ref.Ref(true), DeleteRules: ref.Ref(false)}).Return(&cp.DeleteEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
		JSON202:      &cp.Environment{Status: cp.EnvironmentStatusDeleting},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId, noWaitFlag, noPromptFlag, "--force"})
	assert.NoError(t, err)
}

func TestDelete_env_wait(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	fakeDepId := uuid.New()
	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: fakeDepId}}},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, fakeDepId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.Deployment{Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{envsTestThingName: {}}}},
	}, nil)

	// With --no-prompt, no listing of module rules should occur
	cpc.EXPECT().DeleteEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.DeleteEnvironmentParams{Force: ref.Ref(false), DeleteRules: ref.Ref(false)}).Return(&cp.DeleteEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
		JSON202:      &cp.Environment{Status: cp.EnvironmentStatusDeleting, StatusMessage: ref.Ref("Attempting to destroy environment")},
	}, nil)
	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{Status: cp.EnvironmentStatusDeleting, StatusMessage: ref.Ref(fmt.Sprintf("Waiting for destroy deployment %s to finish", fakeDepId))},
	}, nil)
	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, fakeDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				Id:            fakeDepId,
				CompletedAt:   &time.Time{},
				Mode:          testDeployCmd,
				Status:        envsTestStatusSucceeded,
				StatusMessage: envsTestStatusMessage,
				Manifest:      dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			},
		}, nil)
	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId, noPromptFlag})
	assert.NoError(t, err)
}

func TestDelete_env_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: envsTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId})
	assert.EqualError(t, err, fmt.Sprintf("environment '%s' not found in project '%s' in org '%s'", testEnvId, testProjectId, orgId))
}

func TestDelete_env_deleting(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{Status: cp.EnvironmentStatusDeleting},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId})
	assert.EqualError(t, err, fmt.Sprintf("environment '%s' is already deleting", testEnvId))
}

func TestGet_env(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Environment{
			Id:          testEnvId,
			DisplayName: testEnvName,
			EnvTypeId:   testEnvTypeId,
			ProjectId:   testProjectId,
			RunnerId:    ref.Ref("test-runner"),
		},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, envsTestAlias, testProjectId, testEnvId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"created_at": "0001-01-01T00:00:00Z",
	"id": "`+testEnvId+`",
	"display_name": "`+testEnvName+`",
	"env_type_id": "`+testEnvTypeId+`",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"project_id": "`+testProjectId+`",
	"runner_id": "test-runner",
	"updated_at": "0001-01-01T00:00:00Z",
	"status": ""
}`, stdout)
	}
}

func TestGet_env_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Environment{
			Id:          testEnvId,
			DisplayName: testEnvName,
			EnvTypeId:   testEnvTypeId,
			ProjectId:   testProjectId,
			RunnerId:    ref.Ref("test-runner"),
		},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, envsTestAlias, testProjectId, testEnvId})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
	}
}

func TestGet_env_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: envsTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, envsTestAlias, testProjectId, testEnvId})
	assert.EqualError(t, err, fmt.Sprintf("environment '%s' not found in project '%s' in org '%s'", testEnvId, testProjectId, orgId))
}

func TestList_env(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListEnvironmentsWithResponse(gomock.Any(), orgId, testProjectId, &cp.ListEnvironmentsParams{}).Return(&cp.ListEnvironmentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.EnvironmentPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListEnvironmentsWithResponse(gomock.Any(), orgId, testProjectId, &cp.ListEnvironmentsParams{Page: ref.Ref("next-page")}).Return(&cp.ListEnvironmentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.EnvironmentPage{Items: []cp.Environment{{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId, ProjectId: testProjectId, RunnerId: ref.Ref("test-runner"), Uuid: uuid.MustParse("00000000-0000-0000-0000-000000000000"), Status: cp.EnvironmentStatusActive}}},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "envs", testProjectId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
	"created_at": "0001-01-01T00:00:00Z",
	"id": "`+testEnvId+`",
	"display_name": "`+testEnvName+`",
	"env_type_id": "`+testEnvTypeId+`",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"project_id": "`+testProjectId+`",
	"runner_id": "test-runner",
	"updated_at": "0001-01-01T00:00:00Z",
	"status": "active"
}]`, stdout)
	}
}

func TestUpdate_environment(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, cp.EnvironmentUpdateBody{
		DisplayName: testUpdatedEnvDisplayName,
	}).Return(&cp.UpdateEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Environment{
			Id:          testEnvId,
			DisplayName: testUpdatedEnvDisplayName,
			EnvTypeId:   testEnvTypeId,
			ProjectId:   testProjectId,
			RunnerId:    ref.Ref("test-runner"),
			Uuid:        uuid.MustParse("00000000-0000-0000-0000-000000000000"),
			Status:      cp.EnvironmentStatusActive,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, envsTestAlias, testProjectId, testEnvId, `--set-json={"display_name": "` + testUpdatedEnvDisplayName + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"created_at": "0001-01-01T00:00:00Z",
	"id": "`+testEnvId+`",
	"display_name": "`+testUpdatedEnvDisplayName+`",
	"env_type_id": "`+testEnvTypeId+`",
	"uuid": "00000000-0000-0000-0000-000000000000",
	"project_id": "`+testProjectId+`",
	"runner_id": "test-runner",
	"updated_at": "0001-01-01T00:00:00Z",
	"status": "active"
}`, stdout)
	}
}

func TestUpdate_environment_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, cp.EnvironmentUpdateBody{
		DisplayName: testUpdatedEnvDisplayName,
	}).Return(&cp.UpdateEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: envsTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, envsTestAlias, testProjectId, testEnvId, `--set-json={"display_name": "` + testUpdatedEnvDisplayName + `"}`})
	assert.EqualError(t, err, fmt.Sprintf("environment '%s' not found in project '%s' in org '%s'", testEnvId, testProjectId, orgId))
}

func TestDelete_env_with_delete_rules_flag(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	// When --delete-rules is set, no listing should occur
	cpc.EXPECT().DeleteEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.DeleteEnvironmentParams{
		Force:       ref.Ref(false),
		DeleteRules: ref.Ref(true),
	}).Return(&cp.DeleteEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
		JSON202:      &cp.Environment{Status: cp.EnvironmentStatusDeleting},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId, noWaitFlag, noPromptFlag, "--delete-rules"})
	assert.NoError(t, err)
}

func TestDelete_env_with_rules_prompt_error(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	moduleRuleId := uuid.New()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	// Expect listing of module rules
	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{
		ByProjectId: ref.Ref(testProjectId),
		ByEnvId:     ref.Ref(testEnvId),
	}).Return(&cp.ListModuleRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.RulePage{Items: []cp.RuleSummary{
			{Id: moduleRuleId, ModuleId: "s3-module", ResourceType: "s3"},
		}},
	}, nil)

	// Should fail when trying to prompt without --no-prompt (stdin is not interactive in tests)
	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId})
	assert.Error(t, err)
}

func TestDelete_env_with_no_prompt_no_rules(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	// With --no-prompt, no listing of module rules should occur, and should skip environment deletion prompt
	cpc.EXPECT().DeleteEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.DeleteEnvironmentParams{
		Force:       ref.Ref(false),
		DeleteRules: ref.Ref(false),
	}).Return(&cp.DeleteEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusAccepted},
		JSON202:      &cp.Environment{Status: cp.EnvironmentStatusDeleting},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId, noWaitFlag, noPromptFlag})
	assert.NoError(t, err)
}

func TestDelete_env_with_no_prompt_with_rules_fails(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{},
	}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	// With --no-prompt, no listing of module rules occurs. The deletion will be attempted
	// and should fail with a conflict error because rules exist but DeleteRules is false
	cpc.EXPECT().DeleteEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.DeleteEnvironmentParams{
		Force:       ref.Ref(false),
		DeleteRules: ref.Ref(false),
	}).Return(&cp.DeleteEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusConflict},
		JSON409:      &cp.Error{Message: "environment has associated module rules"},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, envsTestAlias, testProjectId, testEnvId, noPromptFlag})
	assert.EqualError(t, err, "environment '"+testEnvId+"' cannot be deleted: environment has associated module rules")
}
