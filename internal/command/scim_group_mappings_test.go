package command

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
)

const (
	scimGroupMappingCommandName = "scim-group-mapping"
	testScimGroupDisplayName    = "Platform Engineers"
)

func TestCreateScimGroupMapping(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	roleID := uuid.New()
	body := iam.ScimGroupMappingWriteBody{RoleId: roleID}
	iamc.EXPECT().UpsertScimGroupMappingWithResponse(gomock.Any(), orgID, testScimGroupDisplayName, body).Return(&iam.UpsertScimGroupMappingResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ScimGroupMapping{GroupDisplayName: testScimGroupDisplayName, RoleId: roleID},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testCreateCmd, scimGroupMappingCommandName, testScimGroupDisplayName, `--set=role_id=` + roleID.String()})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, testScimGroupDisplayName)
		assert.Contains(t, stdout, roleID.String())
	}
}

func TestGetScimGroupMappingMatchesCaseInsensitively(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	roleID := uuid.New()
	iamc.EXPECT().ListScimGroupMappingsWithResponse(gomock.Any(), orgID).Return(&iam.ListScimGroupMappingsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ScimGroupMappingPage{Items: []iam.ScimGroupMapping{{GroupDisplayName: testScimGroupDisplayName, RoleId: roleID}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, scimGroupMappingCommandName, "platform engineers"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, roleID.String())
	}
}

func TestListScimGroupMappings(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	iamc.EXPECT().ListScimGroupMappingsWithResponse(gomock.Any(), orgID).Return(&iam.ListScimGroupMappingsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.ScimGroupMappingPage{Items: []iam.ScimGroupMapping{{GroupDisplayName: "Operators", RoleId: uuid.New()}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, "scim-group-mappings"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Operators")
	}
}

func TestDeleteScimGroupMappingNotFound(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	iamc.EXPECT().DeleteScimGroupMappingWithResponse(gomock.Any(), orgID, "Missing").Return(&iam.DeleteScimGroupMappingResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &iam.N404NotFound{Message: envTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, testDeleteCmd, scimGroupMappingCommandName, "Missing"})
	assert.EqualError(t, err, `SCIM group mapping for "Missing" not found in organization '`+orgID+`'`)
}
