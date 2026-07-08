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

var ruleId = uuid.New()

const (
	moduleRulesTestModuleId             = "s3-dev"
	moduleRulesTestModuleSetFlag        = "--set=module_id=s3-dev"
	moduleRulesTestResourceClass        = "sensitive"
	moduleRulesTestResourceClassSetFlag = "--set=resource_class=sensitive"
	moduleRulesTestNotFound             = "not found"
	moduleRulesTestResourceTypeField    = "ResourceType"
	moduleRulesTestListAlias            = "rules"
)

func TestCreate_create_module_rule_no_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()
	cpc.EXPECT().CreateModuleRuleInOrgWithResponse(gomock.Any(), orgId, cp.RuleCreateBody{
		ModuleId:      moduleRulesTestModuleId,
		ResourceClass: ref.Ref(moduleRulesTestResourceClass),
		ResourceId:    ref.Ref("specific"),
	}).Return(&cp.CreateModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.Rule{
			OrgId:         testModuleOrgId,
			Id:            ruleId,
			ModuleId:      moduleRulesTestModuleId,
			ResourceType:  "s3",
			ResourceClass: moduleRulesTestResourceClass,
			ResourceId:    ref.Ref("specific"),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", moduleRulesTestModuleSetFlag, moduleRulesTestResourceClassSetFlag, `--set=resource_id=specific`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "org-1",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"resource_class": "sensitive",
	"resource_id": "specific",
    "created_at": "0001-01-01T00:00:00Z"
}`, ruleId), stdout)
	}
}

func TestCreate_create_module_rule_with_valid_project(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Project{Id: projectId},
	}, nil)

	cpc.EXPECT().CreateModuleRuleInOrgWithResponse(gomock.Any(), orgId, cp.RuleCreateBody{
		ModuleId:      moduleRulesTestModuleId,
		ProjectId:     ref.Ref(projectId),
		ResourceClass: ref.Ref(moduleRulesTestResourceClass),
	}).Return(&cp.CreateModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.Rule{
			OrgId:         orgId,
			Id:            ruleId,
			ModuleId:      moduleRulesTestModuleId,
			ProjectId:     ref.Ref(projectId),
			ResourceType:  "s3",
			ResourceClass: moduleRulesTestResourceClass,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", moduleRulesTestModuleSetFlag, "--set=project_id=" + projectId, moduleRulesTestResourceClassSetFlag})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"project_id": "%s",
	"resource_class": "sensitive",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, ruleId, projectId), stdout)
	}
}

func TestCreate_create_module_rule_with_valid_project_and_env(t *testing.T) {
	envId := "development"
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Project{Id: projectId},
	}, nil)

	// Expect environment validation
	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, projectId, envId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Environment{Id: envId},
	}, nil)

	cpc.EXPECT().CreateModuleRuleInOrgWithResponse(gomock.Any(), orgId, cp.RuleCreateBody{
		ModuleId:      moduleRulesTestModuleId,
		ProjectId:     ref.Ref(projectId),
		EnvId:         ref.Ref(envId),
		ResourceClass: ref.Ref(moduleRulesTestResourceClass),
	}).Return(&cp.CreateModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.Rule{
			OrgId:         orgId,
			Id:            ruleId,
			ModuleId:      moduleRulesTestModuleId,
			ProjectId:     ref.Ref(projectId),
			EnvId:         ref.Ref(envId),
			ResourceType:  "s3",
			ResourceClass: moduleRulesTestResourceClass,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", moduleRulesTestModuleSetFlag, "--set=project_id=" + projectId, "--set=env_id=" + envId, moduleRulesTestResourceClassSetFlag})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"project_id": "%s",
	"env_id": "%s",
	"resource_class": "sensitive",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, ruleId, projectId, envId), stdout)
	}
}

func TestCreate_create_module_rule_with_nonexistent_project(t *testing.T) {
	projectId := "nonexistent-project"
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation to return 404
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleRulesTestNotFound},
	}, nil)

	// Rule should still be created with --no-prompt flag
	cpc.EXPECT().CreateModuleRuleInOrgWithResponse(gomock.Any(), orgId, cp.RuleCreateBody{
		ModuleId:      moduleRulesTestModuleId,
		ProjectId:     ref.Ref(projectId),
		ResourceClass: ref.Ref(moduleRulesTestResourceClass),
	}).Return(&cp.CreateModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.Rule{
			OrgId:         orgId,
			Id:            ruleId,
			ModuleId:      moduleRulesTestModuleId,
			ProjectId:     ref.Ref(projectId),
			ResourceType:  "s3",
			ResourceClass: moduleRulesTestResourceClass,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", noPromptFlag, moduleRulesTestModuleSetFlag, "--set=project_id=" + projectId, moduleRulesTestResourceClassSetFlag})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"project_id": "%s",
	"resource_class": "sensitive",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, ruleId, projectId), stdout)
	}
}

func TestCreate_create_module_rule_with_valid_project_and_nonexistent_env(t *testing.T) {
	envId := "nonexistent-env"
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation to succeed
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Project{Id: projectId},
	}, nil)

	// Expect environment validation to return 404
	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, projectId, envId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleRulesTestNotFound},
	}, nil)

	// Rule should still be created with --no-prompt flag
	cpc.EXPECT().CreateModuleRuleInOrgWithResponse(gomock.Any(), orgId, cp.RuleCreateBody{
		ModuleId:      moduleRulesTestModuleId,
		ProjectId:     ref.Ref(projectId),
		EnvId:         ref.Ref(envId),
		ResourceClass: ref.Ref(moduleRulesTestResourceClass),
	}).Return(&cp.CreateModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201: &cp.Rule{
			OrgId:         orgId,
			Id:            ruleId,
			ModuleId:      moduleRulesTestModuleId,
			ProjectId:     ref.Ref(projectId),
			EnvId:         ref.Ref(envId),
			ResourceType:  "s3",
			ResourceClass: moduleRulesTestResourceClass,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", noPromptFlag, moduleRulesTestModuleSetFlag, "--set=project_id=" + projectId, "--set=env_id=" + envId, moduleRulesTestResourceClassSetFlag})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "%s",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"project_id": "%s",
	"env_id": "%s",
	"resource_class": "sensitive",
    "created_at": "0001-01-01T00:00:00Z"
}`, orgId, ruleId, projectId, envId), stdout)
	}
}

