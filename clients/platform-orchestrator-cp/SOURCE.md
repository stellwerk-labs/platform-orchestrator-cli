# Orchestrator Control Plane API contract

`spec.yaml` is a byte-identical copy of the selected source commit and the sole
input of the generated client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `1a2a812e48ac35a478398747b37cf90f6d536c07`
- File: `openapi/spec.yaml`
- Git blob: `a8f6384df376398578fd88352d5187e3f84be01b`
- File SHA-256: `cc9c8c3813be81bfa487fa8d8e38457c51cca8ef17f6eecf59d775ef89677875`

This contract adds revisioned Module Draft authoring, immutable scoped
test snapshots and explicit test execution through the ordinary deployment
engine, alongside exact Resource Type interface versions. Published Module
Versions remain immutable. Saving a Draft does not execute it or reserve SemVer.

Before a client release, verify this immutable source through the unchanged
public-source checks. A local Git comparison proves byte identity, not public
availability. Regenerate through the repository official generators; do not
hand-edit this snapshot or generated client.
