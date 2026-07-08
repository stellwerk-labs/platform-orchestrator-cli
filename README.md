# octl

![Build Status](./badge.svg)

The Stellwerk CLI for platform-orchestrator-cp, platform-orchestrator-dp and platform-orchestrator-iam.

## License

Licensed under the EUPL-1.2. See [LICENSE](./LICENSE).

## Followups

The following placeholders and `FIXME`/`TODO` comments remain in the codebase and must be addressed before the final release:

- **`internal/command/root.go`:** `defaultApiUrl = "[Default api url]"`
- **`internal/version_checker.go`:** `versionCheckDocsURL = "[Documentation url]"`
- **`.goreleaser.yaml`:** `homepage: "[Documentation url]"`, `email: [Support email]`
- **`.goreleaser.yaml`:** `# FIXME: Add back when tap and scoop will be configured.

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
