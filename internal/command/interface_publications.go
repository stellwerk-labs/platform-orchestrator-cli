package command

import (
	"context"
	"fmt"
	cp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-cp"
	"net/http"
)

// publishVersionForSelectedInterface fails closed against older servers when an interface is explicit.
func publishVersionForSelectedInterface(ctx context.Context, client cp.ClientWithResponsesInterface, org, module string, params *cp.PublishModuleVersionParams, body cp.ModuleVersionPublishBody) (*cp.PublishModuleVersionResponse, error) {
	if body.ResourceTypeVersion == nil {
		return client.PublishModuleVersionWithResponse(ctx, org, module, params, body)
	}
	response, err := client.PublishModuleVersionWithInterfaceWithResponse(ctx, org, module, &cp.PublishModuleVersionWithInterfaceParams{IdempotencyKey: params.IdempotencyKey}, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("explicit Resource Type interface publication is unavailable (HTTP 404); check the Module and matching Orchestrator version; no legacy publication was attempted")
	}
	return &cp.PublishModuleVersionResponse{HTTPResponse: response.HTTPResponse, Body: response.Body,
		JSON201: response.JSON201, JSON400: response.JSON400, JSON403: response.JSON403, JSON404: response.JSON404, JSON409: response.JSON409}, nil
}

// publishSuccessorForSelectedInterface fails closed against older servers when an interface is explicit.
func publishSuccessorForSelectedInterface(ctx context.Context, client cp.ClientWithResponsesInterface, org, module string, prerelease string, params *cp.PublishStableModuleVersionSuccessorParams, body cp.StableModuleVersionSuccessorBody) (*cp.PublishStableModuleVersionSuccessorResponse, error) {
	if body.Version.ResourceTypeVersion == nil {
		return client.PublishStableModuleVersionSuccessorWithResponse(ctx, org, module, prerelease, params, body)
	}
	response, err := client.PublishStableModuleVersionSuccessorWithInterfaceWithResponse(ctx, org, module, prerelease, &cp.PublishStableModuleVersionSuccessorWithInterfaceParams{IdempotencyKey: params.IdempotencyKey}, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("explicit Resource Type interface publication is unavailable (HTTP 404); check the Module and matching Orchestrator version; no legacy publication was attempted")
	}
	return &cp.PublishStableModuleVersionSuccessorResponse{HTTPResponse: response.HTTPResponse, Body: response.Body,
		JSON201: response.JSON201, JSON400: response.JSON400, JSON403: response.JSON403, JSON404: response.JSON404, JSON409: response.JSON409}, nil
}
