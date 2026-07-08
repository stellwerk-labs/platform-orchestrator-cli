package command

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	mockdp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp/mocks"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	deployTestWorkloadName    = "test-sample"
	deployTestFooKey          = "foo"
	deployTestBarValue        = "bar"
	deployTestStatusSucceeded = "succeeded"
	deployTestStatusMessage   = "it was successful!"
)

func TestNominal_deploy_fresh(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	newDepId := uuid.New()

	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId: testProjectId,
			EnvId:     testEnvId,
			Mode:      dp.DeploymentCreateBodyModeDeploy,
			Manifest: &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{
				deployTestWorkloadName: {
					Outputs: map[string]string{
						deployTestFooKey: deployTestBarValue,
					},
				},
			}},
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
			JSON201:      &dp.Deployment{Id: newDepId},
		}, nil
	})

	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, newDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				OrgId:         orgId,
				Id:            newDepId,
				CompletedAt:   &time.Time{},
				Mode:          testDeployCmd,
				Status:        deployTestStatusSucceeded,
				StatusMessage: deployTestStatusMessage,
				Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{
					deployTestWorkloadName: {
						Outputs: map[string]string{
							deployTestFooKey: deployTestBarValue,
						},
					},
				}},
			},
		}, nil)

	lk := ctx.Value("logsAgeKey").(*age.X25519Identity)
	b := new(bytes.Buffer)
	{
		ak := ctx.Value("ageKey").(*age.X25519Identity)
		bw := base64.NewEncoder(base64.StdEncoding, b)
		ew, err := age.Encrypt(bw, ak.Recipient())
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(ew).Encode(map[string]interface{}{deployTestFooKey: deployTestBarValue}))
		require.NoError(t, ew.Close())
		require.NoError(t, bw.Close())
	}

	dpc.EXPECT().GetDeploymentEncryptedOutputsWithResponse(gomock.Any(), orgId, newDepId).Return(&dp.GetDeploymentEncryptedOutputsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.DeploymentEncryptedOutputs{
			Raw: b.String(),
		},
	}, nil)

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`
workloads:
  test-sample:
    outputs:
      foo: bar`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(), noPromptFlag, "--result", path.Join(os.TempDir(), "outputs.raw"), outFlag, jsonOutput})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
