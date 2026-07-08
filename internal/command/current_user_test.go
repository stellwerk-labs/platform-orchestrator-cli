package command

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
	mockiam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam/mocks"
)

const currentUserTestObjectType = "current-user"

func TestGet_current_user(t *testing.T) {
	_, _, _, ctx, fin := setupTestContext(t)
	defer fin()

	iamc := MustIamClient(ctx).(*mockiam.MockClientWithResponsesInterface)
	iamc.EXPECT().GetCurrentUserWithResponse(gomock.Any()).Return(&iam.GetCurrentUserResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &iam.CurrentUser{
			Id:                      uuid.Nil,
			LoginProviders:          []string{},
			OrganizationMemberships: []iam.CurrentUserOrgMembership{},
		},
	}, nil)

	out, _, err := executeAndResetCommand(ctx, RootCmd, []string{outFlag, jsonOutput, testGetCmd, currentUserTestObjectType})
	require.NoError(t, err)
	assert.JSONEq(t, `{
	"created_at":"0001-01-01T00:00:00Z",
	"display_name":"",
	"id":"00000000-0000-0000-0000-000000000000",
	"login_providers": [],
	"organization_memberships":[],
	"dismissed_prompts": null
}`, out)
}

func TestGet_current_user_default_printer(t *testing.T) {
	_, _, _, ctx, fin := setupTestContext(t)
	defer fin()

	iamc := MustIamClient(ctx).(*mockiam.MockClientWithResponsesInterface)
	iamc.EXPECT().GetCurrentUserWithResponse(gomock.Any()).Return(&iam.GetCurrentUserResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &iam.CurrentUser{
			Id:                      uuid.Nil,
			LoginProviders:          []string{},
			OrganizationMemberships: []iam.CurrentUserOrgMembership{},
		},
	}, nil)

	out, _, err := executeAndResetCommand(ctx, RootCmd, []string{testGetCmd, currentUserTestObjectType})

	require.NoError(t, err)

	if assert.NoError(t, err) {
		assert.Contains(t, out, "Id")
	}
}
