package command

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/stellwerk-labs/platform-orchestrator-cli/clients"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/printer"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	deployCmdNoQueueFlag  = "no-queue"
	deployCmdQueueTTLFlag = "queue-ttl"
	queueItemStateFlag    = "state"
	reasonFlag            = "reason"

	errorCodeEnvironmentBusy       = "environment_busy"
	errorCodeDeploymentQueueFull   = "deployment_queue_full"
	errorCodeDeploymentBaseChanged = "deployment_base_changed"
	errorCodeOutcomeUnreconciled   = "outcome_unreconciled"
)

// deploymentQueuePollInterval is how often a waiting queue item is read again.
var deploymentQueuePollInterval = 5 * time.Second

// addDeployQueueFlags registers the queue flags of a command that creates
// deploy or rollback Deployments. Call it after the plan-only and dry-run flags
// exist: neither kind of request is ever queued.
func addDeployQueueFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(deployCmdNoQueueFlag, false, "Fail with environment_busy instead of waiting in the environment's deployment queue while another deployment runs")
	cmd.Flags().Duration(deployCmdQueueTTLFlag, 0, "How long the request may wait in the deployment queue before it expires, for example 45m (default: the server's queue TTL)")
	cmd.MarkFlagsMutuallyExclusive(deployCmdNoQueueFlag, deployCmdQueueTTLFlag)
	cmd.MarkFlagsMutuallyExclusive(deployCmdPlanOnlyFlag, deployCmdQueueTTLFlag)
	cmd.MarkFlagsMutuallyExclusive(deployCmdDryRunFlag, deployCmdQueueTTLFlag)
}

// deployQueue says what a request does when its Environment is busy. Deploy and
// rollback requests always say it explicitly: they wait in the Environment's
// deployment queue, or fail with environment_busy under --no-queue. Plan-only
// requests and dry runs are never queued and send no queue fields.
type deployQueue struct {
	planOnly bool
	dryRun   bool
	reject   bool
	ttl      time.Duration
}

func deployQueueFromFlags(cmd *cobra.Command) (deployQueue, error) {
	noQueue, _ := cmd.Flags().GetBool(deployCmdNoQueueFlag)
	planOnly, _ := cmd.Flags().GetBool(deployCmdPlanOnlyFlag)
	dryRun, _ := cmd.Flags().GetBool(deployCmdDryRunFlag)
	ttl, _ := cmd.Flags().GetDuration(deployCmdQueueTTLFlag)
	if cmd.Flags().Changed(deployCmdQueueTTLFlag) && (ttl < time.Second || ttl%time.Second != 0) {
		return deployQueue{}, errors.Errorf("--%s must be a whole number of seconds of at least 1s, got %s", deployCmdQueueTTLFlag, ttl)
	}
	return deployQueue{planOnly: planOnly, dryRun: dryRun, reject: noQueue, ttl: ttl}, nil
}

func (q deployQueue) queueable() bool {
	return !q.planOnly && !q.dryRun
}

func (q deployQueue) apply(body *dp.DeploymentCreateBody) {
	switch {
	case !q.queueable():
	case q.reject:
		// Omitting when_busy means queue: fail fast only when asked to.
		body.WhenBusy = ref.Ref(dp.Reject)
	default:
		body.WhenBusy = ref.Ref(dp.Queue)
		if q.ttl > 0 {
			body.QueueTtlSeconds = ref.Ref(int(q.ttl / time.Second))
		}
	}
}

// executingBase decides what the request does while base, the Environment's
// current deployment, still executes: a deploy or rollback waits behind it in
// the queue, anything else fails before the diff is shown.
func (q deployQueue) executingBase(base dp.DeploymentSummary) error {
	executing := fmt.Sprintf("last deployment %s is still executing for %s", base.Id, time.Since(base.CreatedAt))
	switch {
	case !q.queueable():
		return errors.New(executing)
	case q.reject:
		return errors.Errorf("%s; retry once it finished, or run the command without --%s to wait in the environment's deployment queue", executing, deployCmdNoQueueFlag)
	}
	changedMessageF("Deployment %s is still executing; this request will wait in the environment's deployment queue.", base.Id)
	return nil
}

