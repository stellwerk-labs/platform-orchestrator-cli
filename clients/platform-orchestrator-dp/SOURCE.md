# Data Plane contract

`spec.yaml` is a byte-identical copy of the Data Plane OpenAPI document and
the only input of the generated `dp` client.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `c101ee24c0171eea581899d131fcb61dfa5560b7`
- File: `openapi/spec.yaml`
- Git blob: `47cfe212334619403f40f75b078e277658ce01d0`
- File SHA-256: `410482810079b527428180a468ac943f98f1ca86368c8032d5eddc85791381ec`

The commit is the Data Plane plugin foundation export (deployment queue with
`queue` as the default, `halt_on_failure`, `reason_deployment_id` on queue
items, `pending_changes` on plans and dry runs, Environment fence, schema 35)
and is not published yet. Repin to the published commit at release: copy
`openapi/spec.yaml` from that commit unchanged, update commit, blob and
SHA-256 here, then run `make generate` and `make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand.
