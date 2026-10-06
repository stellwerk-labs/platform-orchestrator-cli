# Core contract

`spec.yaml` is a byte-identical copy of the Control Plane OpenAPI document and
the only input of the generated `cp` client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `adf840141b953615551456640e2c15ccd9af4d3c`
- File: `openapi/spec.yaml`
- Git blob: `fae60471ba92c78d56b6310524ef3d7aafdf7105`
- File SHA-256: `ecab1b562efe7a5ca16ea137f7cb1bf48d4b38c9adca6f114f977c1b413a67f4`

The commit is the Control Plane plugin foundation export and is not published
yet. Repin to the published commit at release: copy `openapi/spec.yaml` from
that commit unchanged, update commit, blob and SHA-256 here, then run
`make generate` and `make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand, and never add schemas
that Core does not publish. Plugin APIs have their own contracts, for example
[Progressive Rollouts](../progressive-rollouts/SOURCE.md). Module publication,
definition/detail and comparison snapshots all come from the exact Core schema.
`output_schema` may be absent or a declared object, including `{}`;
`resource_type` remains a catalogue binding, not a publication-body field.