// waitForQueuedDeployment follows a queued request until it became a
// Deployment and returns that Deployment. Every other end of the wait is an
// error that says why and what to do next.
func waitForQueuedDeployment(ctx context.Context, orgId string, item dp.DeploymentQueueItem) (dp.Deployment, error) {
	dpClient := MustDpClient(ctx)
	changedMessageF("Environment is busy: request queued as item %s at position %d, expires at %s.", item.Id, item.Position, item.ExpiresAt.Format(time.RFC3339))
	ticker := time.NewTicker(deploymentQueuePollInterval)
	defer ticker.Stop()
	for item.State == dp.Queued {
		select {
		case <-ctx.Done():
			return dp.Deployment{}, errors.Errorf("stopped waiting: queue item %s is still queued and deploys once the environment is free; cancel it with 'octl deployments cancel-queued %s'", item.Id, item.Id)
		case <-ticker.C:
		}
		r, err := dpClient.GetDeploymentQueueItemWithResponse(ctx, orgId, item.Id)
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			return dp.Deployment{}, errors.Wrapf(err, "failed to read queue item %s", item.Id)
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return dp.Deployment{}, errors.Errorf("unexpected status code %d when reading queue item %s: %s", r.StatusCode(), item.Id, string(r.Body))
		}
		previous := item
		item = *r.JSON200
		if item.State != dp.Queued {
			break
		}
		if item.Attempts != previous.Attempts && item.LastError != nil {
			changedMessageF("Starting the queued request failed transiently (attempt %d), retrying: %s", item.Attempts, *item.LastError)
		}
		if item.Position != previous.Position {
			infoMessageF("Waiting in the deployment queue at position %d (%s)...", item.Position, time.Since(item.CreatedAt).Round(time.Second))
		}
	}

	reason := ref.DerefOr(item.ReasonCode, string(item.State))
	message := ref.DerefOr(item.ReasonMessage, "")
	switch {
	case item.State == dp.Dispatched && item.DeploymentId != nil:
		successMessageF("Queue item %s dispatched as deployment %s.", item.Id, *item.DeploymentId)
		r, err := dpClient.GetDeploymentWithResponse(ctx, orgId, *item.DeploymentId)
		if err != nil {
			return dp.Deployment{}, errors.Wrap(err, "failed to get deployment")
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return dp.Deployment{}, errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
		}
		return *r.JSON200, nil
	case reason == errorCodeDeploymentBaseChanged:
		return dp.Deployment{}, errors.Errorf("queue item %s was rejected (%s): %s. Another deployment changed the environment while this request waited; run the command again to review the diff against the current state", item.Id, reason, message)
	case item.State == dp.Expired:
		return dp.Deployment{}, errors.Errorf("queue item %s expired at %s before the environment became free; run the command again, with a longer --%s if needed", item.Id, item.ExpiresAt.Format(time.RFC3339), deployCmdQueueTTLFlag)
	default:
		return dp.Deployment{}, errors.Errorf("queue item %s was %s (%s): %s", item.Id, item.State, reason, message)
	}
}

// deploymentConflictError turns a refused deploy or rollback into an error that
// names the stable code and the next step.
func deploymentConflictError(conflict *dp.N409Conflict, raw []byte, body dp.DeploymentCreateBody) error {
	if conflict == nil {
		return errors.Errorf("conflict: %s", string(raw))
	}
	switch conflict.Error {
	case errorCodeEnvironmentBusy:
		switch {
		case body.WhenBusy == nil:
			return errors.Errorf("conflict (%s): %s. Plan-only requests never wait in the deployment queue and do not run beside another deployment of the environment; retry once it finished", conflict.Error, conflict.Message)
		case *body.WhenBusy == dp.Reject:
			return errors.Errorf("conflict (%s): %s. Retry once the running deployment finished, or run the command without --%s to wait in the environment's deployment queue", conflict.Error, conflict.Message, deployCmdNoQueueFlag)
		default:
			return errors.Errorf("conflict (%s): %s. Requests authorized by an organization token cannot wait in the deployment queue; retry once the running deployment finished, or authenticate as a user ('octl login') or service user to wait", conflict.Error, conflict.Message)
		}
	case errorCodeDeploymentQueueFull:
		return errors.Errorf("conflict (%s): %s. List the waiting requests with 'octl get deployment-queue-items %s %s --state queued' and cancel obsolete ones with 'octl deployments cancel-queued <queue-item-id>'", conflict.Error, conflict.Message, body.ProjectId, body.EnvId)
	case errorCodeDeploymentBaseChanged:
		if body.ExpectedBaseDeploymentId != nil {
			return errors.Errorf("conflict (%s): %s. The manifest and diff were derived from deployment %s, but another deployment has changed the environment since; run the command again to review the diff against the current state", conflict.Error, conflict.Message, *body.ExpectedBaseDeploymentId)
		}
	case errorCodeOutcomeUnreconciled:
		return errors.Errorf("conflict (%s): %s. Once an operator confirmed that its Runner can no longer change the environment, acknowledge it with 'octl deployments acknowledge-outcome <deployment-id> --%s <text>'", conflict.Error, conflict.Message, reasonFlag)
	}
	return errors.Errorf("conflict (%s): %s", conflict.Error, conflict.Message)
}

