# Core contract

`spec.yaml` is a byte-identical copy of the Control Plane OpenAPI document and
the only input of the generated `cp` client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `301e2448ae8913e61dcc27d4ab75fc1d1046bcae`
- File: `openapi/spec.yaml`
- Git blob: `cf62f4e2fbdbb2218edc261d495598b788e73159`
- File SHA-256: `6c1c61a0382e9714b12dc0c12bffb47faa39a95a2e10776cfee208cdc706dfbb`

The commit is the Control Plane plugin foundation export (platform principals
as identities, several organizations per installation, organization mode,
installation-wide licence grants, schema 48) and is not published yet.
Repin to the published commit at release: copy `openapi/spec.yaml` from that
commit unchanged, update commit, blob and SHA-256 here, then run
`make generate` and `make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand, and never add schemas
that Core does not publish. Plugin APIs have their own contracts, for example
[Progressive Rollouts](../progressive-rollouts/SOURCE.md). Module publication,
definition/detail and comparison snapshots all come from the exact Core schema.
`output_schema` may be absent or a declared object, including `{}`;
`resource_type` remains a catalogue binding, not a publication-body field.
