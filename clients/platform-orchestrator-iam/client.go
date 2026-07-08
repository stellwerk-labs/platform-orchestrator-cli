package iam

//go:generate go tool oapi-codegen --config=oapi-codegen.cfg.yaml spec.yaml
//go:generate go tool mockgen -destination mocks/client_mock.go -package mockiam github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-iam ClientWithResponsesInterface