No previous stateful deployment exists for this environment.
Generating diff...
Manifest changes detected:
Add     /workloads/test-sample
Creating deploy deployment...
Deployment %[2]s created.
Once deployment is complete, logs will be available. To access logs use this secret key: %[4]s, e.g. 'octl logs %[2]s --key=%[4]s'
Waiting for deployment %[2]s to complete (0s)...
Deployment %[2]s succeeded after 0s: it was successful!
Retrieved outputs, writing to destination '%[5]s'
`, testApiUrl, newDepId, orgId, lk.String(), path.Join(os.TempDir(), "outputs.raw")), stderr)

	raw, err := os.ReadFile(path.Join(os.TempDir(), "outputs.raw"))
	require.NoError(t, err)
	assert.JSONEq(t, `{
  "foo": "bar"
	}`, string(raw))
}

func TestNominal_deploy_fresh_with_deprecated_variables(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	newDepId := uuid.New()

	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId: testProjectId,
			EnvId:     testEnvId,
			Mode:      dp.DeploymentCreateBodyModeDeploy,
			Manifest: &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{
				deployTestWorkloadName: {
					Variables: map[string]string{
						deployTestFooKey: deployTestBarValue,
					},
				},
			}},
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
			JSON201:      &dp.Deployment{Id: newDepId},
		}, nil
	})

	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, newDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				OrgId:         orgId,
				Id:            newDepId,
				CompletedAt:   &time.Time{},
				Mode:          testDeployCmd,
				Status:        deployTestStatusSucceeded,
				StatusMessage: deployTestStatusMessage,
				Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{
					deployTestWorkloadName: {
						Outputs: map[string]string{
							deployTestFooKey: deployTestBarValue,
						},
					},
				}},
			},
		}, nil)

	lk := ctx.Value("logsAgeKey").(*age.X25519Identity)
	b := new(bytes.Buffer)
	{
		ak := ctx.Value("ageKey").(*age.X25519Identity)
		bw := base64.NewEncoder(base64.StdEncoding, b)
		ew, err := age.Encrypt(bw, ak.Recipient())
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(ew).Encode(map[string]interface{}{deployTestFooKey: deployTestBarValue}))
		require.NoError(t, ew.Close())
		require.NoError(t, bw.Close())
	}

	dpc.EXPECT().GetDeploymentEncryptedOutputsWithResponse(gomock.Any(), orgId, newDepId).Return(&dp.GetDeploymentEncryptedOutputsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &dp.DeploymentEncryptedOutputs{
			Raw: b.String(),
		},
	}, nil)

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`
workloads:
  test-sample:
    variables:
      foo: bar`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(), noPromptFlag, "--result", path.Join(os.TempDir(), "outputs.raw"), outFlag, jsonOutput})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
No previous stateful deployment exists for this environment.
Generating diff...
Manifest changes detected:
Add     /workloads/test-sample
Creating deploy deployment...
Deployment %[2]s created.
Once deployment is complete, logs will be available. To access logs use this secret key: %[4]s, e.g. 'octl logs %[2]s --key=%[4]s'
Waiting for deployment %[2]s to complete (0s)...
Deployment %[2]s succeeded after 0s: it was successful!
Retrieved outputs, writing to destination '%[5]s'
`, testApiUrl, newDepId, orgId, lk.String(), path.Join(os.TempDir(), "outputs.raw")), stderr)

	raw, err := os.ReadFile(path.Join(os.TempDir(), "outputs.raw"))
	require.NoError(t, err)
	assert.JSONEq(t, `{
  "foo": "bar"
	}`, string(raw))
}

func TestNominal_deploy_second_plan(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	lastDepId := uuid.New()
	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: lastDepId}}},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, lastDepId).
		Return(&dp.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &dp.Deployment{Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}}},
		}, nil)

	newDepId := uuid.New()
	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId:      testProjectId,
			EnvId:          testEnvId,
			Mode:           dp.DeploymentCreateBodyModeDeploy,
			PlanOnly:       ref.Ref(true),
			Manifest:       &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
			JSON201:      &dp.Deployment{Id: newDepId},
		}, nil
	})

	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, newDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				Id:            newDepId,
				CompletedAt:   &time.Time{},
				Status:        deployTestStatusSucceeded,
				StatusMessage: deployTestStatusMessage,
			},
		}, nil)

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`{}`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(), planOnlyFlag, noPromptFlag, skipLogsFlag})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
Found previous stateful deployment %[1]s.
Generating diff...
No manifest changes detected - this will be a re-deployment.
Creating deploy deployment (plan only)...
Deployment %[2]s created.
Waiting for deployment %[2]s to complete (0s)...
Deployment %[2]s succeeded after 0s: it was successful!
Outputs ignored due to unset --result flag.
`, lastDepId, newDepId), stderr)
}

func TestNominal_deploy_dry_plan(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	lastDepId := uuid.New()
	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: lastDepId}}},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, lastDepId).
		Return(&dp.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &dp.Deployment{Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}}},
		}, nil)

	newDepId := uuid.New()
	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		assert.NotEmpty(t, bod.EncryptedLogsRecipient)
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId:      testProjectId,
			EnvId:          testEnvId,
			Manifest:       &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			Mode:           dp.DeploymentCreateBodyModeDeploy,
			PlanOnly:       ref.Ref(true),
			IsDryRun:       true,
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &dp.DeploymentDryRun{},
		}, nil
	})

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`{}`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(), planOnlyFlag, dryRunFlag, noPromptFlag})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
Found previous stateful deployment %[1]s.
Generating diff...
No manifest changes detected - this will be a re-deployment.
Creating deploy deployment (plan only)...
Dry-run deployment is valid.
`, lastDepId, newDepId), stderr)
}

