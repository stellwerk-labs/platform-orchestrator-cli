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
	manifestTestDeploymentId = "01234567-89ab-cdef-0123-456789abcdef"
	manifestTestObjectType   = "manifest"
	manifestTestSampleName   = "sample"
)

func TestGetManifest_by_uuid(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, uuid.MustParse(manifestTestDeploymentId)).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.Deployment{
			Manifest: dp.DeploymentManifest{
				Workloads: map[string]dp.DeploymentManifestWorkload{
					manifestTestSampleName: {},
				},
			},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, manifestTestObjectType, manifestTestDeploymentId})

	if assert.NoError(t, err) {
		assert.JSONEq(t, `{"workloads": {"sample": {}}}`, stdout)
	}
}

func TestGetManifest_default_printer(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, uuid.MustParse(manifestTestDeploymentId)).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.Deployment{
			Manifest: dp.DeploymentManifest{
				Workloads: map[string]dp.DeploymentManifestWorkload{
					manifestTestSampleName: {},
				},
			},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, manifestTestObjectType, manifestTestDeploymentId})

	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Workloads")
		assert.Contains(t, stdout, manifestTestSampleName)
	}
}

func TestGetManifest_by_env(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.RefStringEmptyNil(testMpId),
		EnvId:           ref.RefStringEmptyNil("development"),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.DeploymentPage{
			Items: []dp.DeploymentSummary{{Id: uuid.MustParse(manifestTestDeploymentId)}},
		},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, uuid.MustParse(manifestTestDeploymentId)).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.Deployment{
			Manifest: dp.DeploymentManifest{
				Workloads: map[string]dp.DeploymentManifestWorkload{
					manifestTestSampleName: {},
				},
			},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, manifestTestObjectType, testMpId, "development"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{"workloads": {"sample": {}}}`, stdout)
	}
}
