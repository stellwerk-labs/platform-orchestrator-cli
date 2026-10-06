package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	rollouts "github.com/stellwerk-labs/platform-orchestrator-cli/clients/progressive-rollouts"
)

const (
	rolloutIDPathParameter     = "rolloutId"
	planRevisionQueryParameter = "plan_revision"
)

type nativeRolloutOperation struct {
	method          string
	mutating        bool
	pathParameters  []string
	queryParameters []string
	bodyRequired    bool
	responseStatus  int
}

var nativeRolloutOperations = map[string]nativeRolloutOperation{
	"listProgressiveRollouts":           {method: http.MethodGet, queryParameters: []string{"per_page", "page", "state", "project", "module", "conflicts", "view", "history_visibility", "include_view_counts"}, responseStatus: http.StatusOK},
	"getProgressiveRollout":             {method: http.MethodGet, pathParameters: []string{rolloutIDPathParameter}, responseStatus: http.StatusOK},
	"previewProgressiveRollout":         {method: http.MethodPost, bodyRequired: true, responseStatus: http.StatusOK},
	"createProgressiveRollout":          {method: http.MethodPost, mutating: true, bodyRequired: true, responseStatus: http.StatusCreated},
	"updateProgressiveRolloutDraft":     {method: http.MethodPatch, mutating: true, pathParameters: []string{rolloutIDPathParameter}, bodyRequired: true, responseStatus: http.StatusOK},
	"prepareProgressiveRollout":         {method: http.MethodPost, mutating: true, pathParameters: []string{rolloutIDPathParameter}, bodyRequired: true, responseStatus: http.StatusOK},
	"commandProgressiveRollout":         {method: http.MethodPost, mutating: true, pathParameters: []string{rolloutIDPathParameter, "rolloutCommand"}, bodyRequired: true, responseStatus: http.StatusOK},
	"approveProgressiveRolloutScope":    {method: http.MethodPost, mutating: true, pathParameters: []string{rolloutIDPathParameter}, bodyRequired: true, responseStatus: http.StatusCreated},
	"revokeProgressiveRolloutApproval":  {method: http.MethodDelete, mutating: true, pathParameters: []string{rolloutIDPathParameter, "approvalId"}, queryParameters: []string{planRevisionQueryParameter}, responseStatus: http.StatusNoContent},
	"resolveProgressiveRolloutConflict": {method: http.MethodPost, mutating: true, pathParameters: []string{rolloutIDPathParameter, "conflictId", "resolution"}, bodyRequired: true, responseStatus: http.StatusOK},
	"listProgressiveRolloutApprovals":   {method: http.MethodGet, pathParameters: []string{rolloutIDPathParameter}, queryParameters: []string{planRevisionQueryParameter}, responseStatus: http.StatusOK},
	"listProgressiveRolloutConflicts":   {method: http.MethodGet, pathParameters: []string{rolloutIDPathParameter}, queryParameters: []string{planRevisionQueryParameter}, responseStatus: http.StatusOK},
	"getProgressiveRolloutPlan":         {method: http.MethodGet, pathParameters: []string{rolloutIDPathParameter, "planRevision"}, responseStatus: http.StatusOK},
	"listProgressiveRolloutEvents":      {method: http.MethodGet, pathParameters: []string{rolloutIDPathParameter}, queryParameters: []string{"after"}, responseStatus: http.StatusOK},
}

