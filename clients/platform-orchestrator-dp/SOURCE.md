# Data Plane contract

`spec.yaml` is a byte-identical copy of the Data Plane OpenAPI document and
the only input of the generated `dp` client.

- Repository: `stellwerk-labs/platform-orchestrator-dp`
- Commit: `4384d1fa309dc226a11379b58229070ddf3772af`
- File: `openapi/spec.yaml`
- Git blob: `dc4c389f9887d096f8b79ff07fee1dd8e371a8db`
- File SHA-256: `51e9639a5ba2ec9196701255d5b64f2d8b51b4e2e5430261c8fb177c00b82a0e`

The commit is the Data Plane plugin foundation export (deployment queue and
Environment fence, schema 33) and is not published yet. Repin to the
published commit at release: copy `openapi/spec.yaml` from that commit
unchanged, update commit, blob and SHA-256 here, then run `make generate` and
`make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand.
