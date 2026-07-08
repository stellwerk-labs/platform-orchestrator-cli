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
	testModuleSource      = "/modules/s3"
	testResourceTypeS3    = "s3"
	testModuleDescription = "An example"
	testModuleVersionId   = "012345"
	testModuleOrgId       = "org-1"
	moduleTestNotFound    = "not found"
	moduleTestUpdateSet   = "--set=description=something"
)

func TestCreate_create_module(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateModuleWithResponse(gomock.Any(), orgId, cp.ModuleCreateBody{
		Id:           testMpId,
		ResourceType: testResourceTypeS3,
		ModuleSource: testModuleSource,
		Description:  ref.Ref(testModuleDescription),
	}).Return(&cp.CreateModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.Module{
			OrgId:           testModuleOrgId,
			Id:              testMpId,
			ResourceType:    testResourceTypeS3,
			ModuleSource:    testModuleSource,
			VersionId:       testModuleVersionId,
			Description:     ref.Ref(testModuleDescription),
			Dependencies:    map[string]cp.ModuleDependencyManifest{},
			Coprovisioned:   []cp.ModuleCoProvisionManifest{},
			ModuleParams:    map[string]cp.ModuleParamItem{},
			ModuleInputs:    map[string]interface{}{},
			ProviderMapping: map[string]string{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "mod", testMpId, "--set=module_source=" + testModuleSource, "--set=resource_type=" + testResourceTypeS3, `--set-json={"description": "` + testModuleDescription + `"}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "`+testModuleOrgId+`",
	"id": "example",
    "resource_type": "`+testResourceTypeS3+`",
	"created_at": "0001-01-01T00:00:00Z",
	"description": "`+testModuleDescription+`",
	"module_source": "`+testModuleSource+`",
    "module_params": {},
    "module_inputs": {},
    "provider_mapping": {},
    "dependencies": {},
    "coprovisioned": [],
    "version_id": "`+testModuleVersionId+`",
    "updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestDelete_module(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteModuleWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.DeleteModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, listModuleRulesModuleFlag, testMpId})
	assert.NoError(t, err)
}

func TestDelete_module_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteModuleWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.DeleteModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, listModuleRulesModuleFlag, testMpId})
	assert.EqualError(t, err, fmt.Sprintf("module 'example' not found in org '%s'", orgId))
}

func TestGet_module(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.GetModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Module{
			OrgId:           testModuleOrgId,
			Id:              testMpId,
			ResourceType:    testResourceTypeS3,
			ModuleSource:    testModuleSource,
			VersionId:       testModuleVersionId,
			Description:     ref.Ref(testModuleDescription),
			Dependencies:    map[string]cp.ModuleDependencyManifest{},
			Coprovisioned:   []cp.ModuleCoProvisionManifest{},
			ModuleParams:    map[string]cp.ModuleParamItem{},
			ModuleInputs:    map[string]interface{}{},
			ProviderMapping: map[string]string{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, listModuleRulesModuleFlag, testMpId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "`+testModuleOrgId+`",
	"id": "example",
    "resource_type": "`+testResourceTypeS3+`",
	"created_at": "0001-01-01T00:00:00Z",
	"description": "`+testModuleDescription+`",
	"module_source": "`+testModuleSource+`",
    "module_params": {},
    "module_inputs": {},
    "provider_mapping": {},
    "dependencies": {},
    "coprovisioned": [],
    "version_id": "`+testModuleVersionId+`",
    "updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestGet_module_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.GetModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Module{
			OrgId:           testModuleOrgId,
			Id:              testMpId,
			ResourceType:    testResourceTypeS3,
			ModuleSource:    testModuleSource,
			VersionId:       testModuleVersionId,
			Description:     ref.Ref(testModuleDescription),
			Dependencies:    map[string]cp.ModuleDependencyManifest{},
			Coprovisioned:   []cp.ModuleCoProvisionManifest{},
			ModuleParams:    map[string]cp.ModuleParamItem{},
			ModuleInputs:    map[string]interface{}{},
			ProviderMapping: map[string]string{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, listModuleRulesModuleFlag, testMpId})

	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")

		assert.Contains(t, stdout, "OrgId")
		assert.Contains(t, stdout, testModuleOrgId)

		assert.Contains(t, stdout, "ResourceType")
		assert.Contains(t, stdout, testResourceTypeS3)

		assert.Contains(t, stdout, "ModuleSource")
		assert.Contains(t, stdout, testModuleSource)

		assert.Contains(t, stdout, "VersionId")
		assert.Contains(t, stdout, testModuleVersionId)
	}
}

func TestGet_module_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.GetModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, listModuleRulesModuleFlag, testMpId})
	assert.EqualError(t, err, fmt.Sprintf("module 'example' not found in org '%s'", orgId))
}

func TestList_module(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListModulesWithResponse(gomock.Any(), orgId, &cp.ListModulesParams{}).Return(&cp.ListModulesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ModulePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListModulesWithResponse(gomock.Any(), orgId, &cp.ListModulesParams{Page: ref.Ref("next-page")}).Return(&cp.ListModulesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.ModulePage{Items: []cp.ModuleSummary{
			{OrgId: "my-org", Id: testMpDefaultId, ResourceType: testResourceTypeS3, ModuleSource: testModuleSource, ProviderMapping: map[string]string{}},
		}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "modules"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
    "org_id": "my-org",
	"id": "`+testMpDefaultId+`",
    "resource_type": "`+testResourceTypeS3+`",
	"created_at": "0001-01-01T00:00:00Z",
    "module_source": "`+testModuleSource+`",
    "version_id": "",
    "updated_at": "0001-01-01T00:00:00Z",
    "provider_mapping": {}
}]`, stdout)
	}
}

func TestUpdate_module(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateModuleWithResponse(gomock.Any(), orgId, testMpId, cp.ModuleUpdateBody{
		Description: ref.Ref("something"),
	}).Return(&cp.UpdateModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Module{
			OrgId:           testModuleOrgId,
			Id:              testMpId,
			ResourceType:    testResourceTypeS3,
			ModuleSource:    testModuleSource,
			VersionId:       testModuleVersionId,
			Description:     ref.Ref(testModuleDescription),
			Dependencies:    map[string]cp.ModuleDependencyManifest{},
			Coprovisioned:   []cp.ModuleCoProvisionManifest{},
			ModuleParams:    map[string]cp.ModuleParamItem{},
			ModuleInputs:    map[string]interface{}{},
			ProviderMapping: map[string]string{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testUpdateCmd, outFlag, jsonOutput, listModuleRulesModuleFlag, testMpId, moduleTestUpdateSet})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "`+testModuleOrgId+`",
	"id": "example",
    "resource_type": "`+testResourceTypeS3+`",
	"created_at": "0001-01-01T00:00:00Z",
	"description": "`+testModuleDescription+`",
	"module_source": "`+testModuleSource+`",
    "module_params": {},
    "module_inputs": {},
    "provider_mapping": {},
    "dependencies": {},
    "coprovisioned": [],
    "version_id": "`+testModuleVersionId+`",
    "updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestUpdate_module_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateModuleWithResponse(gomock.Any(), orgId, testMpId, cp.ModuleUpdateBody{Description: ref.Ref("something")}).Return(&cp.UpdateModuleResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testUpdateCmd, outFlag, jsonOutput, listModuleRulesModuleFlag, testMpId, moduleTestUpdateSet})
	assert.EqualError(t, err, fmt.Sprintf("module 'example' not found in org '%s'", orgId))
}
