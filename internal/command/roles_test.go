package command

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	iam "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const roleCommandName = "role"

func TestCreateRole(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	roleID := uuid.New()
	body := iam.RoleWriteBody{DisplayName: "Module Maintainer", Permissions: []string{"module_read", "module_write"}}
	iamc.EXPECT().CreateRoleWithResponse(gomock.Any(), orgID, body).Return(&iam.CreateRoleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201:      &iam.Role{Id: roleID, DisplayName: body.DisplayName, Permissions: body.Permissions},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testCreateCmd, roleCommandName, `--set-json={"display_name":"Module Maintainer","permissions":["module_read","module_write"]}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{"id":"`+roleID.String()+`","display_name":"Module Maintainer","permissions":["module_read","module_write"],"created_at":"0001-01-01T00:00:00Z","created_by":"00000000-0000-0000-0000-000000000000","is_system":false}`, stdout)
	}
}

func TestListRolesCollectsPages(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	roleID := uuid.New()
	iamc.EXPECT().ListRolesWithResponse(gomock.Any(), orgID, &iam.ListRolesParams{}).Return(&iam.ListRolesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.RolePage{NextPageToken: ref.Ref("next")},
	}, nil)
	iamc.EXPECT().ListRolesWithResponse(gomock.Any(), orgID, &iam.ListRolesParams{Page: ref.Ref("next")}).Return(&iam.ListRolesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &iam.RolePage{Items: []iam.Role{{Id: roleID, DisplayName: "Viewer", IsSystem: true}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, "roles"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, roleID.String())
		assert.Contains(t, stdout, "Viewer")
	}
}

func TestListPermissions(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	iamc.EXPECT().ListPermissionsWithResponse(gomock.Any(), orgID).Return(&iam.ListPermissionsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &iam.PermissionDefinitionPage{Items: []iam.PermissionDefinition{{
			Id: "module_write", DisplayName: "Manage modules", Category: "Modules", Level: iam.Manage,
		}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, "permissions"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "module_write")
		assert.Contains(t, stdout, "Manage modules")
	}
}

func TestGetRoleRejectsInvalidUUID(t *testing.T) {
	orgID, _, _, _, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, testGetCmd, roleCommandName, "not-a-uuid"})
	assert.ErrorContains(t, err, "role ID must be a UUID")
}

func TestUpdateRoleReportsConflict(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	roleID := uuid.New()
	body := iam.RoleWriteBody{DisplayName: "Assigned", Permissions: []string{"module_read"}}
	iamc.EXPECT().UpdateRoleWithResponse(gomock.Any(), orgID, roleID, body).Return(&iam.UpdateRoleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusConflict},
		JSON409:      &iam.N409Conflict{Message: "immutable system role"},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, testUpdateCmd, roleCommandName, roleID.String(), `--set-json={"display_name":"Assigned","permissions":["module_read"]}`})
	assert.EqualError(t, err, "role cannot be updated: immutable system role")
}

func TestDeleteRoleReportsAssignedConflict(t *testing.T) {
	orgID, _, _, iamc, ctx, fin := setupTestContextWithIam(t)
	defer fin()
	roleID := uuid.New()
	iamc.EXPECT().DeleteRoleWithResponse(gomock.Any(), orgID, roleID).Return(&iam.DeleteRoleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusConflict},
		JSON409:      &iam.N409Conflict{Message: "role is assigned"},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, testDeleteCmd, roleCommandName, roleID.String()})
	assert.EqualError(t, err, "role cannot be deleted: role is assigned")
}
