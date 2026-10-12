# Release inspection worklog

## 2026-10-12: OCI metadata and signer corrections

Changes follow [DECISIONS.md](DECISIONS.md): exact raw OCI JSON retention,
continued decoded-content scanning, recursive duplicate-key refusal, shared
producer/publisher inspection, and the verified explicit Cosign v2 pin. No
application/API, target architecture, release identity or publisher choreography
changes. No commit, push, image build, publication or signing was performed.

Verification used Go 1.26.2, `GOWORK=off`, `GOENV=off`, empty `GOFLAGS` and
`GOEXPERIMENT`, and `GOMAXPROCS=2`. Each command was bounded below five minutes.
The existing verified go-licenses v2.0.1 binary was on PATH; its module build
checksum was checked. Lint used exactly v2.11.4, GoReleaser exactly v2.18.1,
Gitleaks exactly 8.30.1 and the already cached digest-pinned Syft image.

Commands and results:

- `make generate`: PASS, no tracked generated-file changes.
- `make test`: PASS after normal clean-checkout prerequisites were supplied.
  The first attempt failed because ignored generated mocks and go-licenses on
  PATH were missing; no code or test requirement was weakened.
- `make lint`: PASS, zero issues at v2.11.4.
- `go test -count=1 ./scripts`: PASS; pinned lint on `./scripts`: zero issues.
- `python3 scripts/client_oci_test.py`: 14 PASS. Three new tests exercise exact
  index/manifest/config bytes, escaped private references, nested/escaped
  duplicate keys, both architecture fixtures and actual publisher reinspection.
  Seeded credentials fail both production scan and publisher reinspection.
- `python3 scripts/release_content_test.py`: 14 PASS (9.571s).
- `python3 scripts/qualify_release_test.py`: 10 PASS (4.542s), including actual
  cached pinned Syft against the real fixture binary, without network access.
- `python3 scripts/candidate-release_test.py`: 11 PASS.
- `python3 scripts/stable-release-preflight_test.py`: 4 PASS.
- `goreleaser check`: PASS, one configuration validated.
- In-memory mutation checks: four of four killed (duplicate-key refusal and
  exact index, manifest and config retention independently removed). No
  production source files were edited by the mutation harness.
- Official Cosign native binary checksum, `version --json`, `sign-blob --help`
  and `verify-blob --help`: PASS for the existing flag contract. No signing.

These are local control/fixture checks, not full multiarch artifact production,
hosted qualification, real keyless signing or public-consumer verification.
See [LEARNINGS.md](LEARNINGS.md) for the source of the corrected boundaries.
