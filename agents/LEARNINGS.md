# Release inspection learnings

- A digest-only ORAS copy can change an OCI image manifest descriptor into a
  different media type. Copy from the tagged index with an explicit platform,
  then validate the descriptor consumed by the SBOM generator.
- A base-image home path can occur in archived layer metadata without being a
  host-home disclosure. An exception needs exact path, byte hash, layer digest
  and entry-list agreement, with the original artifact bytes retained.
- Buildx may inspect the enclosing worktree even when the build context only
  contains compiled inputs. Disable that incidental Git discovery for Buildx
  alone, not for the broader release process.


- JSON parsing and reserialization are not byte-preserving scans. Duplicate
  keys can discard an earlier value; strict parsing must reject that ambiguity.
  Keep both original and decoded forms to cover raw and escaped content.
- Exercise the producer's real scan boundary and publisher reinspection, not
  only parser helpers. Digest-valid fixture graphs must update their manifest
  references and attached attestation subjects together.
- Clean checkouts need the normal `make generate` step for ignored client mocks
  and the verified collector on PATH before `make test`. Missing prerequisites
  are not reasons to change product code or skip required behavior.
- Use `finally` for per-subtest fixture cleanup so a deliberately failing
  mutation does not turn later subtests into output-directory errors.
- Verify immutable action commits against their official repository and inspect
  their implementation. A plausible forty-character pin can still be nonexistent.

## 2026-10-31: MCP integration

Cobra traversal hooks are explicitly enabled in main, so child pre-run handlers
alone cannot isolate stdio startup from root version checks/client setup. A tree
annotation bypasses root pre/post hooks. The stable source export omits ignored
client mocks; `make generate` is needed before existing command tests compile.
The module catalogue endpoint exposes stable module identities without fetching
legacy inline module source. SDK typed struct schemas reject additional fields.
Use the CI-pinned lint version; newer local lint versions produce different gates.
Resource Type `output_schema` is JSON Schema, so output field counts must inspect
its `properties` object, not count top-level schema keywords. Wire fixtures should
represent the real contract, including `type`, `properties` and other keywords.
