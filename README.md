# octl

![Build Status](./badge.svg)

The Stellwerk CLI for platform-orchestrator-cp, platform-orchestrator-dp and platform-orchestrator-iam.

## License

Release archives and the CLI release image include `licenses/`: the application's EUPL
text, Go's license/patent grants, and the union of dependency notices for every
configured GoReleaser target. Collection uses checksum-verified go-licenses
v2.0.1 and Go 1.26.2, without changing the module's minimum Go version. Unknown
licenses and missing/empty legal notices fail the build. Reciprocal-license
source files saved by go-licenses are preserved too. Generated notices are
replaced only when their recorded checksums match and no untracked files exist.

One mechanical classifier correction is restricted to the exact unreplaced
`github.com/oapi-codegen/nullable@v1.2.0`, its module checksum and original LICENSE
hash: the upstream file contains the Apache-2.0 copyright/header but not the full
license, and go-licenses excludes header-only matches. Its original bytes are
preserved alongside the hash-pinned full Apache license from the Apache Software
Foundation and a provenance record. Any changed identity/content or colliding
module prefix fails; other unknown licenses remain errors. This documents
packaging behavior, not a legal or compatibility certification.

Run `go test -count=1 ./scripts/licensecollector` for the real-classifier notice
and failure-boundary tests. The test installs the same verified tool into its
temporary directory if necessary. GoReleaser's before-hook generates notices;
archive entry timestamps and binary mtimes use the source commit date, CLI
binaries use `-trimpath`, and the release image base is digest-pinned.

Licensed under the EUPL-1.2. See [LICENSE](./LICENSE).

## Configuration

By default, `octl` connects to `https://api.stellwerk.localhost`.

Override the API URL with `PO_API_URL` or persist it in the local configuration:

```bash
octl config set-url https://api.stellwerk.localhost
```

The configuration file is stored at `~/.config/octl/config.yaml`.

Log in with `octl login`, or store a token, for example a service user's, with
`octl config set-token`. It reads the token from a file or from standard input,
so the token does not end up in shell history or the process list:

```bash
octl config set-token --token-file service-user.token
octl config set-token --token-file -    # paste the token, then press Ctrl-D
```

A token given as an argument still works, with a warning. `PO_AUTH_TOKEN`
overrides the stored token.

Progressive Rollouts uses Core's plugin discovery and invocation path. Each
operation must be advertised by organization-scoped plugin discovery. Requests and
responses use the plugin's own API contract in `clients/progressive-rollouts`; the
Core contract contains no Progressive Rollouts types.

Rollout logs and outputs use Core's protected material route, never the plugin
invocation endpoint. Supply the immutable receipt scope with `--project-uuid`
and `--environment-uuid`, or let the CLI resolve it from the Rollout's frozen
Plan while the detail operation remains available.

For a private API certificate authority, set `PO_CA_FILE` to a bounded PEM CA
bundle. The CLI appends it to the operating-system trust roots and still
requires normal hostname verification and TLS 1.2 or newer.

## Deployment runner logs

Runner logs are encrypted with a key created for each deployment. `octl deploy`,
`octl rollback` and `octl score deploy` print that key only with
`--print-logs-key`, after a warning. `octl logs` reads it from a file or from
standard input, never from the command line, where it would end up in shell
history and the process list:

```bash
octl logs <deployment-id> --key-file runner-logs.key
printenv RUNNER_LOGS_KEY | octl logs <deployment-id> --key-file -
```

## Install

Install directly into your Go bin (your `$PATH` should include `$GOPATH/bin`):

```bash
go install .
```

Verify the install:

```bash
octl --help
```

## Orchestrator Module Management

Use [immutable Module versions and Pins](docs/module-management.md) to publish,
promote, compare and inspect versions in the Orchestrator. Existing mutable Module authoring and Resource Type
updates must migrate to the new lifecycle commands.

## Collaboration

CLI conventions follow `kubectl` where possible.

**Install dependencies & run generate:**

```bash
make install
make generate
```

**Test & build:**

```shell
make build

make lint
make test
```

`make check-api-specs` checks every vendored contract against the SHA-256 recorded
in its `SOURCE.md` ([Core](clients/platform-orchestrator-cp/SOURCE.md),
[Data Plane](clients/platform-orchestrator-dp/SOURCE.md),
[Progressive Rollouts](clients/progressive-rollouts/SOURCE.md)) and then compares the
Core, Data Plane and IAM documents with their recorded upstream revisions, which
needs network access.
