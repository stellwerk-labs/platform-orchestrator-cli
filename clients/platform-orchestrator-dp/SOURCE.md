# Orchestrator Data Plane API contract

The immutable source commit below is publicly reachable through shared/v3.3.0.
`spec.yaml` is a byte-identical copy of its `openapi/spec.yaml`.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `6eea87239038c8f6208c9c9b68dc98797d407a7b`
- File: `openapi/spec.yaml`
- Git blob: `d95247ac045fd16aaf6e0c653702501a7d86b0b1`
- File SHA-256: `9421b3f94b36cab8e66653db258a0172c61bfc0c812a76b63024a462fa2cd24c`

This contract adds `managed_logs` and authenticated `decrypt=true` log retrieval.
The pinned official generator produces `client.gen.go`; neither the snapshot nor
generated code is hand edited. `make check-api-specs` verifies public source
parity. The SDK source pin does not establish server availability; publication
requires the qualified coordinated server release.
