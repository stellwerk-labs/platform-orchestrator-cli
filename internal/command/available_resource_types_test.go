package command

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	testArtResourceTypeId = "postgres"
	testArtDescription    = "My PostgreSQL"
	testArtModuleId       = "def-1"
	testArtRuleId         = "rule-1"
	testArtHostField      = "host"
	testArtStringType     = "string"
	testArtObjectType     = "available-resource-type"
)

func TestGet_available_resource_type(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListAvailableResourceTypesWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.ListAvailableResourceTypesParams{TypeId: ref.Ref(testArtResourceTypeId)}).Return(&cp.ListAvailableResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.AvailableResourceTypePage{
			Items: []cp.AvailableResourceType{
				{
					Id:          testArtResourceTypeId,
					Description: ref.Ref(testArtDescription),
					Options: []cp.AvailableResourceTypeOption{
						{ResourceClass: testMpDefaultId, ModuleId: testArtModuleId, RuleId: testArtRuleId, ModuleParams: map[string]cp.ModuleParamItem{}},
					},
					OutputSchema: map[string]interface{}{
						testArtHostField: testArtStringType,
					},
				},
			},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, testArtObjectType, testProjectId, testEnvId, testArtResourceTypeId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
	"id": "`+testArtResourceTypeId+`",
	"description": "`+testArtDescription+`",
	"options": [{
		"resource_class": "default",
		"module_id": "`+testArtModuleId+`",
        "module_params": {},
		"rule_id": "`+testArtRuleId+`"
	}],
	"output_schema": {
		"host": "string"
	}
}`, stdout)
	}
}

func TestGet_available_resource_type_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListAvailableResourceTypesWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.ListAvailableResourceTypesParams{TypeId: ref.Ref(testArtResourceTypeId)}).Return(&cp.ListAvailableResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.AvailableResourceTypePage{
			Items: []cp.AvailableResourceType{
				{
					Id:          testArtResourceTypeId,
					Description: ref.Ref(testArtDescription),
					Options: []cp.AvailableResourceTypeOption{
						{ResourceClass: testMpDefaultId, ModuleId: testArtModuleId, RuleId: testArtRuleId, ModuleParams: map[string]cp.ModuleParamItem{}},
					},
					OutputSchema: map[string]interface{}{
						testArtHostField: testArtStringType,
					},
				},
			},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, testArtObjectType, testProjectId, testEnvId, testArtResourceTypeId})

	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
	}
}

func TestGet_available_resource_type_no_type_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListAvailableResourceTypesWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.ListAvailableResourceTypesParams{TypeId: ref.Ref(testArtResourceTypeId)}).Return(&cp.ListAvailableResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.AvailableResourceTypePage{
			Items: []cp.AvailableResourceType{},
		},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, testArtObjectType, testProjectId, testEnvId, testArtResourceTypeId})
	if assert.Error(t, err) {
		assert.EqualError(t, err, "available resource type '"+testArtResourceTypeId+"' not found in project '"+testProjectId+"' for environment '"+testEnvId+"'")
	}
}

func TestList_available_resource_types(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListAvailableResourceTypesWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.ListAvailableResourceTypesParams{}).Return(&cp.ListAvailableResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.AvailableResourceTypePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListAvailableResourceTypesWithResponse(gomock.Any(), orgId, testProjectId, testEnvId, &cp.ListAvailableResourceTypesParams{Page: ref.Ref("next-page")}).Return(&cp.ListAvailableResourceTypesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.AvailableResourceTypePage{Items: []cp.AvailableResourceType{
			{
				Id:          testArtResourceTypeId,
				Description: ref.Ref(testArtDescription),
				Options: []cp.AvailableResourceTypeOption{
					{ResourceClass: testMpDefaultId, ModuleId: testArtModuleId, RuleId: testArtRuleId, ModuleParams: map[string]cp.ModuleParamItem{}},
				},
				OutputSchema: map[string]interface{}{
					testArtHostField: testArtStringType,
				},
			},
		}, NextPageToken: nil},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "available-resource-types", testProjectId, testEnvId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
	"id": "`+testArtResourceTypeId+`",
	"description": "`+testArtDescription+`",
	"options": [{
		"resource_class": "default",
		"module_id": "`+testArtModuleId+`",
        "module_params": {},
		"rule_id": "`+testArtRuleId+`"
	}],
	"output_schema": {
		"host": "string"
	}
}]`, stdout)
	}
}
