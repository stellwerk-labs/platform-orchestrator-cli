# Orchestrator Data Plane API contract, public source

`spec.yaml` is byte-identical to the immutable public source below, verified by
anonymous HTTP readback. It is the input of the existing pinned official client
generator. This records public source provenance, not a released client or server.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `e31b51422403c7b1aee3ee033bb645a304009b94`
- Qualification: `public-source`
- File: `openapi/spec.yaml`
- Git blob: `5d3a805e0b742fda21026e5af9cee7aa532696ab`
- File SHA-256: `43a4ad79fc8e1ad2f8dcc8410c3eb6e884f829325f9c18abecfcc5e55885dfef`
- Public URL: `https://raw.githubusercontent.com/stellwerk-labs/platform-orchestrator-dp/e31b51422403c7b1aee3ee033bb645a304009b94/openapi/spec.yaml`

The public canonical contract includes deployment pipeline controls, phase-log selectors and observation metadata. Source availability and generated client checks are separate from server/runtime qualification.

Regenerate through the existing repository toolchain. Normal source verification
fetches this public revision with every local source override unset. Do not
hand-edit the snapshot or generated client.
