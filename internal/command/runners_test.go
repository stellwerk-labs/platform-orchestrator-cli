package command

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	runnersTestServiceAccount = "platform-orchestrator-runner"
	runnersTestNotFound       = "not found"
	runnersTestUpdateSet      = "--set=description=something"
)

func getK8sRunnerConfiguration() cp.RunnerConfiguration {
	cfg := new(cp.RunnerConfiguration)
	_ = cfg.FromK8sRunnerConfiguration(cp.K8sRunnerConfiguration{
		Job: cp.K8sRunnerJobConfig{
			Namespace:      testMpDefaultId,
			ServiceAccount: runnersTestServiceAccount,
		},
		Cluster: cp.K8sRunnerK8sCluster{
			ClusterData: cp.K8sRunnerK8sClusterClusterData{
				Server: "https://kubernetes.default.svc",
			},
		},
	})
	return *cfg
}

func getRemoteK8sRunnerConfiguration() cp.RunnerConfiguration {
	cfg := new(cp.RunnerConfiguration)
	_ = cfg.FromK8sAgentRunnerConfiguration(cp.K8sAgentRunnerConfiguration{
		Job: cp.K8sRunnerJobConfig{
			Namespace:      testMpDefaultId,
			ServiceAccount: runnersTestServiceAccount,
		},
		Key: "public-key",
	})
	return *cfg
}

func getStateStorageConfiguration() cp.StateStorageConfiguration {
	cfg := new(cp.StateStorageConfiguration)
	_ = cfg.FromK8sStorageConfiguration(cp.K8sStorageConfiguration{
		Namespace: testMpDefaultId,
	})
	return *cfg
}