func TestNominal_merge_deployment(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	lastDepId := uuid.New()
	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: lastDepId}}},
	}, nil)

	dpc.EXPECT().GetDeploymentWithResponse(gomock.Any(), orgId, lastDepId).
		Return(&dp.GetDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{Manifest: dp.DeploymentManifest{
				Workloads: map[string]dp.DeploymentManifestWorkload{
					"old":     {},
					"current": {},
				},
				Shared: map[string]dp.DeploymentManifestResource{
					"old-db":     {},
					"current-db": {},
				},
			}},
		}, nil)

	newDepId := uuid.New()
	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		assert.NotEmpty(t, bod.EncryptedLogsRecipient)
		bod.EncryptedOutputsRecipient = nil
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId: testProjectId,
			EnvId:     testEnvId,
			Manifest: &dp.DeploymentManifest{
				Workloads: map[string]dp.DeploymentManifestWorkload{
					"new":     {},
					"current": {},
				},
				Shared: map[string]dp.DeploymentManifestResource{
					"new-db":     {},
					"current-db": {},
				},
			},
			Mode:           dp.DeploymentCreateBodyModeDeploy,
			PlanOnly:       ref.Ref(true),
			IsDryRun:       true,
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &dp.DeploymentDryRun{},
		}, nil
	})

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(tf).Encode(dp.DeploymentManifest{
		Workloads: map[string]dp.DeploymentManifestWorkload{
			"new": {},
		},
		Shared: map[string]dp.DeploymentManifestResource{
			"new-db": {},
		},
	}))
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{
		orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(),
		planOnlyFlag, dryRunFlag, noPromptFlag,
		"--merge", "--drop-workload", "old", "--drop-shared", "old-db",
	})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
Found previous stateful deployment %[1]s.
Generating diff...
Manifest changes detected:
Copy    /shared/current-db -> /shared/new-db
Remove  /shared/old-db
Copy    /workloads/current -> /workloads/new
Remove  /workloads/old
Creating deploy deployment (plan only)...
Dry-run deployment is valid.
`, lastDepId, newDepId), stderr)
}

func TestNominal_deploy_with_show_logs(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	newDepId := uuid.New()

	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId:      testProjectId,
			EnvId:          testEnvId,
			Mode:           dp.DeploymentCreateBodyModeDeploy,
			Manifest:       &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
			JSON201:      &dp.Deployment{Id: newDepId},
		}, nil
	})

	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, newDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				Id:            newDepId,
				CompletedAt:   &time.Time{},
				Mode:          testDeployCmd,
				Status:        deployTestStatusSucceeded,
				StatusMessage: deployTestStatusMessage,
				Manifest:      dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			},
		}, nil)

	lk := ctx.Value("logsAgeKey").(*age.X25519Identity)
	logsText := "Deployment logs content"

	dpc.EXPECT().GetDeploymentLogsWithResponse(gomock.Any(), orgId, newDepId, &dp.GetDeploymentLogsParams{
		DecryptKey: ref.Ref(lk.String()),
	}).Return(&dp.GetDeploymentLogsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		Body:         []byte(logsText),
	}, nil)

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`{}`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(), noPromptFlag, "--show-logs"})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
No previous stateful deployment exists for this environment.
Generating diff...
No manifest changes detected - this will be a re-deployment.
Creating deploy deployment...
Deployment %[2]s created.
Once deployment is complete, logs will be available. To access logs use this secret key: %[4]s, e.g. 'octl logs %[2]s --key=%[4]s'
Waiting for deployment %[2]s to complete (0s)...

Runner logs:
Deployment logs content

Deployment %[2]s succeeded after 0s: it was successful!
Outputs ignored due to unset --result flag.
`, testApiUrl, newDepId, orgId, lk.String()), stderr)
}

