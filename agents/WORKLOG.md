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

## 2026-10-17: generic client image export correction

Prepared an isolated source change for the Buildx invocation,
platform-selected ORAS copy, and narrowly pinned Distroless metadata scan.
Added real ORAS, byte-bound scanner, and subprocess-environment regressions,
and installed ORAS in the public CI test job. No client Go code, API inputs,
publisher semantics, tags, signing or publication were changed. Local test
results: complete OCI (15), content scanner (15), release qualification (11),
candidate release (11), and stable preflight (4) Python helper suites passed.
Python compilation and workflow YAML parsing passed. The preliminary PyYAML
check could not run because that optional module was absent; the available
`yq` parser passed without adding a dependency.

Standard gates also passed with `GOTOOLCHAIN=go1.26.2 GOWORK=off GOENV=off
GOFLAGS='' GOEXPERIMENT='' GOMAXPROCS=2`, the original PATH retained and
golangci-lint2.11.4 prefixed: `make generate` (no generated drift), `make test`
(74.7% total coverage), and `make lint` (zero issues). Each completed under
270seconds. No exact Go test count was collected for this run. Full helper
commands were `python3 scripts/client_oci_test.py`,
`python3 scripts/release_content_test.py`, `python3 scripts/qualify_release_test.py`,
`python3 scripts/candidate-release_test.py`, and
`python3 scripts/stable-release-preflight_test.py` with the pinned content tools
and Go toolchain. `git diff --check` passed.

Root reviewed all nine changed paths. Application/Go/API/generator inputs and
release publication semantics are unchanged. The metadata allowance fails
closed on absent, malformed or mismatched evidence. This is a local source
correction, not a shipping artifact, public contract consumer or release claim.

## 2026-10-31: read-only MCP implementation

Based on released CLI source 69f45b6b3c1e1fc745bfc05b7d36b8fb096eb609 in an
isolated private-remote checkout. Implemented eight typed read-only metadata
tools, bounded secure generated-client transport, shared configuration resolution,
stdio command lifecycle, stderr errors and usage documentation. SDK pin and
security boundaries are recorded in DECISIONS.md; hook/generator/lint findings
are in LEARNINGS.md. Validation includes real SDK in-memory and subprocess stdio
sessions, generated-client wire GET/scope checks, rejected arguments, secret
redaction, redirects, cancellation, payload/count limits and loopback token
requirements. Full CI-pinned checks and separate security review are recorded in
the external task evidence directory before the local signed commit. Only the verified private feature branch may be pushed; no public push or
publication is part of this iteration.

Final local validation uses Go1.26.10 and the CI-pinned golangci-lint2.11.4:
`make test` passed599tests; `make lint` passed0issues; `make build`, targeted
race tests, format/diff checks, deterministic tidy, API-spec checks and actual
multi-platform dependency notice collection passed. Release content and OCI
contract suites passed; final qualification suite evidence is kept externally.
The first full test attempt lacked the verified go-licenses PATH prerequisite;
the first qualification attempt lacked the exact pinned Syft Docker image. Both
prerequisites were restored without changing product code. Root review corrected
the Resource Type field count to inspect JSON Schema properties, with real wire
cases for one, two and absent properties. No public writes were performed.

Root source and independent security review accepted the implementation. Source
and built-binary govulncheck passed with no vulnerable imported packages or
symbols; the module-only x/crypto/openpgp finding is not reachable. Documentation
polish removed internal publication workflow language from product docs.
