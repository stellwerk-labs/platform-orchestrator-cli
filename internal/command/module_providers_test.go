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
	testVersionConstraint = ">=v1"
	testMpProviderType    = "aws"
	testMpId              = "example"
	testMpSource          = "http://my/module"
	testMpDescription     = "An example"
	testMpDefaultId       = "default"
	testMpDefaultSource   = "module-source"
	testMpOrgId           = "my-org"
	mpTestNotFound        = "not found"
	mpTestProviderType    = "ProviderType"
	mpTestUpdateSet       = "--set=description=something"
)

func TestCreate_create_mp(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateModuleProviderWithResponse(gomock.Any(), orgId, cp.ModuleProviderCreateBody{
		ProviderType:      testMpProviderType,
		Id:                testMpId,
		Source:            testMpSource,
		VersionConstraint: testVersionConstraint,
		Description:       ref.Ref(testMpDescription),
		Configuration:     map[string]interface{}{},
	}).Return(&cp.CreateModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.ModuleProvider{
			OrgId:             testMpOrgId,
			ProviderType:      testMpProviderType,
			Id:                testMpId,
			Source:            testMpSource,
			VersionConstraint: testVersionConstraint,
			Description:       ref.Ref(testMpDescription),
			Configuration:     map[string]interface{}{},
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{
		orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "mp",
		testMpProviderType, testMpId,
		"--set=source=" + testMpSource,
		"--set=version_constraint=" + testVersionConstraint,
		`--set-json={"description": "` + testMpDescription + `", "configuration": {}}`,
	})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "`+testMpOrgId+`",
	"provider_type": "`+testMpProviderType+`",
	"created_at": "0001-01-01T00:00:00Z",
	"id": "`+testMpId+`",
	"description": "`+testMpDescription+`",
	"source": "`+testMpSource+`",
	"version_constraint": "`+testVersionConstraint+`",
	"configuration": {}
}`, stdout)
	}
}

func TestDelete_mp(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId).Return(&cp.DeleteModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "mp", testMpProviderType, testMpDefaultId})
	assert.NoError(t, err)
}

func TestDelete_mp_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId).Return(&cp.DeleteModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: mpTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "mp", testMpProviderType, testMpDefaultId})
	assert.EqualError(t, err, fmt.Sprintf("module provider '%s' '%s' not found in org '%s'", testMpProviderType, testMpDefaultId, orgId))
}

func TestGet_mp(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId).Return(&cp.GetModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.ModuleProvider{
			OrgId:             testMpOrgId,
			ProviderType:      testMpProviderType,
			Id:                testMpId,
			Source:            testMpSource,
			VersionConstraint: testVersionConstraint,
			Configuration:     map[string]interface{}{},
			Description:       ref.Ref(testMpDescription),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "mp", testMpProviderType, testMpDefaultId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "`+testMpOrgId+`",
	"provider_type": "`+testMpProviderType+`",
	"created_at": "0001-01-01T00:00:00Z",
	"id": "`+testMpId+`",
	"description": "`+testMpDescription+`",
	"source": "`+testMpSource+`",
	"version_constraint": "`+testVersionConstraint+`",
	"configuration": {}
}`, stdout)
	}
}

func TestGet_mp_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId).Return(&cp.GetModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.ModuleProvider{
			OrgId:             testMpOrgId,
			ProviderType:      testMpProviderType,
			Id:                testMpId,
			Source:            testMpSource,
			VersionConstraint: testVersionConstraint,
			Configuration:     map[string]interface{}{},
			Description:       ref.Ref(testMpDescription),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, "mp", testMpProviderType, testMpDefaultId})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, mpTestProviderType)
		assert.Contains(t, stdout, testMpProviderType)
		assert.Contains(t, stdout, "Source")
		assert.Contains(t, stdout, testMpSource)
		assert.Contains(t, stdout, "VersionConstraint")
		assert.Contains(t, stdout, testVersionConstraint)
		assert.Contains(t, stdout, "Configuration")
		assert.Contains(t, stdout, "map[]")
	}
}

func TestGet_mp_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId).Return(&cp.GetModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: mpTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "mp", testMpProviderType, testMpDefaultId})
	assert.EqualError(t, err, fmt.Sprintf("module provider '%s' '%s' not found in org '%s'", testMpProviderType, testMpDefaultId, orgId))
}

func TestList_mp(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListModuleProvidersWithResponse(gomock.Any(), orgId, &cp.ListModuleProvidersParams{}).Return(&cp.ListModuleProvidersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ModuleProviderPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListModuleProvidersWithResponse(gomock.Any(), orgId, &cp.ListModuleProvidersParams{Page: ref.Ref("next-page")}).Return(&cp.ListModuleProvidersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.ModuleProviderPage{Items: []cp.ModuleProviderSummary{{OrgId: testMpOrgId, ProviderType: testMpProviderType, Id: testMpDefaultId, Source: testMpDefaultSource}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "mps"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
    "org_id": "`+testMpOrgId+`",
	"provider_type": "`+testMpProviderType+`",
	"id": "`+testMpDefaultId+`",
	"created_at": "0001-01-01T00:00:00Z",
    "source": "`+testMpDefaultSource+`"
}]`, stdout)
	}
}

func TestUpdate_mp(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId, cp.ModuleProviderUpdateBody{
		Description: ref.Ref("something"),
	}).Return(&cp.UpdateModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.ModuleProvider{
			OrgId:             testMpOrgId,
			ProviderType:      testMpProviderType,
			Id:                testMpId,
			Source:            testMpSource,
			VersionConstraint: testVersionConstraint,
			Configuration:     map[string]interface{}{},
			Description:       ref.Ref("something"),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, "mp", testMpProviderType, testMpDefaultId, mpTestUpdateSet})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "`+testMpOrgId+`",
	"provider_type": "`+testMpProviderType+`",
	"created_at": "0001-01-01T00:00:00Z",
	"id": "`+testMpId+`",
	"description": "something",
	"source": "`+testMpSource+`",
	"version_constraint": "`+testVersionConstraint+`",
	"configuration": {}
}`, stdout)
	}
}

func TestUpdate_mp_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateModuleProviderWithResponse(gomock.Any(), orgId, testMpProviderType, testMpDefaultId, cp.ModuleProviderUpdateBody{
		Description: ref.Ref("something"),
	}).Return(&cp.UpdateModuleProviderResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: mpTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, "mp", testMpProviderType, testMpDefaultId, mpTestUpdateSet})
	assert.EqualError(t, err, fmt.Sprintf("module provider '%s' '%s' not found in org '%s'", testMpProviderType, testMpDefaultId, orgId))
}
