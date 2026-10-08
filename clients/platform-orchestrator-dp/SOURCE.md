# Data Plane contract

`spec.yaml` is a byte-identical copy of the Data Plane OpenAPI document and
the only input of the generated `dp` client.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `2ab9b9ac6784dc31580fab0bb03b18ef2907f7df`
- File: `openapi/spec.yaml`
- Git blob: `4940df060d33276691b33b720e6a51592794944d`
- File SHA-256: `a4c2a3fc2f11684499c3ec2c2f0fb2d43b3cccbd50038a69f667cda3711a74a1`

The commit is the Data Plane plugin foundation export (deployment queue with
`queue` as the default, `halt_on_failure`, `pending_changes` on plans and dry
runs, Environment fence, schema 34) and is not published yet. Repin to the
published commit at release: copy `openapi/spec.yaml` from that commit
unchanged, update commit, blob and SHA-256 here, then run `make generate` and
`make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand.
