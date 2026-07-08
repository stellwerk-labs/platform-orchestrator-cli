package command

import (
	"net/http"
	"testing"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
)

const logsTestCmd = "logs"

func TestLogs(t *testing.T) {
	var (
		deploymentId = "00374e43-96b5-416f-977b-514b9e267f89"
		secretKey    = "AGE-SECRET-KEY-1SGKT64UHNGURNZUQSAJ9J0C65R2F2PXDCZQVVGRUPN98L33G2HFSUC24EJ"
		logsText     = `Deployment succeeded after 1s: it was successful!`
	)

	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	dpc.EXPECT().GetDeploymentLogsWithResponse(gomock.Any(), orgId, uuid.MustParse(deploymentId), &dp.GetDeploymentLogsParams{DecryptKey: &secretKey}).
		Return(&dp.GetDeploymentLogsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			Body:         []byte(logsText),
		}, nil).
		Times(1)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, logsTestCmd, deploymentId, keyFlag, secretKey})
	require.NoError(t, err)
	assert.Equal(t, logsText, stdout)
}

func TestLogs_without_decryption(t *testing.T) {
	var (
		deploymentId = "00374e43-96b5-416f-977b-514b9e267f89"
		logsText     = `<ENCRYPTED_LOGS>`
	)

	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	dpc.EXPECT().GetDeploymentLogsWithResponse(gomock.Any(), orgId, uuid.MustParse(deploymentId), &dp.GetDeploymentLogsParams{}).
		Return(&dp.GetDeploymentLogsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			Body:         []byte(logsText),
		}, nil).
		Times(1)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, logsTestCmd, deploymentId})
	require.NoError(t, err)
	assert.Equal(t, logsText, stdout)
}

func TestLogs_error400(t *testing.T) {
	var (
		deploymentId = "00374e43-96b5-416f-977b-514b9e267f89"
		secretKey    = "AGE-SECRET-KEY-1SGKT64UHNGURNZUQSAJ9J0C65R2F2PXDCZQVVGRUPN98L33G2HFSUC24EJ"
	)

	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	dpc.EXPECT().GetDeploymentLogsWithResponse(gomock.Any(), orgId, uuid.MustParse(deploymentId), &dp.GetDeploymentLogsParams{DecryptKey: &secretKey}).
		Return(&dp.GetDeploymentLogsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
			JSON400: &dp.N400BadRequest{
				Message: "Secret key not valid",
			},
		}, nil).
		Times(1)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, logsTestCmd, deploymentId, keyFlag, secretKey})
	require.Errorf(t, err, "unexpected status code 400 when getting deployment logs: Secret key not valid")
}

func TestLogs_error404(t *testing.T) {
	var (
		deploymentId = "00374e43-96b5-416f-977b-514b9e267f89"
		secretKey    = "AGE-SECRET-KEY-1SGKT64UHNGURNZUQSAJ9J0C65R2F2PXDCZQVVGRUPN98L33G2HFSUC24EJ"
	)

	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	dpc.EXPECT().GetDeploymentLogsWithResponse(gomock.Any(), orgId, uuid.MustParse(deploymentId), &dp.GetDeploymentLogsParams{DecryptKey: &secretKey}).
		Return(&dp.GetDeploymentLogsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
			JSON400: &dp.N404NotFound{
				Message: "Deployment not found",
			},
		}, nil).
		Times(1)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, logsTestCmd, deploymentId, keyFlag, secretKey})
	require.EqualError(t, err, "unexpected status code 404 when getting deployment logs: Deployment not found")
}
