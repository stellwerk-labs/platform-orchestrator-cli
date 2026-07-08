package dp

//go:generate go tool oapi-codegen --config=oapi-codegen.cfg.yaml spec.yaml
//go:generate go tool mockgen -destination mocks/client_mock.go -package mockdp github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp ClientWithResponsesInterface
