# octl

![Build Status](./badge.svg)

The Stellwerk CLI for platform-orchestrator-cp, platform-orchestrator-dp and platform-orchestrator-iam.

## License

Licensed under the EUPL-1.2. See [LICENSE](./LICENSE).

## Open tasks

- Set the correct default API URL in `internal/command/root.go`.

## Install

Install directly into your Go bin (your `$PATH` should include `$GOPATH/bin`):

```bash
go install .
```

Verify the install:

```bash
octl --help
```

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
