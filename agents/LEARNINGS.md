# Release inspection learnings

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
