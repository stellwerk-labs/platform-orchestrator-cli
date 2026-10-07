# Data Plane contract

`spec.yaml` is a byte-identical copy of the Data Plane OpenAPI document and
the only input of the generated `dp` client.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `6dc9644c05d54125f97aea5c40e0ca84cddaa513`
- File: `openapi/spec.yaml`
- Git blob: `864cd4f3469ebd79e7ef9c5892ce1b78db2ac3ed`
- File SHA-256: `8c605ea21653d7da9fcdf60cb8d08190b19dc135cef07c4cc48d1a4e74eb27c1`

The commit is the Data Plane plugin foundation export (deployment queue,
schema 33) and is not published yet. Repin to the published commit at
release: copy `openapi/spec.yaml` from that commit unchanged, update commit,
blob and SHA-256 here, then run `make generate` and `make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand.
