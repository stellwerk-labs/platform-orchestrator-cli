package command

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
)

const (
	tfTestDeploymentId = "01234567-89ab-cdef-0123-456789abcdef"
	tfTestNotFound     = "not found"
)

func TestGetTf_nominal(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	dpc.EXPECT().GetDeploymentTfWithResponse(gomock.Any(), orgId, uuid.MustParse(tfTestDeploymentId)).Return(&dp.GetDeploymentTfResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		Body:         []byte("source\ncode\nhere"),
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, "tf", tfTestDeploymentId})
	if assert.NoError(t, err) {
		assert.Equal(t, "source\ncode\nhere", stdout)
	}
}

func TestGetTf_not_found(t *testing.T) {
	orgId, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()

	dpc.EXPECT().GetDeploymentTfWithResponse(gomock.Any(), orgId, uuid.MustParse(tfTestDeploymentId)).Return(&dp.GetDeploymentTfResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &dp.Error{Message: tfTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, "tf", tfTestDeploymentId})
	assert.EqualError(t, err, "deployment '01234567-89ab-cdef-0123-456789abcdef' not found in org '"+orgId+"'")
}
