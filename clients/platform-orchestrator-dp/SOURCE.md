# Orchestrator Data Plane API contract

`spec.yaml` is a byte-identical copy of the selected source commit and the sole
input of the generated client.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `eee2fb607084ce5eef994726a2c3b88082e85397`
- File: `openapi/spec.yaml`
- Git blob: `8b5e32e7dad33fc9cd0e260fc1d085d05e54c353`
- File SHA-256: `e2cb98ee7622a892a6cb20d67afeae70fc81e17cffd982757a227e3db24b38fa`

This contract adds revisioned Module Draft authoring, immutable scoped
test snapshots and explicit test execution through the ordinary deployment
engine, alongside exact Resource Type interface versions. Published Module
Versions remain immutable. Saving a Draft does not execute it or reserve SemVer.

Before a client release, verify this immutable source through the unchanged
public-source checks. A local Git comparison proves byte identity, not public
availability. Regenerate through the repository official generators; do not
hand-edit this snapshot or generated client.
