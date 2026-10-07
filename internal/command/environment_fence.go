package command

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

// An Environment is fenced while its current Deployment is a failed
// plugin-dispatched Deployment whose Runner may still be active. The Data
// Plane reports the fence on the Environment's last-deployments item and
// refuses every new Deployment with outcome_unreconciled until it ends.

// environmentFenceMessage explains a fence and its remedy.
func environmentFenceMessage(fence dp.EnvironmentFence, now time.Time) string {
	return fmt.Sprintf("Environment is fenced (%s) since %s: deployment %s failed and its Runner may still be active, so new deployments are refused. Once an operator confirmed that the Runner can no longer change the environment, acknowledge it with 'octl deployments acknowledge-outcome %s --%s <text>' (%s).",
		fence.Code, fence.Since.UTC().Format(time.RFC3339), fence.DeploymentId, fence.DeploymentId, reasonFlag, remedyAvailability(fence, now))
}

// remedyAvailability says when acknowledge-outcome is accepted: only after
// the fencing Deployment's Runner dispatch window closed.
func remedyAvailability(fence dp.EnvironmentFence, now time.Time) string {
	if fence.RemedyAvailableAt == nil || !fence.RemedyAvailableAt.After(now) {
		return "possible now"
	}
	return fmt.Sprintf("possible from %s, in %s, when its Runner dispatch window closes",
		fence.RemedyAvailableAt.UTC().Format(time.RFC3339), fence.RemedyAvailableAt.Sub(now).Round(time.Second))
}

// readEnvironmentFence returns the Environment's fence, or nil while it
// accepts new Deployments.
func readEnvironmentFence(ctx context.Context, orgId, projectId, envId string) (*dp.EnvironmentFence, error) {
	r, err := MustDpClient(ctx).ListLastDeploymentsWithResponse(ctx, orgId, &dp.ListLastDeploymentsParams{
		ProjectId:       ref.Ref(projectId),
		EnvId:           ref.Ref(envId),
		StateChangeOnly: ref.Ref(true),
		PerPage:         ref.Ref(1),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list last deployments")
	} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
		return nil, errors.Errorf("unexpected status code %d when listing last deployments: %s", r.StatusCode(), string(r.Body))
	}
	for _, item := range r.JSON200.Items {
		if item.EnvironmentFence != nil {
			return item.EnvironmentFence, nil
		}
	}
	return nil, nil
}

// acknowledgeConflictError explains a refused acknowledgement with the
// Environment's current fence.
func acknowledgeConflictError(ctx context.Context, orgId string, deployment dp.Deployment, conflict *cp.N409Conflict, raw []byte) error {
	refused := fmt.Sprintf("conflict: %s", string(raw))
	if conflict != nil {
		refused = fmt.Sprintf("conflict (%s): %s", conflict.Error, conflict.Message)
	}
	fence, err := readEnvironmentFence(ctx, orgId, deployment.ProjectId, deployment.EnvId)
	if err != nil {
		return errors.Errorf("%s. Reading the environment fence failed too: %s", refused, err)
	}
	now := time.Now()
	switch {
	case fence == nil:
		return errors.Errorf("%s. Environment '%s' is not fenced and accepts new deployments: a Runner result arrived, the outcome was already acknowledged, or another deployment replaced deployment %s", refused, deployment.EnvId, deployment.Id)
	case fence.DeploymentId != deployment.Id:
		return errors.Errorf("%s. Environment '%s' is fenced by deployment %s, not by %s: acknowledge that one (%s)", refused, deployment.EnvId, fence.DeploymentId, deployment.Id, remedyAvailability(*fence, now))
	case fence.RemedyAvailableAt != nil && fence.RemedyAvailableAt.After(now):
		return errors.Errorf("%s. The Runner dispatch window of deployment %s closes at %s (in %s); acknowledge-outcome is accepted from then on, run the command again after it", refused, deployment.Id, fence.RemedyAvailableAt.UTC().Format(time.RFC3339), fence.RemedyAvailableAt.Sub(now).Round(time.Second))
	}
	return errors.New(refused)
}
