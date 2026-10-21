# Core contract

`spec.yaml` is a byte-identical copy of the Control Plane OpenAPI document and
the only input of the generated `cp` client.

- Repository: `stellwerk-labs/platform-orchestrator-cp`
- Commit: `fc8059255bd16ff61b0e4d3db122efd34912d068`
- File: `openapi/spec.yaml`
- Git blob: `b8ddf111b54f363169798121e0ce1526f627cfe0`
- File SHA-256: `903b836a6acd02aa24aa0f9adb41093a65df7e1be8358d9f11baf1b2de906022`

The public source commit contains the Control Plane plugin contracts: platform
principals as identities, several organizations per installation, organization
mode, installation-wide licence grants and schema 48. This records the API
source, not an application image release. To update this snapshot, copy
`openapi/spec.yaml` from the selected public commit unchanged. Update the commit,
blob and SHA-256 here, then run
`make generate` and `make check-api-specs`.

Never edit `spec.yaml` or the generated client by hand, and never add schemas
that Core does not publish. Plugin APIs have their own contracts, for example
[Progressive Rollouts](../progressive-rollouts/SOURCE.md). Module publication,
definition/detail and comparison snapshots all come from the exact Core schema.
`output_schema` may be absent or a declared object, including `{}`;
`resource_type` remains a catalogue binding, not a publication-body field.