func validateNativeParameters(operationID string, operation nativeRolloutOperation, pathParameters map[string]string, query map[string][]string, body any, idempotencyKey string, expectedStatus int) error {
	allowedPaths := make(map[string]struct{}, len(operation.pathParameters))
	for _, name := range operation.pathParameters {
		allowedPaths[name] = struct{}{}
		if strings.TrimSpace(pathParameters[name]) == "" {
			return errors.Errorf("%s requires path parameter %q", operationID, name)
		}
	}
	if len(pathParameters) != len(allowedPaths) {
		return errors.Errorf("%s received unexpected path parameters", operationID)
	}
	allowedQueries := make(map[string]struct{}, len(operation.queryParameters))
	for _, name := range operation.queryParameters {
		allowedQueries[name] = struct{}{}
	}
	for name, values := range query {
		if _, allowed := allowedQueries[name]; !allowed {
			return errors.Errorf("%s received unexpected query parameter %q", operationID, name)
		}
		if len(values) == 0 {
			return errors.Errorf("%s query parameter %q must contain a value", operationID, name)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return errors.Errorf("%s query parameter %q must not contain an empty value", operationID, name)
			}
		}
	}
	if operation.bodyRequired != (body != nil) {
		return errors.Errorf("%s has an unexpected request body", operationID)
	}
	if operation.mutating && strings.TrimSpace(idempotencyKey) == "" {
		return errors.Errorf("idempotency key is required for %s", operationID)
	}
	if !operation.mutating && idempotencyKey != "" {
		return errors.Errorf("%s does not accept an idempotency key", operationID)
	}
	if operation.responseStatus != expectedStatus {
		return errors.Errorf("%s expected response status %d, not %d", operationID, operation.responseStatus, expectedStatus)
	}
	return nil
}

func nativeProgressiveRolloutListQuery(page string, states []string, projectID, moduleID, view, historyVisibility string, conflicts *bool) map[string][]string {
	query := make(map[string][]string)
	if page != "" {
		query["page"] = []string{page}
	}
	if len(states) != 0 {
		query["state"] = append([]string(nil), states...)
	}
	for key, value := range map[string]string{"project": projectID, "module": moduleID, "view": view, "history_visibility": historyVisibility} {
		if value != "" {
			query[key] = []string{value}
		}
	}
	if conflicts != nil {
		query["conflicts"] = []string{fmt.Sprint(*conflicts)}
	}
	return query
}

func nativeProgressiveRolloutsContribution(ctx context.Context, orgID string) (cp.PluginContribution, error) {
	response, err := MustCpClient(ctx).ListPluginContributionsWithResponse(ctx, orgID)
	if err != nil {
		return cp.PluginContribution{}, errors.Wrap(err, "plugin_unavailable: Progressive Rollouts discovery failed")
	}
	if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		return cp.PluginContribution{}, progressiveRolloutsCapabilityDiscoveryError(response.StatusCode(), response.Body)
	}
	for _, contribution := range response.JSON200.Items {
		if contribution.Id == progressiveRolloutsPluginID {
			return contribution, nil
		}
	}
	return cp.PluginContribution{}, errors.Errorf("plugin_not_installed: Progressive Rollouts (%s) is not installed for this organization", progressiveRolloutsPluginID)
}

func requireNativeProgressiveRolloutsOperation(ctx context.Context, orgID, operationID string, access progressiveRolloutsAccess) error {
	expected, known := nativeRolloutOperations[operationID]
	if !known {
		return errors.Errorf("plugin_operation_unavailable: client does not permit Progressive Rollouts operation %q", operationID)
	}
	contribution, err := nativeProgressiveRolloutsContribution(ctx, orgID)
	if err != nil {
		return err
	}
	capability := cp.PluginCapability{Id: contribution.Id, DisplayName: contribution.DisplayName, State: contribution.State}
	if err := progressiveRolloutsPluginError(capability, access); err != nil {
		return err
	}
	if contribution.Mode == cp.Hidden ||
		(access == progressiveRolloutsMutate && contribution.Mode != cp.Full) ||
		(access == progressiveRolloutsSafety && contribution.Mode != cp.Full && contribution.Mode != cp.SafetyOnly) {
		return errors.Errorf("plugin_operation_unavailable: Progressive Rollouts operation %q is unavailable in %s mode", operationID, contribution.Mode)
	}
	for _, operation := range contribution.Operations {
		if operation.Id != operationID {
			continue
		}
		if operation.Method != expected.method || operation.Mutating != expected.mutating {
			return errors.Errorf("plugin_incompatible: Progressive Rollouts operation %q has unexpected transport metadata", operationID)
		}
		return nil
	}
	return errors.Errorf("plugin_operation_unavailable: Progressive Rollouts operation %q is not advertised", operationID)
}

