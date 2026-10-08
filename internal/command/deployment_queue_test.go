package command

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/config"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	queueTestOrg            = "queue-org"
	queueTestBusyError      = `{"error":"environment_busy","message":"deployment is still executing"}`
	queueTestRollback       = "rollback"
	queueTestItemID         = "22222222-2222-4222-8222-222222222222"
	queueTestWorkload       = "running"
	queueTestDeploymentsCmd = "deployments"
	queueTestTTLFlag        = "--" + deployCmdQueueTTLFlag
	queueTestNoQueueFlag    = "--" + deployCmdNoQueueFlag
	queueTestMergeFlag      = "--" + deployCmdMergeFlag
	queueTestScoreFile      = "apiVersion: score.dev/v1b1\nmetadata:\n  name: web\ncontainers:\n  main:\n    image: alpine:latest\n"
)

// deployQueueServer is a Data Plane and Control Plane fake that speaks the
// vendored contracts to the real generated clients. It serves one Environment
// whose current stateful Deployment is base.
type deployQueueServer struct {
	t  *testing.T
	mu sync.Mutex

	base         *dp.DeploymentSummary
	baseManifest dp.DeploymentManifest
	// createStatus and createBody answer POST /deployments.
	createStatus int
	createBody   any
	// queueReads answer successive reads of the queue item; the last repeats.
	queueReads []dp.DeploymentQueueItem
	deployment dp.Deployment
	// ackStatus and ackBody, when set, answer acknowledge-outcome.
	ackStatus int
	ackBody   any

	creates      []dp.DeploymentCreateBody
	queueQueries []string
	cancels      []dp.DeploymentQueueCancelBody
	acks         []string
}

func newDeployQueueServer(t *testing.T) (*deployQueueServer, context.Context) {
	t.Helper()
	fixture := &deployQueueServer{t: t, deployment: dp.Deployment{
		Id: uuid.New(), OrgId: queueTestOrg, ProjectId: testProjectId, EnvId: testEnvId,
		Mode: testDeployCmd, Status: "executing", CreatedAt: time.Now(),
	}}
	server := httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(server.Close)

	cpc, err := cp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	dpc, err := dp.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	ageKey, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), ConfigContextKey, config.Config{DefaultOrg: queueTestOrg})
	ctx = context.WithValue(ctx, CpClientContextKey, cpc)
	ctx = context.WithValue(ctx, DpClientContextKey, dpc)
	//nolint
	ctx = context.WithValue(ctx, "ageKey", ageKey)
	//nolint
	ctx = context.WithValue(ctx, "logsAgeKey", ageKey)

	t.Setenv("PO_API_URL", server.URL)
	cobra.EnableTraverseRunHooks = true
	color.NoColor = true
	previousInterval := deploymentQueuePollInterval
	deploymentQueuePollInterval = time.Millisecond
	t.Cleanup(func() { deploymentQueuePollInterval = previousInterval })
	return fixture, ctx
}

func (f *deployQueueServer) withBase(status string) uuid.UUID {
	f.base = &dp.DeploymentSummary{Id: uuid.New(), OrgId: queueTestOrg, ProjectId: testProjectId, EnvId: testEnvId, Mode: testDeployCmd, Status: status, CreatedAt: time.Now()}
	f.baseManifest = dp.DeploymentManifest{Workloads: map[string]dp.DeploymentManifestWorkload{queueTestWorkload: {}}}
	return f.base.Id
}

func (f *deployQueueServer) queueItem(state dp.DeploymentQueueItemState, position int) dp.DeploymentQueueItem {
	item := dp.DeploymentQueueItem{
		Id: uuid.MustParse(queueTestItemID), OrgId: queueTestOrg, ProjectId: testProjectId, EnvId: testEnvId,
		Mode: dp.DeploymentQueueItemModeDeploy, State: state, Position: position, CreatedBy: uuid.New(),
		CreatedAt: time.Now(), ExpiresAt: time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC),
	}
	if state == dp.Dispatched {
		item.DeploymentId = ref.Ref(f.deployment.Id)
	}
	return item
}