var ListDeploymentQueueItems = &cobra.Command{
	Use:     "deployment-queue-items [project-id] [environment-id]",
	Aliases: []string{"deployment-queue", "queue-items"},
	Args:    cobra.RangeArgs(0, 2),
	Short:   "List deploy and rollback requests in environment deployment queues, oldest first",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		dpc := MustDpClient(cmd.Context())
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		params := dp.ListDeploymentQueueItemsParams{}
		if len(args) > 0 {
			params.ProjectId = ref.RefStringEmptyNil(args[0])
		}
		if len(args) > 1 {
			params.EnvId = ref.RefStringEmptyNil(args[1])
		}
		if state, _ := cmd.Flags().GetString(queueItemStateFlag); state != "" {
			if !slices.Contains([]dp.DeploymentQueueItemState{dp.Queued, dp.Dispatched, dp.Cancelled, dp.Expired, dp.Rejected}, dp.DeploymentQueueItemState(state)) {
				return errors.Errorf("unsupported value %s for --%s", state, queueItemStateFlag)
			}
			params.State = ref.Ref(dp.DeploymentQueueItemState(state))
		}
		items, err := clients.CollectAll(
			func(pt string) (*dp.ListDeploymentQueueItemsResponse, error) {
				params.Page = ref.RefStringEmptyNil(pt)
				return dpc.ListDeploymentQueueItemsWithResponse(cmd.Context(), orgId, &params)
			},
			func(r *dp.ListDeploymentQueueItemsResponse) ([]dp.DeploymentQueueItem, *string) {
				return r.JSON200.Items, r.JSON200.NextPageToken
			},
		)
		if err != nil {
			return err
		}
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), items)
	},
}

var GetDeploymentQueueItem = &cobra.Command{
	Use:     "deployment-queue-item <queue-item-id>",
	Aliases: []string{"queue-item"},
	Args:    cobra.ExactArgs(1),
	Short:   "Get a deployment queue item, including the manifest it requested",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		itemId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrapf(err, "invalid queue item id '%s'", args[0])
		}
		if r, err := MustDpClient(cmd.Context()).GetDeploymentQueueItemWithResponse(cmd.Context(), orgId, itemId); err != nil {
			return errors.Wrap(err, "failed to get queue item")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("queue item '%s' not found in org '%s'", itemId, orgId)
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return errors.Errorf("unexpected status code %d when getting queue item: %s", r.StatusCode(), string(r.Body))
		} else {
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

// DeploymentsCmd groups actions on Deployments and their queue. Reading them
// stays under 'octl get'.
var DeploymentsCmd = &cobra.Command{
	Use:   "deployments",
	Short: "Act on deployments and on requests waiting in deployment queues",
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		output, _ := cmd.Flags().GetString(printer.OutputFormatFlag)
		ctx, err := withPrinter(cmd.Context(), output, []string{printer.JsonPrinterType, printer.YamlPrinterType})
		if err != nil {
			return err
		}
		cmd.SetContext(ctx)
		return nil
	},
}