func TestNominal_deploy_with_show_logs_error(t *testing.T) {
	orgId, cpc, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	color.NoColor = true

	cpc.EXPECT().GetProjectWithResponse(gomock.Any(), orgId, testProjectId).
		Return(&cp.GetProjectResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Project{Id: testProjectId, DisplayName: testProjectName},
		}, nil)

	cpc.EXPECT().GetEnvironmentWithResponse(gomock.Any(), orgId, testProjectId, testEnvId).
		Return(&cp.GetEnvironmentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId},
		}, nil)

	dpc.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil)

	newDepId := uuid.New()

	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		assert.NotEmpty(t, bod.EncryptedOutputsRecipient)
		bod.EncryptedOutputsRecipient = nil
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId:      testProjectId,
			EnvId:          testEnvId,
			Mode:           dp.DeploymentCreateBodyModeDeploy,
			Manifest:       &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
			JSON201:      &dp.Deployment{Id: newDepId},
		}, nil
	})

	dpc.EXPECT().WaitForDeploymentCompleteWithResponse(gomock.Any(), orgId, newDepId, &dp.WaitForDeploymentCompleteParams{}).
		Return(&dp.WaitForDeploymentCompleteResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200: &dp.Deployment{
				Id:            newDepId,
				CompletedAt:   &time.Time{},
				Mode:          testDeployCmd,
				Status:        deployTestStatusSucceeded,
				StatusMessage: deployTestStatusMessage,
				Manifest:      dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			},
		}, nil)

	lk := ctx.Value("logsAgeKey").(*age.X25519Identity)
	errorMessage := `{"error":"HTTP-404","message":"logs for deployment not found"}`

	// Test the case where the logs API call fails
	dpc.EXPECT().GetDeploymentLogsWithResponse(gomock.Any(), orgId, newDepId, &dp.GetDeploymentLogsParams{
		DecryptKey: ref.Ref(lk.String()),
	}).Return(&dp.GetDeploymentLogsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusBadRequest},
		Body:         []byte(errorMessage),
	}, nil)

	tf, err := os.CreateTemp(os.TempDir(), "manifest-*.yaml")
	require.NoError(t, err)
	_, _ = tf.WriteString(`{}`)
	require.NoError(t, tf.Close())

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, testDeployCmd, testProjectId, testEnvId, tf.Name(), noPromptFlag, "--show-logs"})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loaded manifest.
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
No previous stateful deployment exists for this environment.
Generating diff...
No manifest changes detected - this will be a re-deployment.
Creating deploy deployment...
Deployment %[2]s created.
Once deployment is complete, logs will be available. To access logs use this secret key: %[4]s, e.g. 'octl logs %[2]s --key=%[4]s'
Waiting for deployment %[2]s to complete (0s)...