func (f *deployQueueServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	org := "/orgs/" + queueTestOrg
	write := func(status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if raw, ok := value.(string); ok {
			_, _ = w.Write([]byte(raw))
			return
		}
		assert.NoError(f.t, json.NewEncoder(w).Encode(value))
	}
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == org+"/projects/"+testProjectId:
		write(http.StatusOK, cp.Project{Id: testProjectId, DisplayName: testProjectName})
	case r.Method == http.MethodGet && path == org+"/projects/"+testProjectId+"/envs/"+testEnvId:
		write(http.StatusOK, cp.Environment{Id: testEnvId, DisplayName: testEnvName, EnvTypeId: testEnvTypeId})
	case r.Method == http.MethodGet && path == org+"/last-deployments":
		assert.Equal(f.t, "true", r.URL.Query().Get("state_change_only"))
		page := dp.DeploymentPage{Items: []dp.DeploymentSummary{}}
		if f.base != nil {
			page.Items = append(page.Items, *f.base)
		}
		write(http.StatusOK, page)
	case r.Method == http.MethodPost && path == org+"/deployments":
		var body dp.DeploymentCreateBody
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		f.creates = append(f.creates, body)
		write(f.createStatus, f.createBody)
	case r.Method == http.MethodGet && path == org+"/deployment-queue":
		f.queueQueries = append(f.queueQueries, r.URL.RawQuery)
		write(http.StatusOK, dp.DeploymentQueueItemPage{Items: f.queueReads})
	case r.Method == http.MethodGet && strings.HasPrefix(path, org+"/deployment-queue/"):
		item := f.queueReads[0]
		if len(f.queueReads) > 1 {
			f.queueReads = f.queueReads[1:]
		}
		write(http.StatusOK, item)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/actions/cancel"):
		var body dp.DeploymentQueueCancelBody
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		f.cancels = append(f.cancels, body)
		write(f.createStatus, f.createBody)
	case r.Method == http.MethodGet && f.base != nil && path == org+"/deployments/"+f.base.Id.String():
		write(http.StatusOK, dp.Deployment{Id: f.base.Id, ProjectId: testProjectId, EnvId: testEnvId, Manifest: f.baseManifest})
	case r.Method == http.MethodGet && path == org+"/deployments/"+f.deployment.Id.String():
		write(http.StatusOK, f.deployment)
	case r.Method == http.MethodGet && path == org+"/deployments/"+f.deployment.Id.String()+"/actions/wait-for-complete":
		completed := f.deployment
		completed.Status = deployTestStatusSucceeded
		completed.CompletedAt = ref.Ref(completed.CreatedAt.Add(time.Second))
		write(http.StatusOK, completed)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/actions/acknowledge-outcome"):
		var body cp.DeploymentOutcomeAcknowledgementBody
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		f.acks = append(f.acks, path+" "+body.Reason)
		if f.ackStatus != 0 {
			write(f.ackStatus, f.ackBody)
			return
		}
		write(http.StatusCreated, cp.DeploymentOutcomeAcknowledgement{DeploymentId: f.deployment.Id, Reason: body.Reason, PluginId: progressiveRolloutsPluginID})
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, path)
		write(http.StatusNotFound, `{"error":"not_found","message":"not found"}`)
	}
}

func (f *deployQueueServer) createdBodies() []dp.DeploymentCreateBody {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates
}

func writeQueueTestManifest(t *testing.T) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("workloads:\n  web: {}\n"), 0o600))
	return manifestPath
}

func deployQueueArgs(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{orgFlag, queueTestOrg, testDeployCmd, testProjectId, testEnvId, writeQueueTestManifest(t)}, extra...)
}