var CancelQueuedDeployment = &cobra.Command{
	Use:   "cancel-queued <queue-item-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Cancel a deploy or rollback request that is still waiting in its environment's deployment queue",
	Long: `Cancel a deploy or rollback request that is still waiting in its environment's deployment queue.

Any user who may deploy to the environment can cancel its queued requests. Repeating the same cancellation returns the
cancelled item. A request that is being started at this moment cannot be cancelled; read it again with
'octl get deployment-queue-item <queue-item-id>' to see the deployment it became.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		itemId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrapf(err, "invalid queue item id '%s'", args[0])
		}
		reason, _ := cmd.Flags().GetString(reasonFlag)
		if r, err := MustDpClient(cmd.Context()).CancelDeploymentQueueItemWithResponse(cmd.Context(), orgId, itemId, dp.DeploymentQueueCancelBody{Reason: ref.RefStringEmptyNil(reason)}); err != nil {
			return errors.Wrap(err, "failed to cancel queue item")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("queue item '%s' not found in org '%s'", itemId, orgId)
		} else if r.StatusCode() == http.StatusConflict && r.JSON409 != nil {
			return errors.Errorf("conflict (%s): %s", r.JSON409.Error, r.JSON409.Message)
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return errors.Errorf("unexpected status code %d when cancelling queue item: %s", r.StatusCode(), string(r.Body))
		} else {
			successMessageF("Queue item %s cancelled.", itemId)
			return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *r.JSON200)
		}
	},
}

var AcknowledgeDeploymentOutcome = &cobra.Command{
	Use:   "acknowledge-outcome <deployment-id>",
	Args:  cobra.ExactArgs(1),
	Short: "Acknowledge the unknown Runner outcome of a failed plugin-dispatched deployment",
	Long: `Acknowledge the unknown Runner outcome of a failed plugin-dispatched deployment.

A failed deployment started by a plugin whose Runner never reported a result blocks its environment: the Runner may
still be changing it. After confirming that the Runner can no longer change the environment, an operator records that
with a reason. The deployment's status and history stay unchanged, requests that waited in the queue were rejected, and
nothing starts by itself: new deploys work again.

The acknowledgement is accepted only after the deployment's Runner dispatch window closed. 'octl deploy' shows the
fence and when that is; a refused acknowledgement says it too.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		orgId, err := ShouldOrg(cmd.Context())
		if err != nil {
			return err
		}
		deploymentId, err := uuid.Parse(args[0])
		if err != nil {
			return errors.Wrapf(err, "invalid deployment id '%s'", args[0])
		}
		reason, _ := cmd.Flags().GetString(reasonFlag)
		var deployment dp.Deployment
		if r, err := MustDpClient(cmd.Context()).GetDeploymentWithResponse(cmd.Context(), orgId, deploymentId); err != nil {
			return errors.Wrap(err, "failed to get deployment")
		} else if r.StatusCode() == http.StatusNotFound {
			return errors.Errorf("deployment '%s' not found in org '%s'", deploymentId, orgId)
		} else if r.StatusCode() != http.StatusOK || r.JSON200 == nil {
			return errors.Errorf("unexpected status code %d when getting deployment: %s", r.StatusCode(), string(r.Body))
		} else {
			deployment = *r.JSON200
		}
		r, err := MustCpClient(cmd.Context()).AcknowledgeDeploymentOutcomeWithResponse(cmd.Context(), orgId, deployment.ProjectId, deployment.EnvId, deploymentId, cp.DeploymentOutcomeAcknowledgementBody{Reason: reason})
		if err != nil {
			return errors.Wrap(err, "failed to acknowledge deployment outcome")
		}
		var acknowledgement *cp.DeploymentOutcomeAcknowledgement
		switch r.StatusCode() {
		case http.StatusCreated:
			acknowledgement = r.JSON201
		case http.StatusOK:
			acknowledgement = r.JSON200
		case http.StatusConflict:
			return acknowledgeConflictError(cmd.Context(), orgId, deployment, r.JSON409, r.Body)
		}
		if acknowledgement == nil {
			return errors.Errorf("unexpected status code %d when acknowledging deployment outcome: %s", r.StatusCode(), string(r.Body))
		}
		successMessageF("Outcome of deployment %s acknowledged.", deploymentId)
		return MustPrinter(cmd.Context()).Write(cmd.OutOrStdout(), *acknowledgement)
	},
}

func init() {
	ListDeploymentQueueItems.Flags().String(queueItemStateFlag, "", "Only list items in this state (queued|dispatched|cancelled|expired|rejected)")
	GetCmd.AddCommand(ListDeploymentQueueItems, GetDeploymentQueueItem)

	printer.SetupSingleOutputFormatFlag(DeploymentsCmd.PersistentFlags())
	CancelQueuedDeployment.Flags().String(reasonFlag, "", "Why the request is no longer wanted")
	AcknowledgeDeploymentOutcome.Flags().String(reasonFlag, "", "Why the outcome can no longer change, for audit")
	_ = AcknowledgeDeploymentOutcome.MarkFlagRequired(reasonFlag)
	DeploymentsCmd.AddCommand(CancelQueuedDeployment, AcknowledgeDeploymentOutcome)
	RootCmd.AddCommand(DeploymentsCmd)
}
