# Orchestrator Control Plane API contract

`spec.yaml` is a byte-identical copy of the selected immutable public source
commit and the sole input of the repository's pinned official generated client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `c5d1b57338250f375e1481d066be81b5525d2686`
- File: `openapi/spec.yaml`
- Git blob: `109e93fa9b4777c7e9d5c16eaf9aa9fc6690a78a`
- File SHA-256: `8230ef5ca9a1b1c22401596e5f116789c031f8e8b726d454252f2bc093d900af`

This contract exposes deferred Provider deletion: `202 Accepted` requests
retirement, and `deletion_requested_at` remains readable until cleanup completes.
Existing Runner capacity/protocol fields and Module Draft interfaces are retained.
The contract also exposes optional `has_proposed` and `default_verified` catalogue
summaries and dedicated `deployment_logs_read` authorization for plugin logs.
The corresponding public SDK is CP shared/v3 v3.4.0.

The exact source commit is publicly reachable. This source pin does not prove
that a server artifact has been released or that a running installation supports
the new behavior. Client publication requires the qualified coordinated server
set and ordinary public-source checks without local specification overrides.
Regenerate through the repository's official generators; do not hand-edit the
snapshot or generated client. IAM's automation contract is unchanged in this
train; its verifier now checks the matching public IAM source revision.