func invokeNativeProgressiveRollout[T any](ctx context.Context, orgID, operationID string, pathParameters map[string]string, query map[string][]string, body any, idempotencyKey string, expectedStatus int) (*T, error) {
	operation, known := nativeRolloutOperations[operationID]
	if !known {
		return nil, errors.Errorf("plugin_operation_unavailable: client does not permit Progressive Rollouts operation %q", operationID)
	}
	if err := validateNativeParameters(operationID, operation, pathParameters, query, body, idempotencyKey, expectedStatus); err != nil {
		return nil, err
	}
	invocation := cp.PluginInvocation{OperationId: operationID}
	if len(pathParameters) != 0 {
		invocation.PathParameters = &pathParameters
	}
	if len(query) != 0 {
		invocation.Query = &query
	}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, errors.Wrapf(err, "encode %s request", operationID)
		}
		raw := json.RawMessage(encoded)
		invocation.Body = &raw
	}
	if idempotencyKey != "" {
		invocation.IdempotencyKey = &idempotencyKey
	}
	response, err := MustCpClient(ctx).InvokePluginOperationWithResponse(ctx, orgID, progressiveRolloutsPluginID, invocation)
	if err != nil {
		return nil, errors.Wrapf(err, "invoke Progressive Rollouts operation %s", operationID)
	}
	if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		return nil, rolloutResponseError("invoke "+operationID, response.StatusCode(), response.Body)
	}
	if response.JSON200.Status != expectedStatus {
		return nil, rolloutResponseError(operationID, response.JSON200.Status, response.JSON200.Body)
	}
	if expectedStatus == http.StatusNoContent {
		return nil, nil
	}
	var result T
	if err := json.Unmarshal(response.JSON200.Body, &result); err != nil {
		return nil, errors.Wrapf(err, "decode %s response", operationID)
	}
	return &result, nil
}

func nativeProgressiveRolloutMaterial(ctx context.Context, orgID string, detail rollouts.ProgressiveRolloutDetail, deploymentID uuid.UUID, kind string) ([]byte, int, error) {
	if detail.Plan == nil {
		return nil, 0, errors.New("plugin_material_reference_missing: Rollout has no frozen Plan")
	}
	var environmentID uuid.UUID
	for _, execution := range detail.Executions {
		if uuidPointerEquals(execution.DeploymentId, deploymentID) || uuidPointerEquals(execution.RollbackDeploymentId, deploymentID) || uuidPointerEquals(execution.EffectiveBaseDeploymentId, deploymentID) {
			environmentID = execution.EnvironmentId
			break
		}
	}
	if environmentID == uuid.Nil {
		return nil, 0, errors.Errorf("plugin_material_reference_missing: deployment %s is not retained by this Rollout", deploymentID)
	}
	var projectID uuid.UUID
	for _, environment := range detail.Plan.Environments {
		if environment.Uuid == environmentID {
			projectID = environment.ProjectUuid
			break
		}
	}
	if projectID == uuid.Nil {
		return nil, 0, errors.Errorf("plugin_material_reference_missing: frozen scope for deployment %s is unavailable", deploymentID)
	}
	materialKind := cp.GetPluginDeploymentMaterialParamsMaterialKind(kind)
	response, err := MustCpClient(ctx).GetPluginDeploymentMaterialWithResponse(ctx, orgID, progressiveRolloutsPluginID, projectID, environmentID, deploymentID, materialKind)
	if err != nil {
		return nil, 0, errors.Wrap(err, "retrieve protected Rollout deployment material")
	}
	return response.Body, response.StatusCode(), nil
}

func uuidPointerEquals(value *uuid.UUID, expected uuid.UUID) bool {
	return value != nil && *value == expected
}