func TestCreate_create_runner_json(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateRunnerWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, body cp.RunnerCreateBody, reqEditors ...cp.RequestEditorFn) (*cp.CreateRunnerResponse, error) {
			assert.Equal(t, testMpDefaultId, body.Id)
			k8sCfg, err := body.RunnerConfiguration.AsK8sRunnerConfiguration()
			require.NoError(t, err)
			assert.Equal(t, cp.K8sRunnerK8sCluster{
				ClusterData: cp.K8sRunnerK8sClusterClusterData{
					Server:                   "https://kubernetes.default.svc",
					CertificateAuthorityData: "",
				},
				Auth: cp.K8sRunnerK8sClusterAuth{},
			}, k8sCfg.Cluster)
			assert.Equal(t, cp.K8sRunnerJobConfig{
				Namespace:      testMpDefaultId,
				ServiceAccount: runnersTestServiceAccount,
			}, k8sCfg.Job)
			k8sStateCfg, err := body.StateStorageConfiguration.AsK8sStorageConfiguration()
			require.NoError(t, err)
			assert.Equal(t, cp.K8sStorageConfiguration{
				Namespace: testMpDefaultId,
				Type:      cp.StateStorageTypeKubernetes,
			}, k8sStateCfg)
			return &cp.CreateRunnerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
				JSON201: &cp.Runner{
					OrgId:                     testModuleOrgId,
					Id:                        testMpDefaultId,
					RunnerConfiguration:       getK8sRunnerConfiguration(),
					StateStorageConfiguration: getStateStorageConfiguration(),
				},
			}, nil
		},
	).Times(1)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, listRunnerRulesRunnerFlag, testMpDefaultId, `--set-json={
"runner_configuration": {
  "type": "kubernetes",
  "job": {
    "namespace": "default",
    "service_account": "platform-orchestrator-runner"
  },
  "cluster": {
	"cluster_data": {
		"server": "https://kubernetes.default.svc"
	}
  }
},
"state_storage_configuration": {
  "type": "kubernetes",
  "namespace": "default"
}}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "org-1",
	"id": "default",
	"runner_configuration": {
	  "type": "kubernetes",
      "job": {
	    "namespace": "default",
		"service_account": "platform-orchestrator-runner"
	  },
	  "cluster": {
		"cluster_data": {
			"server": "https://kubernetes.default.svc",
			"certificate_authority_data": ""
		},
		"auth": {}
	  }
	},
	"state_storage_configuration": {
	  "type": "kubernetes",
	  "namespace": "default"
	},
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestCreate_create_remote_runner_json(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().CreateRunnerWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, body cp.RunnerCreateBody, reqEditors ...cp.RequestEditorFn) (*cp.CreateRunnerResponse, error) {
			assert.Equal(t, testMpDefaultId, body.Id)
			k8sAgent, err := body.RunnerConfiguration.AsK8sAgentRunnerConfiguration()
			require.NoError(t, err)
			assert.Equal(t, cp.K8sRunnerJobConfig{
				Namespace:      testMpDefaultId,
				ServiceAccount: runnersTestServiceAccount,
			}, k8sAgent.Job)
			assert.Equal(t, "public-key", k8sAgent.Key)
			k8sStateCfg, err := body.StateStorageConfiguration.AsK8sStorageConfiguration()
			require.NoError(t, err)
			assert.Equal(t, cp.K8sStorageConfiguration{
				Namespace: testMpDefaultId,
				Type:      cp.StateStorageTypeKubernetes,
			}, k8sStateCfg)
			return &cp.CreateRunnerResponse{
				HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
				JSON201: &cp.Runner{
					OrgId:                     testModuleOrgId,
					Id:                        testMpDefaultId,
					RunnerConfiguration:       getRemoteK8sRunnerConfiguration(),
					StateStorageConfiguration: getStateStorageConfiguration(),
				},
			}, nil
		},
	).Times(1)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testCreateCmd, listRunnerRulesRunnerFlag, testMpDefaultId, `--set-json={
"runner_configuration": {
  "type": "kubernetes-agent",
  "job": {
    "namespace": "default",
    "service_account": "platform-orchestrator-runner"
  },
  "key": "public-key"
},
"state_storage_configuration": {
  "type": "kubernetes",
  "namespace": "default"
}}`})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "org-1",
	"id": "default",
	"runner_configuration": {
	  "type": "kubernetes-agent",
	   "job": {
	     "namespace": "default",
		  "service_account": "platform-orchestrator-runner"
		},
	  "key": "public-key"
	},
	"state_storage_configuration": {
	  "type": "kubernetes",
	  "namespace": "default"
	},
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestDelete_runner(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteRunnerWithResponse(gomock.Any(), orgId, testMpDefaultId).Return(&cp.DeleteRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, listRunnerRulesRunnerFlag, testMpDefaultId})
	assert.NoError(t, err)
}

func TestDelete_runner_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().DeleteRunnerWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.DeleteRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: runnersTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeleteCmd, listRunnerRulesRunnerFlag, testMpId})
	assert.EqualError(t, err, fmt.Sprintf("runner 'example' not found in org '%s'", orgId))
}

func TestGet_runner(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetRunnerWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.GetRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Runner{
			OrgId:                     testModuleOrgId,
			Id:                        testMpId,
			RunnerConfiguration:       getK8sRunnerConfiguration(),
			StateStorageConfiguration: getStateStorageConfiguration(),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, listRunnerRulesRunnerFlag, testMpId})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "org-1",
	"id": "example",
	"runner_configuration": {
	  "type": "kubernetes",
      "job": {
	    "namespace": "default",
		"service_account": "platform-orchestrator-runner"
	  },
	  "cluster": {
		"cluster_data": {
			"server": "https://kubernetes.default.svc",
			"certificate_authority_data": ""
		},
		"auth": {}
	  }
	},
	"state_storage_configuration": {
	  "type": "kubernetes",
	  "namespace": "default"
	},
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestGet_runner_default_printer(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetRunnerWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.GetRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Runner{
			OrgId:                     testModuleOrgId,
			Id:                        testMpId,
			RunnerConfiguration:       getK8sRunnerConfiguration(),
			StateStorageConfiguration: getStateStorageConfiguration(),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testGetCmd, listRunnerRulesRunnerFlag, testMpId})

	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Id")
		assert.Contains(t, stdout, "OrgId")
		assert.Contains(t, stdout, testModuleOrgId)
	}
}

func TestGet_runner_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().GetRunnerWithResponse(gomock.Any(), orgId, testMpId).Return(&cp.GetRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: runnersTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, listRunnerRulesRunnerFlag, testMpId})
	assert.EqualError(t, err, fmt.Sprintf("runner 'example' not found in org '%s'", orgId))
}

func TestList_runners(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().ListRunnersWithResponse(gomock.Any(), orgId, &cp.ListRunnersParams{}).Return(&cp.ListRunnersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &cp.RunnerPage{NextPageToken: ref.Ref("next-page")},
	}, nil)
	cpc.EXPECT().ListRunnersWithResponse(gomock.Any(), orgId, &cp.ListRunnersParams{Page: ref.Ref("next-page")}).Return(&cp.ListRunnersResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.RunnerPage{Items: []cp.RunnerSummary{
			{OrgId: "my-org", Id: testMpDefaultId, RunnerConfiguration: &cp.RunnerConfigurationSummary{Type: cp.RunnerTypeKubernetes}, StateStorageConfiguration: &cp.StateStorageConfigurationSummary{
				Type: cp.StateStorageTypeKubernetes}},
		}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testGetCmd, "runners"})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `[{
	"org_id": "my-org",
	"id": "default",
	"runner_configuration": {
	  "type": "kubernetes"
	},
	"state_storage_configuration": {
	  "type": "kubernetes"
	},
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z"
}]`, stdout)
	}
}

func TestUpdate_runner(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateRunnerWithResponse(gomock.Any(), orgId, testMpId, cp.RunnerUpdateBody{Description: ref.Ref("something")}).Return(&cp.UpdateRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &cp.Runner{
			OrgId:                     testModuleOrgId,
			Id:                        testMpId,
			RunnerConfiguration:       getK8sRunnerConfiguration(),
			StateStorageConfiguration: getStateStorageConfiguration(),
		},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, listRunnerRulesRunnerFlag, testMpId, runnersTestUpdateSet})
	if assert.NoError(t, err) {
		assert.JSONEq(t, `{
    "org_id": "org-1",
	"id": "example",
	"runner_configuration": {
	  "type": "kubernetes",
      "job": {
	    "namespace": "default",
		"service_account": "platform-orchestrator-runner"
	  },
	  "cluster": {
		"cluster_data": {
			"server": "https://kubernetes.default.svc",
			"certificate_authority_data": ""
		},
		"auth": {}
	  }
	},
	"state_storage_configuration": {
	  "type": "kubernetes",
	  "namespace": "default"
	},
	"created_at": "0001-01-01T00:00:00Z",
	"updated_at": "0001-01-01T00:00:00Z"
}`, stdout)
	}
}

func TestUpdate_runner_not_found(t *testing.T) {
	orgId, cpc, _, ctx, fin := setupTestContext(t)
	defer fin()

	cpc.EXPECT().UpdateRunnerWithResponse(gomock.Any(), orgId, testMpId, cp.RunnerUpdateBody{Description: ref.Ref("something")}).Return(&cp.UpdateRunnerResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &cp.Error{Message: runnersTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, outFlag, jsonOutput, testUpdateCmd, listRunnerRulesRunnerFlag, testMpId, runnersTestUpdateSet})
	assert.EqualError(t, err, fmt.Sprintf("runner 'example' not found in org '%s'", orgId))
}
