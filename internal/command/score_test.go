package command

import (
	"context"
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
	"github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	scoreTestOtherAlias      = "other"
	scoreTestThingName       = "thing"
	scoreTestWorkloadName    = "test-sample"
	scoreTestMetadataParam   = "metadata"
	scoreTestContainersParam = "containers"
	scoreTestServiceParam    = "service"
	scoreTestStatusSucceeded = "succeeded"
	scoreTestStatusMessage   = "it was successful!"
	scoreTestCmd             = "score"
)

func TestScoreDeploy(t *testing.T) {
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
					scoreTestOtherAlias: {},
				},
				Shared: map[string]dp.DeploymentManifestResource{
					scoreTestThingName: {
						Type: scoreTestThingName,
					},
				},
			}},
		}, nil)

	newDepId := uuid.New()
	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		bod.EncryptedOutputsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId: testProjectId,
			EnvId:     testEnvId,
			Mode:      dp.DeploymentCreateBodyModePlanOnly,
			IsDryRun:  true,
			Manifest: &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{
				scoreTestWorkloadName: {
					Resources: map[string]dp.DeploymentManifestResource{
						"db": {Type: testArtResourceTypeId},
						scoreDeployDefaultResourceType: {
							Type: scoreDeployDefaultResourceType,
							Params: map[string]interface{}{
								scoreTestMetadataParam: types.WorkloadMetadata{"name": scoreTestWorkloadName},
								scoreTestContainersParam: types.WorkloadContainers{
									"one": {
										Image: "alpine:latest",
										Variables: map[string]string{
											"ONE":   scoreTestWorkloadName,
											"TWO":   "${resources.db.outputs.id}",
											"THREE": "${context.env_id}",
										},
										Files: map[string]types.ContainerFile{
											"/etc/test": {Content: ref.Ref("test test-sample")},
										},
									},
								},
								scoreTestServiceParam: types.WorkloadService{
									Ports: map[string]types.ServicePort{"web": {Port: 8080}},
								},
							},
						},
						"specific": {
							Type: testArtResourceTypeId,
							Id:   ref.Ref("shared.common-db"),
						},
						scoreTestOtherAlias: {
							Type: scoreTestThingName,
							Params: map[string]interface{}{
								"id": "${resources.db.outputs.id}",
							},
						},
					},
				},
				scoreTestOtherAlias: {},
			}, Shared: map[string]dp.DeploymentManifestResource{
				scoreTestThingName: {
					Type: scoreTestThingName,
				},
			}},
		}, bod)
		return &dp.CreateDeploymentResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &dp.DeploymentDryRun{},
		}, nil
	})

	td, err := os.MkdirTemp(os.TempDir(), "test-deploy-*")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(td+"/score.yaml", []byte(`apiVersion: score.dev/v1b1
metadata:
  name: test-sample
containers:
  one:
    image: alpine:latest
    variables:
      ONE: ${metadata.name}
      TWO: ${resources.db.id}
      THREE: $${context.env_id}
    files:
      /etc/test:
        source: test.txt
service:
  ports:
    web:
      port: 8080
resources:
  db:
    type: postgres
  other:
    type: thing
    params:
      id: ${resources.db.id}
  specific:
    type: postgres
    id: common-db
`), 0600))
	require.NoError(t, os.WriteFile(td+"/test.txt", []byte("test ${metadata.name}"), 0600))

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, scoreTestCmd, testDeployCmd, testProjectId, testEnvId, td + "/score.yaml", planOnlyFlag, dryRunFlag, noPromptFlag})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loading score file '%[3]s'
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
Found previous stateful deployment %[1]s.
Generating diff...
Manifest changes detected:
Add     /workloads/test-sample
Creating plan_only deployment...
Dry-run deployment is valid.
`, lastDepId, newDepId, td+"/score.yaml"), stderr)
}

func TestScoreDeploy_custom_class(t *testing.T) {
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
				Workloads: map[string]dp.DeploymentManifestWorkload{},
			}},
		}, nil)

	newDepId := uuid.New()
	dpc.EXPECT().CreateDeploymentWithResponse(gomock.Any(), orgId, gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, orgId string, params *dp.CreateDeploymentParams, bod dp.DeploymentCreateBody, _ ...dp.RequestEditorFn) (*dp.CreateDeploymentResponse, error) {
		assert.NotEmpty(t, params.IdempotencyKey)
		bod.EncryptedOutputsRecipient = nil
		bod.EncryptedLogsRecipient = nil
		assert.Equal(t, dp.DeploymentCreateBody{
			ProjectId:      testProjectId,
			EnvId:          testEnvId,
			Mode:           dp.DeploymentCreateBodyModePlanOnly,
			RunnerLogLevel: ref.Ref(dp.DeploymentCreateBodyRunnerLogLevel("info")),
			Manifest: &dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{
				scoreTestWorkloadName: {
					Resources: map[string]dp.DeploymentManifestResource{
						scoreDeployDefaultResourceType: {
							Type:  scoreDeployDefaultResourceType,
							Class: ref.Ref("ha"),
							Id:    ref.Ref("main"),
							Params: map[string]interface{}{
								scoreTestMetadataParam: types.WorkloadMetadata{
									"name": scoreTestWorkloadName,
									"annotations": map[string]interface{}{
										scoreAnnotationResClass: "ha",
										scoreAnnotationResId:    "main",
									},
								},
								scoreTestContainersParam: types.WorkloadContainers{
									"one": {
										Image: "alpine:latest",
									},
								},
								scoreTestServiceParam: types.WorkloadService{},
							},
						},
					},
				},
			}},
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
				Status:        scoreTestStatusSucceeded,
				StatusMessage: scoreTestStatusMessage,
				Manifest:      dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{}},
			},
		}, nil)
	lk := ctx.Value("logsAgeKey").(*age.X25519Identity)

	td, err := os.MkdirTemp(os.TempDir(), "test-deploy-*")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(td+"/score.yaml", []byte(`apiVersion: score.dev/v1b1