func TestCreate_create_module_rule_project_validation_unexpected_error(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation to return an unexpected status code (500)
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
		Body:         []byte("Internal Server Error"),
	}, nil)

	// The command should fail with an error about unexpected status code
	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", moduleRulesTestModuleSetFlag, "--set=project_id=" + projectId, moduleRulesTestResourceClassSetFlag})
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "unexpected status code 500")
	}
}

func TestCreate_create_module_rule_env_validation_unexpected_error(t *testing.T) {
	envId := testEnvId
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	// Expect project validation to succeed
	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, projectId).Return(&cp.GetProjectResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.Project{Id: projectId},
	}, nil)

	// Expect environment validation to return an unexpected status code (500)
	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, projectId, envId).Return(&cp.GetEnvironmentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusInternalServerError},
		Body:         []byte("Internal Server Error"),
	}, nil)

	// The command should fail with an error about unexpected status code
	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, "rl", moduleRulesTestModuleSetFlag, "--set=project_id=" + projectId, "--set=env_id=" + envId, moduleRulesTestResourceClassSetFlag})
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "unexpected status code 500")
	}
}

func TestDelete_module_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteModuleRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.DeleteModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "rl", ruleId.String()})
	assert.NoError(t, err)
}

func TestDelete_module_rule_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteModuleRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.DeleteModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleRulesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, "rl", ruleId.String()})
	assert.EqualError(t, err, fmt.Sprintf("module rule '%s' not found in org '%s'", ruleId, orgId))
}

func TestGet_module_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.GetModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Rule{
			OrgId:         testModuleOrgId,
			Id:            ruleId,
			ResourceType:  "s3",
			ModuleId:      moduleRulesTestModuleId,
			ResourceClass: moduleRulesTestResourceClass,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "rl", ruleId.String()})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`{
    "org_id": "org-1",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"resource_class": "sensitive",
    "created_at": "0001-01-01T00:00:00Z"
}`, ruleId), stdout)
	}
}

func TestGet_module_rule_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.GetModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Rule{
			OrgId:         testModuleOrgId,
			Id:            ruleId,
			ResourceType:  "s3",
			ModuleId:      moduleRulesTestModuleId,
			ResourceClass: moduleRulesTestResourceClass,
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, "rl", ruleId.String()})

	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")

		assert.Contains(t, stdout, moduleRulesTestResourceTypeField)
		assert.Contains(t, stdout, "s3")

		assert.Contains(t, stdout, "ResourceClass")
		assert.Contains(t, stdout, moduleRulesTestResourceClass)
	}
}

func TestGet_module_rule_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetModuleRuleInOrgWithResponse(gomock.Any(), orgId, ruleId).Return(&cp.GetModuleRuleInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: moduleRulesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "rl", ruleId.String()})
	assert.EqualError(t, err, fmt.Sprintf("module rule '%s' not found in org '%s'", ruleId, orgId))
}

func TestList_module_rule(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{}).Return(&cp.ListModuleRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RulePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{Page: ref.Ref("next-page")}).
		Return(&cp.ListModuleRulesInOrgResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &cp.RulePage{Items: []cp.RuleSummary{
				{OrgId: orgId, Id: ruleId, ResourceType: "s3", ResourceClass: moduleRulesTestResourceClass, ResourceId: ref.Ref("specific"), ModuleId: moduleRulesTestModuleId},
			}},
		}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, moduleRulesTestListAlias})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`[{
    "org_id": "%s",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"resource_class": "sensitive",
	"resource_id": "specific",
    "created_at": "0001-01-01T00:00:00Z"
}]`, orgId, ruleId), stdout)
	}
}

func TestList_module_rule_with_fiters(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{
		ByResourceType: ref.Ref("s3"),
		ByModuleId:     ref.Ref(moduleRulesTestModuleId),
	}).Return(&cp.ListModuleRulesInOrgResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RulePage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListModuleRulesInOrgWithResponse(gomock.Any(), orgId, &cp.ListModuleRulesInOrgParams{
		Page:           ref.Ref("next-page"),
		ByResourceType: ref.Ref("s3"),
		ByModuleId:     ref.Ref(moduleRulesTestModuleId),
	}).
		Return(&cp.ListModuleRulesInOrgResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &cp.RulePage{Items: []cp.RuleSummary{
				{OrgId: orgId, Id: ruleId, ResourceType: "s3", ResourceClass: moduleRulesTestResourceClass, ResourceId: ref.Ref("specific"), ModuleId: moduleRulesTestModuleId},
			}},
		}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, moduleRulesTestListAlias, "--type", "s3", "--module", moduleRulesTestModuleId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, fmt.Sprintf(`[{
    "org_id": "%s",
	"id": "%s",
    "resource_type": "s3",
	"module_id": "s3-dev",
	"resource_class": "sensitive",
	"resource_id": "specific",
    "created_at": "0001-01-01T00:00:00Z"
}]`, orgId, ruleId), stdout)
	}
}
