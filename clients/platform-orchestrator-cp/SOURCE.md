# Core contract

`spec.yaml` is a byte-identical copy of the Control Plane OpenAPI document and
the only input of the generated `cp` client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `035f40519233a30cd84c2b3a573cb88e112f5d14`
- File: `openapi/spec.yaml`
- Git blob: `e045438468651594548cd55974062c4af3d85f20`
- File SHA-256: `74f902245f4df240b6b078acb0c98176e67cb577394614dd0d38313c9994bc4e`

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
