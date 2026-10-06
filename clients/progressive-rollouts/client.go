// Package progressiverollouts holds the request and response models of the
// Enterprise plugin Progressive Rollouts (dev.stellwerk.progressive-rollouts).
// The CLI sends them through Core's generic plugin invocation; see SOURCE.md.
package progressiverollouts

//go:generate go tool oapi-codegen --config=oapi-codegen.cfg.yaml spec.yaml