Runner logs:
failed to get logs: %[5]s
Deployment %[2]s succeeded after 0s: it was successful!
Outputs ignored due to unset --result flag.
`, testApiUrl, newDepId, orgId, lk.String(), errorMessage), stderr)
}

func Test_loadRawManifest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	dpClient := mockdp.NewMockClientWithResponsesInterface(ctrl)

	depId := uuid.New()
	unknownDepId := uuid.New()

	dpClient.EXPECT().GetDeploymentWithResponse(gomock.Any(), "my-org", depId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.Deployment{Manifest: dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}}},
	}, nil).AnyTimes()
	dpClient.EXPECT().GetDeploymentWithResponse(gomock.Any(), "my-org", unknownDepId).Return(&dp.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
	}, nil).AnyTimes()
	dpClient.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), "my-org", &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref(testEnvId),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{{Id: depId}}},
	}, nil).AnyTimes()
	dpClient.EXPECT().ListLastDeploymentsWithResponse(gomock.Any(), "my-org", &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(testProjectId),
		EnvId:           ref.Ref("unknown"),
		StateChangeOnly: ref.Ref(true),
	}).Return(&dp.ListLastDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.DeploymentPage{Items: []dp.DeploymentSummary{}},
	}, nil).AnyTimes()

	t.Run("stdin", func(t *testing.T) {
		buff := bytes.NewBufferString(`{}`)
		raw, err := loadRawManifest(t.Context(), buff, "-", dpClient, "my-org", testProjectId, testEnvId)
		require.NoError(t, err)
		assert.JSONEq(t, "{}", string(raw))
	})

	t.Run("deployment by id", func(t *testing.T) {
		raw, err := loadRawManifest(t.Context(), nil, "deployment://"+depId.String(), dpClient, "my-org", testProjectId, testEnvId)
		require.NoError(t, err)
		assert.JSONEq(t, "{\"workloads\":{}}", string(raw))
	})

	t.Run("deployment by unknown id", func(t *testing.T) {
		_, err := loadRawManifest(t.Context(), nil, "deployment://"+unknownDepId.String(), dpClient, "my-org", testProjectId, testEnvId)
		assert.EqualError(t, err, "deployment "+unknownDepId.String()+" not found")
	})

	t.Run("deployment by env", func(t *testing.T) {
		raw, err := loadRawManifest(t.Context(), nil, "environment://my-env", dpClient, "my-org", testProjectId, testEnvId)
		require.NoError(t, err)
		assert.JSONEq(t, "{\"workloads\":{}}", string(raw))
	})

	t.Run("deployment by unknown env", func(t *testing.T) {
		_, err := loadRawManifest(t.Context(), nil, "environment://unknown", dpClient, "my-org", testProjectId, testEnvId)
		assert.EqualError(t, err, "no deployments found for environment 'unknown' - does it exist?")
	})

	t.Run("deployment by head", func(t *testing.T) {
		raw, err := loadRawManifest(t.Context(), nil, "deployment://HEAD", dpClient, "my-org", testProjectId, testEnvId)
		require.NoError(t, err)
		assert.JSONEq(t, "{\"workloads\":{}}", string(raw))
	})
}

func TestDeploy_FlagManagement(t *testing.T) {

	t.Run("deprecated --format flag still supported", func(t *testing.T) {
		resetCommandFlags(DeployCmd)

		err := DeployCmd.ParseFlags([]string{formatFlag, "format-flag-value"})
		require.NoError(t, err)

		result := GetFlagWithFallback(DeployCmd, deployCmdFormatFlag, deprecatedDeployCmdFormatFlag)

		assert.Equal(t, "format-flag-value", result)
	})

	t.Run("--out flag takes precedence over --format", func(t *testing.T) {
		resetCommandFlags(DeployCmd)

		err := DeployCmd.ParseFlags([]string{outFlag, "out-flag-value", formatFlag, "format-flag-value"})
		require.NoError(t, err)

		result := GetFlagWithFallback(DeployCmd, deployCmdFormatFlag, deprecatedDeployCmdFormatFlag)

		assert.Equal(t, "out-flag-value", result)
	})

	t.Run("returns default value (yaml) when neither flag set", func(t *testing.T) {
		resetCommandFlags(DeployCmd)

		err := DeployCmd.ParseFlags([]string{})
		require.NoError(t, err)

		result := GetFlagWithFallback(DeployCmd, deployCmdFormatFlag, deprecatedDeployCmdFormatFlag)
		assert.Equal(t, "yaml", result)
	})

	t.Run("using deprecated --format flag shows warning", func(t *testing.T) {
		resetCommandFlags(DeployCmd)

		var stdout bytes.Buffer
		DeployCmd.SetOut(&stdout)

		err := DeployCmd.ParseFlags([]string{formatFlag, jsonOutput})
		require.NoError(t, err)

		deprecationWarning := "Flag --format has been deprecated, use --out instead."
		assert.Contains(t, stdout.String(), deprecationWarning)
	})

	t.Run("using deprecated --output flag shows warning", func(t *testing.T) {
		resetCommandFlags(DeployCmd)

		var stdout bytes.Buffer
		DeployCmd.SetOut(&stdout)

		err := DeployCmd.ParseFlags([]string{"--output", "/tmp/test.out"})
		require.NoError(t, err)

		deprecationWarning := "Flag --output has been deprecated, use --result instead."
		assert.Contains(t, stdout.String(), deprecationWarning)
	})
}