func TestDeployWaitsInTheQueueByDefaultAndFollowsTheDispatchedDeployment(t *testing.T) {
	fixture, ctx := newDeployQueueServer(t)
	baseID := fixture.withBase("executing")
	fixture.createStatus = http.StatusAccepted
	fixture.createBody = fixture.queueItem(dp.Queued, 2)
	transient := fixture.queueItem(dp.Queued, 1)
	transient.Attempts, transient.LastError = 1, ref.Ref("Core is unavailable")
	fixture.queueReads = []dp.DeploymentQueueItem{fixture.queueItem(dp.Queued, 2), fixture.queueItem(dp.Queued, 1), transient, fixture.queueItem(dp.Dispatched, 0)}

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag))
	require.NoError(t, err)

	bodies := fixture.createdBodies()
	require.Len(t, bodies, 1)
	require.Equal(t, ref.Ref(dp.Queue), bodies[0].WhenBusy)
	require.Nil(t, bodies[0].QueueTtlSeconds)
	require.Nil(t, bodies[0].ExpectedBaseDeploymentId, "an unprompted full manifest is not derived from the base")

	for _, line := range []string{
		"Deployment " + baseID.String() + " is still executing; this request will wait in the environment's deployment queue.",
		"Environment is busy: request queued as item 22222222-2222-4222-8222-222222222222 at position 2, expires at 2026-10-07T14:00:00Z.",
		"Waiting in the deployment queue at position 1 (",
		"Starting the queued request failed transiently (attempt 1), retrying: Core is unavailable",
		"Queue item 22222222-2222-4222-8222-222222222222 dispatched as deployment " + fixture.deployment.Id.String() + ".",
		"Deployment " + fixture.deployment.Id.String() + " created.",
		"Deployment " + fixture.deployment.Id.String() + " succeeded after 1s",
	} {
		require.Contains(t, stderr, line)
	}
	require.Equal(t, 1, strings.Count(stderr, "at position 1 ("), "progress is printed when the position changes, not on every read")
}

func TestDeployNoQueueKeepsTheFailFastBehaviour(t *testing.T) {
	t.Run("executing base", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		baseID := fixture.withBase("executing")
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, queueTestNoQueueFlag))
		require.ErrorContains(t, err, "last deployment "+baseID.String()+" is still executing")
		require.ErrorContains(t, err, "retry once it finished, or run the command without --no-queue to wait in the environment's deployment queue")
		require.Empty(t, fixture.createdBodies())
	})
	t.Run("busy environment", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.withBase(deployTestStatusSucceeded)
		fixture.createStatus, fixture.createBody = http.StatusConflict, queueTestBusyError
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, queueTestNoQueueFlag))
		require.ErrorContains(t, err, "conflict (environment_busy): deployment is still executing. Retry once the running deployment finished")
		require.ErrorContains(t, err, "or run the command without --no-queue to wait in the environment's deployment queue")
		bodies := fixture.createdBodies()
		require.Len(t, bodies, 1)
		require.Equal(t, ref.Ref(dp.Reject), bodies[0].WhenBusy, "the Data Plane queues a request without when_busy")
	})
}

// The Data Plane queues a busy deploy or rollback that omits when_busy, so
// --no-queue must ask for reject explicitly, on every command that deploys.
func TestNoQueueSendsRejectFromEveryDeployingCommand(t *testing.T) {
	for name, args := range map[string]func(*testing.T, *deployQueueServer) []string{
		"deploy": func(t *testing.T, _ *deployQueueServer) []string {
			return deployQueueArgs(t, noPromptFlag, queueTestNoQueueFlag)
		},
		"rollback": func(_ *testing.T, f *deployQueueServer) []string {
			return []string{orgFlag, queueTestOrg, queueTestRollback, testProjectId, testEnvId, f.deployment.Id.String(), noPromptFlag, queueTestNoQueueFlag}
		},
		"score deploy": func(t *testing.T, _ *deployQueueServer) []string {
			return []string{orgFlag, queueTestOrg, scoreTestCmd, testDeployCmd, testProjectId, testEnvId, writeTestFile(t, "score.yaml", queueTestScoreFile), noPromptFlag, queueTestNoQueueFlag}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ctx := newDeployQueueServer(t)
			fixture.withBase(deployTestStatusSucceeded)
			fixture.createStatus, fixture.createBody = http.StatusCreated, fixture.deployment
			_, _, err := executeAndResetCommand(ctx, RootCmd, args(t, fixture))
			require.NoError(t, err)
			bodies := fixture.createdBodies()
			require.Len(t, bodies, 1)
			require.Equal(t, ref.Ref(dp.Reject), bodies[0].WhenBusy)
			require.Nil(t, bodies[0].QueueTtlSeconds)
		})
	}
}