metadata:
  name: test-sample
  annotations:
    platform-orchestrator.dev/resClass: ha
    platform-orchestrator.dev/resId: main
containers:
  one:
    image: alpine:latest
`), 0600))

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, scoreTestCmd, testDeployCmd, testProjectId, testEnvId, td + "/score.yaml", planOnlyFlag, noPromptFlag})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loading score file '%[5]s'
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
Found previous stateful deployment %[1]s.
Generating diff...
Manifest changes detected:
Add     /workloads/test-sample
Creating plan_only deployment...
Deployment %[2]s created.
Once deployment is complete, logs will be available. To access logs use this secret key: %[4]s, e.g. 'octl logs %[2]s --key=%[4]s'
Waiting for deployment %[2]s to complete (0s)...
Deployment %[2]s succeeded after 0s: it was successful!
`, lastDepId, newDepId, orgId, lk.String(), td+"/score.yaml", testApiUrl), stderr)
}

func TestScoreDeploy_print_manifest_file(t *testing.T) {
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
					scoreTestOtherAlias: {},
				},
				Shared: map[string]dp.DeploymentManifestResource{
					scoreTestThingName: {
						Type: scoreTestThingName,
					},
				},
			}},
		}, nil)

	td, err := os.MkdirTemp(os.TempDir(), "test-deploy-*")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(td+"/score.yaml", []byte(`apiVersion: score.dev/v1b1
metadata:
  name: test-sample
containers:
  one:
    image: alpine:latest
    variables:
      ONE: ${metadata.name}
      TWO: ${resources.db.id}
      THREE: $${context.env_id}
    files:
      /etc/test:
        source: test.txt
service:
  ports:
    web:
      port: 8080
resources:
  db:
    type: postgres
  other:
    type: thing
    params:
      id: ${resources.db.id}
  specific:
    type: postgres
    id: common-db
`), 0600))
	require.NoError(t, os.WriteFile(td+"/test.txt", []byte("test ${metadata.name}"), 0600))

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgId, scoreTestCmd, testDeployCmd, testProjectId, testEnvId, td + "/score.yaml", "--print-manifest", path.Join(os.TempDir(), "manifest_from_score.yaml")})
	require.NoError(t, err)
	stderr = regexp.MustCompile(`to complete \(.+?\)...`).ReplaceAllString(stderr, "to complete (0s)...")
	assert.Equal(t, fmt.Sprintf(`Loading score file '%[2]s'
Checking project 'my-project' and environment 'my-env' exist...
Project 'My Project' (my-project) exists.
Environment 'My Env' (my-env type=my-et) exists.
Checking for last deployment...
Found previous stateful deployment %[1]s.
Deployment manifest produced by score manifest[s], writing to destination '%[3]s'
`, lastDepId, td+"/score.yaml", path.Join(os.TempDir(), "manifest_from_score.yaml")), stderr)
	deploymentManifest, err := os.ReadFile(path.Join(os.TempDir(), "manifest_from_score.yaml"))
	require.NoError(t, err)
	assert.YAMLEq(t, `shared:
    thing:
        class: null
        id: null
        params: {}
        type: thing
workloads:
    other:
        resources: {}
        outputs: {}
    test-sample:
        resources:
            db:
                class: null
                id: null
                params: {}
                type: postgres
            other:
                class: null
                id: null
                params:
                    id: ${resources.db.outputs.id}
                type: thing
            score-workload:
                class: null
                id: null
                params:
                    containers:
                        one:
                            files:
                                /etc/test:
                                    content: test test-sample
                            image: alpine:latest
                            variables:
                                ONE: test-sample
                                THREE: ${context.env_id}
                                TWO: ${resources.db.outputs.id}
                    metadata:
                        name: test-sample
                    service:
                        ports:
                            web:
                                port: 8080
                type: score-workload
            specific:
                class: null
                id: shared.common-db
                params: {}
                type: postgres
        outputs: {}
`, string(deploymentManifest))
}
