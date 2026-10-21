# Data Plane contract

`spec.yaml` is a byte-identical copy of the Data Plane OpenAPI document and
the only input of the generated `dp` client.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `cad9716c8d441826b5fcb6b3715c32c153d86284`
- File: `openapi/spec.yaml`
- Git blob: `47cfe212334619403f40f75b078e277658ce01d0`
- File SHA-256: `410482810079b527428180a468ac943f98f1ca86368c8032d5eddc85791381ec`

The public source commit contains the Data Plane plugin contracts: deployment
queue with `queue` as the default, `halt_on_failure`, `reason_deployment_id` on
queue items, `pending_changes` on plans and dry runs, Environment fence and
schema 35. This records the API source, not an application image release. To
update this snapshot, copy `openapi/spec.yaml` from the selected public commit
unchanged. Update the commit, blob and SHA-256 here, then run `make generate`
and `make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand.