// A request that sent queue and still got environment_busy was authorized by
// an organization token; a plan-only request never waits. Each error says why
// the request did not wait and what to do instead.
func TestEnvironmentBusyNamesWhyTheRequestDidNotWait(t *testing.T) {
	t.Run("organization token", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.withBase(deployTestStatusSucceeded)
		fixture.createStatus, fixture.createBody = http.StatusConflict, `{"error":"environment_busy","message":"the Environment is busy; queueing a Deployment requires an attributable user, not an organization token"}`
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag))
		require.EqualError(t, err, "conflict (environment_busy): the Environment is busy; queueing a Deployment requires an attributable user, not an organization token. "+
			"Requests authorized by an organization token cannot wait in the deployment queue; retry once the running deployment finished, or authenticate as a user ('octl login') or service user to wait")
		require.Equal(t, ref.Ref(dp.Queue), fixture.createdBodies()[0].WhenBusy)
	})
	t.Run("plan-only request beside a running plan", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.withBase(deployTestStatusSucceeded)
		fixture.createStatus, fixture.createBody = http.StatusConflict, queueTestBusyError
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, planOnlyFlag))
		require.EqualError(t, err, "conflict (environment_busy): deployment is still executing. "+
			"Plan-only requests never wait in the deployment queue and do not run beside another deployment of the environment; retry once it finished")
		require.Nil(t, fixture.createdBodies()[0].WhenBusy)
	})
	t.Run("plan-only request beside an executing base", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		baseID := fixture.withBase("executing")
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, planOnlyFlag))
		require.ErrorContains(t, err, "last deployment "+baseID.String()+" is still executing")
		require.Empty(t, fixture.createdBodies())
	})
}

func TestDeployQueueFlags(t *testing.T) {
	t.Run("ttl is sent in seconds", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.createStatus, fixture.createBody = http.StatusCreated, fixture.deployment
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, queueTestTTLFlag, "90m"))
		require.NoError(t, err)
		require.Equal(t, ref.Ref(5400), fixture.createdBodies()[0].QueueTtlSeconds)
	})
	for name, args := range map[string][]string{
		"sub-second ttl":     {queueTestTTLFlag, "1500ms"},
		"zero ttl":           {queueTestTTLFlag, "0s"},
		"ttl without queue":  {queueTestTTLFlag, "1m", queueTestNoQueueFlag},
		"ttl with plan-only": {queueTestTTLFlag, "1m", "--plan-only"},
		"ttl with dry-run":   {queueTestTTLFlag, "1m", "--dry-run"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ctx := newDeployQueueServer(t)
			_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, append([]string{noPromptFlag}, args...)...))
			require.Error(t, err)
			require.Empty(t, fixture.createdBodies())
		})
	}
	t.Run("plan-only and dry runs never queue", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.createStatus, fixture.createBody = http.StatusOK, dp.DeploymentDryRun{}
		_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, "--plan-only", "--dry-run"))
		require.NoError(t, err)
		require.Nil(t, fixture.createdBodies()[0].WhenBusy)
	})
}

