package command

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	serviceUserCommandName = "service-user"
	serviceUserTestToken   = "SU-one-time-token" // #nosec G101 -- fake one-time credential fixture
)

func TestCreateServiceUserPrintsOneTimeToken(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	serviceUserID := uuid.New()
	body := iam.ServiceUserCreateBody{DisplayName: "Entra provisioning", ExpiryInDays: 30}
	iamc.EXPECT().CreateServiceUserWithResponse(gomock.Any(), orgID, body).Return(&iam.CreateServiceUserResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &iam.ServiceUserWithToken{
			Id: serviceUserID, DisplayName: body.DisplayName, Token: serviceUserTestToken, CurrentTokenExpiresAt: time.Unix(1, 0).UTC(),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testCreateCmd, serviceUserCommandName, `--set-json={"display_name":"Entra provisioning","expiry_in_days":30}`})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, serviceUserTestToken)
		assert.Contains(t, stdout, serviceUserID.String())
	}
}

func TestGetServiceUserFiltersList(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	serviceUserID := uuid.New()
	iamc.EXPECT().ListServiceUsersWithResponse(gomock.Any(), orgID, &iam.ListServiceUsersParams{}).Return(&iam.ListServiceUsersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ServiceUserPage{Items: []iam.ServiceUserSummary{{Id: uuid.New()}}, NextPageToken: ref.Ref("next")},
	}, nil)
	iamc.EXPECT().ListServiceUsersWithResponse(gomock.Any(), orgID, &iam.ListServiceUsersParams{Page: ref.Ref("next")}).Return(&iam.ListServiceUsersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ServiceUserPage{Items: []iam.ServiceUserSummary{{Id: serviceUserID, DisplayName: "SCIM"}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, serviceUserCommandName, serviceUserID.String()})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, serviceUserID.String())
		assert.Contains(t, stdout, "SCIM")
	}
}

func TestListServiceUsersCollectsPages(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	firstID := uuid.New()
	secondID := uuid.New()
	iamc.EXPECT().ListServiceUsersWithResponse(gomock.Any(), orgID, &iam.ListServiceUsersParams{}).Return(&iam.ListServiceUsersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ServiceUserPage{Items: []iam.ServiceUserSummary{{Id: firstID}}, NextPageToken: ref.Ref("next")},
	}, nil)
	iamc.EXPECT().ListServiceUsersWithResponse(gomock.Any(), orgID, &iam.ListServiceUsersParams{Page: ref.Ref("next")}).Return(&iam.ListServiceUsersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ServiceUserPage{Items: []iam.ServiceUserSummary{{Id: secondID}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, "service-users"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, firstID.String())
		assert.Contains(t, stdout, secondID.String())
	}
}

func TestUpdateServiceUserReplacesRoles(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	serviceUserID := uuid.New()
	roleID := uuid.New()
	body := iam.ReplaceServiceUserRolesBody{Roles: []iam.ServiceUserRole{{Id: roleID}}}
	iamc.EXPECT().ReplaceServiceUserRolesWithResponse(gomock.Any(), orgID, serviceUserID, body).Return(&iam.ReplaceServiceUserRolesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ServiceUserSummary{Id: serviceUserID, Roles: body.Roles},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testUpdateCmd, serviceUserCommandName, serviceUserID.String(), `--set-json={"roles":[{"id":"` + roleID.String() + `"}]}`})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, roleID.String())
	}
}

func TestRegenerateServiceUser(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	serviceUserID := uuid.New()
	iamc.EXPECT().RegenerateServiceUserWithResponse(gomock.Any(), orgID, serviceUserID, iam.ServiceUserRegenerateBody{ExpiryInDays: 90}).Return(&iam.RegenerateServiceUserResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ServiceUserWithToken{Id: serviceUserID, Token: "SU-rotated"},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, "regenerate", serviceUserCommandName, serviceUserID.String(), "--expiry-in-days=90"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "SU-rotated")
	}
}

func TestRegenerateServiceUserValidatesExpiry(t *testing.T) {
	orgID, _, _, _, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, "regenerate", serviceUserCommandName, uuid.NewString(), "--expiry-in-days=0"})
	assert.EqualError(t, err, "--expiry-in-days must be between 1 and 3660")
}

func TestDeleteServiceUser(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	serviceUserID := uuid.New()
	iamc.EXPECT().DeleteServiceUserWithResponse(gomock.Any(), orgID, serviceUserID).Return(&iam.DeleteServiceUserResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, testDeleteCmd, serviceUserCommandName, serviceUserID.String()})
	assert.NoError(t, err)
}
