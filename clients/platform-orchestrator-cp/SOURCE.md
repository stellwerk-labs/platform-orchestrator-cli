# Orchestrator Control Plane API contract, public source

`spec.yaml` is byte-identical to the immutable public source below, verified by
anonymous HTTP readback. It is the input of the existing pinned official client
generator. This records public source provenance, not a released client or server.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `6b133551f03fe48130b8bf9974f77b19c614271c`
- Qualification: `public-source`
- File: `openapi/spec.yaml`
- Git blob: `66865d77542d16373e26ac10b2e5c92a7b9323b4`
- File SHA-256: `b720ffe7154012200f52c3b509baaa51ff67ac57140a0dabbd5e9b41d9b649b8`
- Public URL: `https://raw.githubusercontent.com/stellwerk-labs/platform-orchestrator-cp/6b133551f03fe48130b8bf9974f77b19c614271c/openapi/spec.yaml`

The public canonical contract includes the qualified Runner metadata and deployment-domain types. Client generation does not activate execution or establish a matching installed server.

Regenerate through the existing repository toolchain. Normal source verification
fetches this public revision with every local source override unset. Do not
hand-edit the snapshot or generated client.