func TestDeployDerivedManifestsNameTheirBase(t *testing.T) {
	for name, args := range map[string][]string{
		"merge":          {noPromptFlag, queueTestMergeFlag},
		"drop workload":  {noPromptFlag, "--drop-workload", queueTestWorkload},
		"drop shared":    {noPromptFlag, "--drop-shared", "db"},
		"prompted diff":  {},
		"merge prompted": {queueTestMergeFlag},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ctx := newDeployQueueServer(t)
			baseID := fixture.withBase(deployTestStatusSucceeded)
			fixture.createStatus, fixture.createBody = http.StatusConflict, `{"error":"deployment_base_changed","message":"the Environment's current Deployment is no longer the expected base"}`
			RootCmd.SetIn(strings.NewReader("\n"))
			t.Cleanup(func() { RootCmd.SetIn(nil) })

			_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, args...))
			require.ErrorContains(t, err, "conflict (deployment_base_changed)")
			require.ErrorContains(t, err, "derived from deployment "+baseID.String())
			require.ErrorContains(t, err, "run the command again to review the diff against the current state")
			bodies := fixture.createdBodies()
			require.Len(t, bodies, 1)
			require.Equal(t, ref.Ref(baseID), bodies[0].ExpectedBaseDeploymentId)
			require.Equal(t, ref.Ref(dp.Queue), bodies[0].WhenBusy)
		})
	}
}

func TestQueuedDeploymentThatNeverStartsFailsWithItsReason(t *testing.T) {
	for name, tc := range map[string]struct {
		state   dp.DeploymentQueueItemState
		code    string
		message string
		want    []string
	}{
		"base changed": {dp.Rejected, errorCodeDeploymentBaseChanged, "the Environment's current Deployment is no longer the expected base",
			[]string{"was rejected (deployment_base_changed)", "Another deployment changed the environment while this request waited"}},
		"expired": {dp.Expired, "expired", "", []string{"expired at 2026-10-07T14:00:00Z", queueTestTTLFlag}},
		"cancelled": {dp.Cancelled, "cancelled", "superseded by release 42",
			[]string{"queue item 22222222-2222-4222-8222-222222222222 was cancelled (cancelled): superseded by release 42"}},
		"rejected": {dp.Rejected, "pin_protection", "module is pinned", []string{"was rejected (pin_protection): module is pinned"}},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, ctx := newDeployQueueServer(t)
			baseID := fixture.withBase(deployTestStatusSucceeded)
			fixture.createStatus, fixture.createBody = http.StatusAccepted, fixture.queueItem(dp.Queued, 1)
			settled := fixture.queueItem(tc.state, 0)
			settled.ReasonCode, settled.ReasonMessage = ref.Ref(tc.code), ref.RefStringEmptyNil(tc.message)
			fixture.queueReads = []dp.DeploymentQueueItem{settled}

			_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag, queueTestMergeFlag))
			for _, want := range tc.want {
				require.ErrorContains(t, err, want)
			}
			require.Equal(t, ref.Ref(baseID), fixture.createdBodies()[0].ExpectedBaseDeploymentId)
		})
	}
}

func TestDeployConflictsNameTheNextStep(t *testing.T) {
	for code, want := range map[string]string{
		errorCodeDeploymentQueueFull: "'octl get deployment-queue-items my-project my-env --state queued'",
		errorCodeOutcomeUnreconciled: "'octl deployments acknowledge-outcome <deployment-id> --reason <text>'",
		"environment_deleting":       "conflict (environment_deleting): refused",
	} {
		t.Run(code, func(t *testing.T) {
			fixture, ctx := newDeployQueueServer(t)
			fixture.createStatus, fixture.createBody = http.StatusConflict, `{"error":"`+code+`","message":"refused"}`
			_, _, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag))
			require.ErrorContains(t, err, "conflict ("+code+"): refused")
			require.ErrorContains(t, err, want)
		})
	}
}

func TestRollbackWaitsInTheQueueByDefault(t *testing.T) {
	fixture, ctx := newDeployQueueServer(t)
	baseID := fixture.withBase(deployTestStatusSucceeded)
	target := fixture.deployment.Id
	fixture.createStatus = http.StatusAccepted
	queued := fixture.queueItem(dp.Queued, 1)
	queued.Mode, queued.RollbackToDeploymentId = dp.DeploymentQueueItemModeRollback, ref.Ref(target)
	fixture.createBody = queued
	fixture.queueReads = []dp.DeploymentQueueItem{fixture.queueItem(dp.Dispatched, 0)}
	RootCmd.SetIn(strings.NewReader("\n"))
	t.Cleanup(func() { RootCmd.SetIn(nil) })

	_, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, queueTestRollback, testProjectId, testEnvId, target.String(), queueTestTTLFlag, "10m"})
	require.NoError(t, err)
	require.Contains(t, stderr, "dispatched as deployment "+fixture.deployment.Id.String())
	body := fixture.createdBodies()[0]
	require.Equal(t, dp.DeploymentCreateBodyModeRollback, body.Mode)
	require.Equal(t, ref.Ref(dp.Queue), body.WhenBusy)
	require.Equal(t, ref.Ref(600), body.QueueTtlSeconds)
	require.Equal(t, ref.Ref(baseID), body.ExpectedBaseDeploymentId, "a confirmed rollback diff names its base")
}

