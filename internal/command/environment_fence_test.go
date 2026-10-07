package command

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	reasonArg             = "--" + reasonFlag
	acknowledgeOutcomeCmd = "acknowledge-outcome"
)

var fenceTestSince = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// fence marks the fixture's base as a failed plugin-dispatched deployment
// that fences the Environment until remedyAvailableAt (nil: no window).
func (f *deployQueueServer) fence(remedyAvailableAt *time.Time) uuid.UUID {
	id := f.withBase("failed")
	f.base.EnvironmentFence = &dp.EnvironmentFence{
		Code: dp.OutcomeUnreconciled, DeploymentId: id, Since: fenceTestSince,
		Remedy: dp.AcknowledgeOutcome, RemedyAvailableAt: remedyAvailableAt,
	}
	return id
}

func TestDeployShowsTheEnvironmentFence(t *testing.T) {
	refused := `{"error":"outcome_unreconciled","message":"the environment is fenced"}`
	t.Run("remedy not yet available", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		availableAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
		fenceID := fixture.fence(&availableAt).String()
		fixture.createStatus, fixture.createBody = http.StatusConflict, refused

		_, stderr, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag))
		require.ErrorContains(t, err, "conflict (outcome_unreconciled)")
		require.Contains(t, stderr, "Found previous stateful deployment "+fenceID+".\n"+
			"Environment is fenced (outcome_unreconciled) since 2026-10-07T12:00:00Z: deployment "+fenceID+" failed and its Runner may still be active, so new deployments are refused. "+
			"Once an operator confirmed that the Runner can no longer change the environment, acknowledge it with 'octl deployments acknowledge-outcome "+fenceID+" --reason <text>' "+
			"(possible from "+availableAt.Format(time.RFC3339)+", in ")
		require.Contains(t, stderr, ", when its Runner dispatch window closes).\n")
	})
	t.Run("remedy available", func(t *testing.T) {
		for name, availableAt := range map[string]*time.Time{"no window": nil, "window closed": ref.Ref(fenceTestSince)} {
			fixture, ctx := newDeployQueueServer(t)
			fixture.fence(availableAt)
			fixture.createStatus, fixture.createBody = http.StatusConflict, refused
			_, stderr, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag))
			require.Error(t, err, name)
			require.Contains(t, stderr, "--reason <text>' (possible now).\n", name)
		}
	})
	t.Run("not fenced", func(t *testing.T) {
		fixture, ctx := newDeployQueueServer(t)
		fixture.withBase("failed")
		fixture.createStatus, fixture.createBody = http.StatusCreated, fixture.deployment
		_, stderr, err := executeAndResetCommand(ctx, RootCmd, deployQueueArgs(t, noPromptFlag))
		require.NoError(t, err)
		require.NotContains(t, stderr, "fenced")
	})
}

func TestAcknowledgeOutcomeExplainsWhenTheRemedyBecomesAvailable(t *testing.T) {
	acknowledge := func(t *testing.T, configure func(f *deployQueueServer)) error {
		t.Helper()
		fixture, ctx := newDeployQueueServer(t)
		fixture.ackStatus = http.StatusConflict
		configure(fixture)
		_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, queueTestOrg, queueTestDeploymentsCmd, acknowledgeOutcomeCmd, fixture.deployment.Id.String(), reasonArg, "runner host gone"})
		require.Len(t, fixture.acks, 1, "the Control Plane decides; the CLI never refuses on its own clock")
		return err
	}

	t.Run("dispatch window still open", func(t *testing.T) {
		availableAt := time.Now().Add(41 * time.Minute).UTC().Truncate(time.Second)
		var deploymentID string
		err := acknowledge(t, func(f *deployQueueServer) {
			f.ackBody = `{"error":"HTTP-409","message":"the Runner dispatch window is open until ` + availableAt.Format(time.RFC3339) + `"}`
			f.fence(&availableAt)
			f.base.EnvironmentFence.DeploymentId = f.deployment.Id
			deploymentID = f.deployment.Id.String()
		})
		require.ErrorContains(t, err, "conflict (HTTP-409): the Runner dispatch window is open until "+availableAt.Format(time.RFC3339)+
			". The Runner dispatch window of deployment "+deploymentID+" closes at "+availableAt.Format(time.RFC3339)+" (in ")
		require.ErrorContains(t, err, "); acknowledge-outcome is accepted from then on, run the command again after it")
	})
	t.Run("fenced by another deployment", func(t *testing.T) {
		var fenceID uuid.UUID
		err := acknowledge(t, func(f *deployQueueServer) {
			f.ackBody = `{"error":"HTTP-409","message":"deployment is no longer the Environment head and blocks nothing"}`
			fenceID = f.fence(nil)
		})
		require.ErrorContains(t, err, "Environment 'my-env' is fenced by deployment "+fenceID.String()+", not by ")
		require.ErrorContains(t, err, ": acknowledge that one (possible now)")
	})
	t.Run("not fenced", func(t *testing.T) {
		err := acknowledge(t, func(f *deployQueueServer) {
			f.ackBody = `{"error":"HTTP-409","message":"deployment already has Runner stop evidence; there is nothing to acknowledge"}`
		})
		require.ErrorContains(t, err, "conflict (HTTP-409): deployment already has Runner stop evidence; there is nothing to acknowledge. Environment 'my-env' is not fenced and accepts new deployments")
	})
}
