# Release inspection decisions

## 2026-10-17: keep client image corrections generic

Scope Buildx Git discovery suppression to the precompiled image build subprocess;
do not change ambient release tooling or the producer statement. Select a
platform manifest through the tagged OCI index when copying for image SBOMs,
and validate that ORAS retained the exact manifest descriptor and media type.
The runtime-home scanner allowance applies only to the byte-identical pinned
Distroless nonroot layer metadata on either target architecture. All other
paths, bytes, and scanner checks remain mandatory. The public CI test job must
install the already-pinned ORAS binary before running the real regression.


## 2026-10-12: inspect exact OCI JSON without ambiguity

Retain original layout marker, transport index, image index, manifests and
configs in the scanned inspection tree. Keep decoded JSON projections too:
original bytes alone would miss private references written with JSON escapes.
Reject duplicate object keys at every nesting level before admitting OCI JSON,
including duplicates whose key spellings differ only by escape sequences.
The producer and publisher reinspection use the same export-and-scan boundary.
No image rebuild, new scanner exception or publication-order change is involved.

## 2026-10-12: restore the existing Cosign v2 interface

Both release paths use official `sigstore/cosign-installer` v3.10.1, immutable
commit `7e8b541eb2e61bf99390e1afd4be13a184e9ebc5`, with explicit
`cosign-release: v2.6.1`. The official tag resolves to this verified commit;
its action implementation checksum-verifies the same bootstrap version.
This preserves the existing separate certificate/signature assets and GitHub
OIDC identity verification. Switching to Cosign v3 bundle delivery is outside
this correction. No keyless signing or transparency-log operation was tested.

Sources:

- [Immutable installer implementation](https://github.com/sigstore/cosign-installer/blob/7e8b541eb2e61bf99390e1afd4be13a184e9ebc5/action.yml)
- [Cosign v2.6.1 blob-signing contract](https://github.com/sigstore/cosign/blob/v2.6.1/doc/cosign_sign-blob.md)

The installed official Darwin/arm64 binary matched action and release-asset
SHA256 `54047052cf46f40a5c3c95a510db276e164ba77e096aea1ca1b733f770359689`.
Its version and help expose the existing sign-blob and verify-blob flags.
The Linux/amd64 asset digest independently matches the action bootstrap pin
`064954c5d8c7e3b28188eee5b1727b31c411550bc5fefd41aa672d3c761d103a`;
that platform binary was not executed locally.

## 2026-10-31: local read-only MCP metadata

Use official `github.com/modelcontextprotocol/go-sdk` v1.8.0 (reviewed upstream
pin), typed handlers and generated CP clients. Keep the existing Go directive and
release toolchain pins. Expose eight fixed metadata tools over stdio only, with
explicit organization/project/module scope. Metadata DTOs exclude schema values,
configuration, code, outputs, labels, arbitrary descriptions and status messages.
A dedicated transport avoids CLI debug-body logging, rejects non-GET requests,
redirects and nonconfigured origins, bounds bodies and requires an existing
Bearer token even on localhost. This avoids synthesizing a privileged internal
identity. Root hooks skip version checks and interactive setup for MCP; the
configuration resolver remains shared with normal CLI commands.

The inherited module graph selected andybalholm/brotli v1.1.0. After verifying
upstream GHSA-c9gr-gmg4-2vqv and official v1.2.7 proxy/SumDB provenance, narrowly
pin v1.2.7 in this private branch. It is not an octl application package dependency
according to `go mod why -m`; the inherited/tool graph still must meet the user's
unaffected-dependency condition. The SDK did not introduce Brotli. Do not modify
the already published v2.2.0 source or artifacts.