func TestDeploymentQueueCommands(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.queueReads = []dp.DeploymentQueueItem{fixture.queueItem(dp.Queued, 1)}
		stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, testGetCmd, "deployment-queue-items", testProjectId, testEnvId, "--state", "queued", outFlag, jsonOutput})
		require.NoError(t, err)
		require.Equal(t, []string{"env_id=my-env&project_id=my-project&state=queued"}, fixture.queueQueries)
		var items []dp.DeploymentQueueItem
		require.NoError(t, json.Unmarshal([]byte(stdout), &items))
		require.Len(t, items, 1)
		require.Equal(t, 1, items[0].Position)

		stdout, _, err = executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, testGetCmd, "deployment-queue"})
		require.NoError(t, err)
		require.Contains(t, stdout, queueTestItemID)
		require.Contains(t, stdout, "queued")

		_, _, err = executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, testGetCmd, "deployment-queue-items", "--state", "waiting"})
		require.ErrorContains(t, err, "unsupported value waiting for --state")
	})
	t.Run("get", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.queueReads = []dp.DeploymentQueueItem{fixture.queueItem(dp.Dispatched, 0)}
		stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, testGetCmd, "deployment-queue-item", queueTestItemID, outFlag, jsonOutput})
		require.NoError(t, err)
		require.Contains(t, stdout, `"deployment_id": "`+fixture.deployment.Id.String()+`"`)
	})
	t.Run("cancel", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		cancelled := fixture.queueItem(dp.Cancelled, 0)
		fixture.createStatus, fixture.createBody = http.StatusOK, cancelled
		stdout, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, queueTestDeploymentsCmd, "cancel-queued", queueTestItemID, "--reason", "superseded"})
		require.NoError(t, err)
		require.Contains(t, stderr, "Queue item 22222222-2222-4222-8222-222222222222 cancelled.")
		require.Contains(t, stdout, "state: cancelled")
		require.Equal(t, []dp.DeploymentQueueCancelBody{{Reason: ref.Ref("superseded")}}, fixture.cancels)

		fixture.createStatus, fixture.createBody = http.StatusConflict, `{"error":"queue_item_dispatching","message":"the item is being dispatched"}`
		_, _, err = executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, queueTestDeploymentsCmd, "cancel-queued", queueTestItemID})
		require.EqualError(t, err, "conflict (queue_item_dispatching): the item is being dispatched")
		require.Equal(t, dp.DeploymentQueueCancelBody{}, fixture.cancels[1], "an omitted reason is not sent")
	})
	t.Run("acknowledge outcome", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, queueTestDeploymentsCmd, "acknowledge-outcome", fixture.deployment.Id.String()})
		require.ErrorContains(t, err, `required flag(s) "reason" not set`)

		stdout, stderr, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, queueTestDeploymentsCmd, "acknowledge-outcome", fixture.deployment.Id.String(), "--reason", "runner host decommissioned", outFlag, jsonOutput})
		require.NoError(t, err)
		require.Equal(t, []string{"/orgs/" + queueTestOrg + "/projects/my-project/envs/my-env/deployments/" + fixture.deployment.Id.String() + "/actions/acknowledge-outcome runner host decommissioned"}, fixture.acks)
		require.Contains(t, stderr, "Outcome of deployment "+fixture.deployment.Id.String()+" acknowledged.")
		require.Contains(t, stdout, `"reason": "runner host decommissioned"`)
	})
}
