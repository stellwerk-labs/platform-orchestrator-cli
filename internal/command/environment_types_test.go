package command

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	testUpdatedEnvTypeDisplayName = "Updated Environment Type"
	envTypesTestNotFound          = "not found"
)

func TestCreate_et(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateEnvironmentTypeWithResponse(gomock.Any(), orgId, cp.EnvironmentTypeCreateBody{
		Id:          testEnvTypeId,
		DisplayName: ref.RefStringEmptyNil(testEnvTypeName),
	}).Return(&cp.CreateEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201:      &cp.EnvironmentType{Id: testEnvTypeId, DisplayName: testEnvTypeName, Uuid: uuid.MustParse("00000000-0000-0000-0000-000000000000")},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "et", testEnvTypeId, `--set-json={"display_name": "` + testEnvTypeName + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"id": "`+testEnvTypeId+`",
	"display_name": "`+testEnvTypeName+`",
	"created_at": "0001-01-01T00:00:00Z",
	"uuid":"00000000-0000-0000-0000-000000000000"
}`, stdout)
	}
}

func TestDelete_et(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId).Return(&cp.DeleteEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "et", testEnvTypeId})
	assert.NoError(t, err)
}

func TestDelete_et_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId).Return(&cp.DeleteEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: envTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "et", testEnvTypeId})
	assert.EqualError(t, err, fmt.Sprintf(`environment type "my-et" not found in org %q.`, orgId))
}

func TestGet_et(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId).Return(&cp.GetEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.EnvironmentType{
			Id:          testEnvTypeId,
			DisplayName: testEnvTypeName,
			Uuid:        uuid.MustParse("00000000-0000-0000-0000-000000000000"),
		},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "et", testEnvTypeId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"created_at": "0001-01-01T00:00:00Z",
	"id": "my-et",
	"display_name": "My Environment Type",
	"uuid":"00000000-0000-0000-0000-000000000000"
}`, stdout)
	}
}

func TestGet_et_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId).Return(&cp.GetEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.EnvironmentType{
			Id:          testEnvTypeId,
			DisplayName: testEnvTypeName,
			Uuid:        uuid.MustParse("00000000-0000-0000-0000-000000000000"),
		},
	}, nil)
	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, "et", testEnvTypeId})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, testEnvTypeName)

	}
}
func TestGet_et_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId).Return(&cp.GetEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: envTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "et", testEnvTypeId})
	assert.EqualError(t, err, fmt.Sprintf(`environment type "my-et" not found in org %q.`, orgId))
}

func TestList_et(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListEnvironmentTypesWithResponse(gomock.Any(), orgId, &cp.ListEnvironmentTypesParams{}).Return(&cp.ListEnvironmentTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.EnvironmentTypePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListEnvironmentTypesWithResponse(gomock.Any(), orgId, &cp.ListEnvironmentTypesParams{Page: ref.Ref("next-page")}).Return(&cp.ListEnvironmentTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.EnvironmentTypePage{Items: []cp.EnvironmentType{{Id: testEnvTypeId, DisplayName: testEnvTypeName, Uuid: uuid.MustParse("00000000-0000-0000-0000-000000000000")}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "ets"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
	"created_at": "0001-01-01T00:00:00Z",
	"id": "my-et",
	"display_name": "My Environment Type",
	"uuid": "00000000-0000-0000-0000-000000000000"
}]`, stdout)
	}
}

func TestUpdate_environment_type(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId, cp.EnvironmentTypeUpdateBody{
		DisplayName: testUpdatedEnvTypeDisplayName,
	}).Return(&cp.UpdateEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.EnvironmentType{
			Id:          testEnvTypeId,
			DisplayName: testUpdatedEnvTypeDisplayName,
			Uuid:        uuid.MustParse("00000000-0000-0000-0000-000000000000"),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, "et", testEnvTypeId, `--set-json={"display_name": "` + testUpdatedEnvTypeDisplayName + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
			"created_at": "0001-01-01T00:00:00Z",
			"id": "my-et",
			"display_name": "`+testUpdatedEnvTypeDisplayName+`",
			"uuid": "00000000-0000-0000-0000-000000000000"
		}`, stdout)
	}
}

func TestUpdate_environment_type_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateEnvironmentTypeWithResponse(gomock.Any(), orgId, testEnvTypeId, cp.EnvironmentTypeUpdateBody{
		DisplayName: testUpdatedEnvTypeDisplayName,
	}).Return(&cp.UpdateEnvironmentTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: envTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, "et", testEnvTypeId, `--set-json={"display_name": "` + testUpdatedEnvTypeDisplayName + `"}`})
	assert.EqualError(t, err, fmt.Sprintf(`environment type "my-et" not found in org %q.`, orgId))
}
