package command

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	resourceTypesTestCreateId    = "rt-1"
	resourceTypesTestId          = "my-rt"
	resourceTypesTestNotFound    = "not found"
	resourceTypesTestDescription = "Description"
)

func TestCreate_create_rt(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateResourceTypeWithResponse(gomock.Any(), orgId, cp.ResourceTypeCreateBody{
		Id:           resourceTypesTestCreateId,
		OutputSchema: map[string]interface{}{listModulesTypeFlag: "object"},
	}).Return(&cp.CreateResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201:      &cp.ResourceType{Id: resourceTypesTestCreateId, OutputSchema: map[string]interface{}{}, IsDeveloperAccessible: true},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rt", resourceTypesTestCreateId, `--set-json={"output_schema": {"type": "object"}}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"built_in": false,
	"created_at": "0001-01-01T00:00:00Z",
	"id": "rt-1",
	"output_schema": {},
	"is_developer_accessible": true
}`, stdout)
	}
}

func TestDelete_rt(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId).Return(&cp.DeleteResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "rt", resourceTypesTestId})
	assert.NoError(t, err)
}

func TestDelete_rt_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId).Return(&cp.DeleteResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: resourceTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "rt", resourceTypesTestId})
	assert.EqualError(t, err, fmt.Sprintf("resource type 'my-rt' not found in org '%s'", orgId))
}

func TestGet_rt(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId).Return(&cp.GetResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.ResourceType{
			Id:           resourceTypesTestId,
			Description:  ref.Ref("My Resource Type"),
			OutputSchema: map[string]interface{}{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "rt", resourceTypesTestId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"built_in": false,
	"created_at": "0001-01-01T00:00:00Z",
	"id": "my-rt",
	"description": "My Resource Type",
	"output_schema": {},
	"is_developer_accessible": false
}`, stdout)
	}
}

func TestGet_rt_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId).Return(&cp.GetResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.ResourceType{
			Id:           resourceTypesTestId,
			Description:  ref.Ref("My Resource Type"),
			OutputSchema: map[string]interface{}{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, "rt", resourceTypesTestId})

	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, resourceTypesTestId)

		assert.Contains(t, stdout, "BuiltIn")
		assert.Contains(t, stdout, stringFalse)

		assert.Contains(t, stdout, resourceTypesTestDescription)
		assert.Contains(t, stdout, "My Resource Type")

		assert.Contains(t, stdout, "IsDeveloperAccessible")
		assert.Contains(t, stdout, stringFalse)
	}
}
func TestGet_rt_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId).Return(&cp.GetResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: resourceTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "rt", resourceTypesTestId})
	assert.EqualError(t, err, fmt.Sprintf("resource type 'my-rt' not found in org '%s'", orgId))
}

func TestList_rt(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListResourceTypesWithResponse(gomock.Any(), orgId, &cp.ListResourceTypesParams{}).Return(&cp.ListResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ResourceTypePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListResourceTypesWithResponse(gomock.Any(), orgId, &cp.ListResourceTypesParams{Page: ref.Ref("next-page")}).Return(&cp.ListResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ResourceTypePage{Items: []cp.ResourceType{{Id: resourceTypesTestId, Description: ref.Ref("My Resource Type"), OutputSchema: map[string]interface{}{}}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "rts"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
	"built_in": false,
	"created_at": "0001-01-01T00:00:00Z",
	"id": "my-rt",
	"description": "My Resource Type",
	"output_schema": {},
	"is_developer_accessible": false
}]`, stdout)
	}
}

func TestUpdate_rt(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId, cp.ResourceTypeUpdateBody{Description: ref.Ref("MyRT")}).Return(&cp.UpdateResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ResourceType{Id: resourceTypesTestId, Description: ref.Ref("MyRT"), OutputSchema: map[string]interface{}{}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, "rt", resourceTypesTestId, "--set=description=MyRT"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"built_in": false,
	"created_at": "0001-01-01T00:00:00Z",
	"id": "my-rt",
	"description": "MyRT",
	"output_schema": {},
	"is_developer_accessible": false
}`, stdout)
	}
}

func TestUpdate_rt_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateResourceTypeWithResponse(gomock.Any(), orgId, resourceTypesTestId, cp.ResourceTypeUpdateBody{Description: ref.Ref("MyRT")}).Return(&cp.UpdateResourceTypeResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: resourceTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, "rt", resourceTypesTestId, "--set=description=MyRT"})
	assert.EqualError(t, err, fmt.Sprintf("resource type 'my-rt' not found in org '%s'", orgId))
}
