package command

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	deploymentsTestAliasSingular = "dep"
	deploymentsTestAliasPlural   = "deps"
)

func TestGet_dep(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	depId := uuid.New()

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, depId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.Deployment{
			Id:        depId,
			ProjectId: testProjectId,
			EnvId:     testEnvId,
			OrgId:     orgId,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, deploymentsTestAliasSingular, depId.String()})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
			"id": "`+depId.String()+`",
			"project_id": "my-project",
			"created_at": "0001-01-01T00:00:00Z",
			"created_by": "00000000-0000-0000-0000-000000000000",
			"env_id": "my-env",
			"manifest": {
				"workloads": null
			},
			"mode": "",
			"plan_only": false,
			"metrics": {
				"num_resource_nodes": 0,
				"num_workloads": 0
			},
			"org_id": "`+orgId+`",
			"runner_id": "",
			"status": "",
			"status_message": ""
		}`, stdout)
	}
}

func TestGet_default_printer(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	depId := uuid.New()

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, depId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.Deployment{
			Id:        depId,
			ProjectId: testProjectId,
			EnvId:     testEnvId,
			OrgId:     orgId,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, deploymentsTestAliasSingular, depId.String()})

	if assert.NoError(t, err) {
		// Columns returned
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, "ProjectId")
		assert.Contains(t, stdout, "EnvId")
		assert.Contains(t, stdout, "Status")
		assert.Contains(t, stdout, "Mode")
		assert.Contains(t, stdout, "CreatedAt")
		assert.Contains(t, stdout, "CompletedAt")
		assert.Contains(t, stdout, "Manifest")
		assert.Contains(t, stdout, "RunnerId")

		// Values returned
		assert.Contains(t, stdout, testProjectId)
		assert.Contains(t, stdout, testEnvId)
	}
}

func TestGet_dep_not_found(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	depId := uuid.New()

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, depId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &dp.Error{Message: "not found"},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, deploymentsTestAliasSingular, depId.String()})
	assert.EqualError(t, err, "deployment '"+depId.String()+"' not found in org '"+orgId+"'")
}

func TestList_dep(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	depId := uuid.New()

	dpc.EXPECT().ListDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListDeploymentsParams{}).Return(&dp.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	dpc.EXPECT().ListDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListDeploymentsParams{Page: ref.Ref("next-page")}).Return(&dp.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: depId, ProjectId: testProjectId, EnvId: testEnvId, OrgId: orgId}}},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, deploymentsTestAliasPlural})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
			"created_at": "0001-01-01T00:00:00Z",
			"created_by": "00000000-0000-0000-0000-000000000000",
			"id": "`+depId.String()+`",
			"project_id": "my-project",
			"env_id": "my-env",
			"mode": "",
			"plan_only": false,
			"metrics": {
				"num_resource_nodes": 0,
				"num_workloads": 0
			},
			"org_id": "`+orgId+`",
			"status": "",
			"status_message": ""
		}]`, stdout)
	}
}

func TestList_dep_project(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	depId := uuid.New()

	dpc.EXPECT().ListDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListDeploymentsParams{
		ProjectId: ref.RefStringEmptyNil(testProjectId),
	}).Return(&dp.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	dpc.EXPECT().ListDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListDeploymentsParams{
		ProjectId: ref.RefStringEmptyNil(testProjectId),
		Page:      ref.Ref("next-page"),
	}).Return(&dp.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: depId, ProjectId: testProjectId, EnvId: testEnvId, OrgId: orgId}}},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, deploymentsTestAliasPlural, testProjectId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
			"created_at": "0001-01-01T00:00:00Z",
			"created_by": "00000000-0000-0000-0000-000000000000",
			"id": "`+depId.String()+`",
			"project_id": "my-project",
			"env_id": "my-env",
			"mode": "",
			"plan_only": false,
			"metrics": {
				"num_resource_nodes": 0,
				"num_workloads": 0
			},
			"org_id": "`+orgId+`",
			"status": "",
			"status_message": ""
		}]`, stdout)
	}
}

func TestList_dep_project_env(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	depId := uuid.New()

	dpc.EXPECT().ListDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListDeploymentsParams{
		ProjectId: ref.RefStringEmptyNil(testProjectId),
		EnvId:     ref.RefStringEmptyNil(testEnvId),
	}).Return(&dp.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	dpc.EXPECT().ListDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListDeploymentsParams{
		ProjectId: ref.RefStringEmptyNil(testProjectId),
		EnvId:     ref.RefStringEmptyNil(testEnvId),
		Page:      ref.Ref("next-page"),
	}).Return(&dp.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: depId, ProjectId: testProjectId, EnvId: testEnvId, OrgId: orgId}}},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, deploymentsTestAliasPlural, testProjectId, testEnvId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
			"created_at": "0001-01-01T00:00:00Z",
			"created_by": "00000000-0000-0000-0000-000000000000",
			"id": "`+depId.String()+`",
			"project_id": "my-project",
			"env_id": "my-env",
			"mode": "",
			"plan_only": false,
			"metrics": {
				"num_resource_nodes": 0,
				"num_workloads": 0
			},
			"org_id": "`+orgId+`",
			"status": "",
			"status_message": ""
		}]`, stdout)
	}
}
