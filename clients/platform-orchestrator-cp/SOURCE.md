# Core contract

`spec.yaml` is a byte-identical copy of the Control Plane OpenAPI document and
the only input of the generated `cp` client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `b66da2b15bddde4a0a60a6402c172bd82eb26598`
- File: `openapi/spec.yaml`
- Git blob: `63560bc0aa982d567d99082501210bb526419cf2`
- File SHA-256: `4a3cdbce0c470603244a869ded37c65fcf10182db6f14ae0e7f9a9a1ed24d259`

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
