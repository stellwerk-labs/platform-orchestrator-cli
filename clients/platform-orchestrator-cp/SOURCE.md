# Orchestrator Control Plane API contract

`spec.yaml` is a byte-identical copy of the selected immutable public source
commit and the sole input of the repository's pinned official generated client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `935835a8efa36623f955294b48fadbaedb90436f`
- File: `openapi/spec.yaml`
- Git blob: `d2862d7346e280051110bfa0394f66bc15a61f62`
- File SHA-256: `d622638c9aaa6baaf3b9648cbba3f3518f1738e6d20583f804c69c0f1421db7c`

This contract exposes deferred Provider deletion: `202 Accepted` requests
retirement, and `deletion_requested_at` remains readable until cleanup completes.
Existing Runner capacity/protocol fields and Module Draft interfaces are retained.
The corresponding public SDK is CP shared/v3 v3.3.0.

The exact source commit is publicly reachable. This source pin does not prove
that a server artifact has been released or that a running installation supports
the new behavior. Client publication requires the qualified coordinated server
set and ordinary public-source checks without local specification overrides.
Regenerate through the repository's official generators; do not hand-edit the
snapshot or generated client. IAM's automation contract is unchanged in this
train, so its existing public source pin is retained.
