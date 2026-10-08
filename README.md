# octl

![Build Status](./badge.svg)

The Stellwerk CLI for platform-orchestrator-cp, platform-orchestrator-dp and platform-orchestrator-iam.

## License

Licensed under the EUPL-1.2. See [LICENSE](./LICENSE).

## Configuration

By default, `octl` connects to `https://api.stellwerk.localhost`.

Override the API URL with `PO_API_URL` or persist it in the local configuration:

```bash
octl config set-url https://api.stellwerk.localhost
```

The configuration file is stored at `~/.config/octl/config.yaml`.

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
