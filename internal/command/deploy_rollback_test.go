package command

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	rollbackTestStatusSucceeded = "succeeded"
	rollbackTestStatusMessage   = "it was successful!"
)

func TestNominal_deploy_rollback_plan(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	targetDepId := uuid.New()
	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, targetDepId).
		Return(&dp.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				ProjectId: testProjectId,
				EnvId:     testEnvId,
				Manifest:  dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			},
		}, nil)

	lastDepId := uuid.New()
	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: lastDepId}}},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, lastDepId).
		Return(&dp.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &dp.Deployment{Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}}},
		}, nil)

	newDepId := uuid.New()
	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId:              testProjectId,
			EnvId:                  testEnvId,
			Mode:                   dp.DeploymentCreateBodyModeRollback,
			PlanOnly:               ref.Ref(true),
			RollbackToDeploymentId: ref.Ref(targetDepId),
			RunnerLogLevel:         ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
			JSON201:      &dp.Deployment{Id: newDepId},
		}, nil
	})

	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, newDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				Id:            newDepId,
				CompletedAt:   &time.Time{},
				Status:        rollbackTestStatusSucceeded,
				StatusMessage: rollbackTestStatusMessage,
			},
		}, nil)

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`{}`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, "rollback", testProjectId, testEnvId, targetDepId.String(), planOnlyFlag, noPromptFlag, skipLogsFlag})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Looking up rollback target deployment '%[3]s'...
Checking for last deployment...
Found previous stateful deployment %[1]s.
Generating diff...
No manifest changes detected - this will be a re-deployment.
Creating rollback deployment (plan only)...
Deployment %[2]s created.
Waiting for deployment %[2]s to complete (0s)...
Deployment %[2]s succeeded after 0s: it was successful!
Outputs ignored due to unset --result flag.
`, lastDepId, newDepId, targetDepId), stderr)
}
