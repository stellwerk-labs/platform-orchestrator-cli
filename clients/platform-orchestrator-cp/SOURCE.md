# Core contract

`spec.yaml` records the API contract for module management, plugin discovery and
invocation. It will be re-sourced against the public CP plugin-foundation export
commit and re-verified once that branch is published.

- spec SHA-256: `af2e0ee1dcdb8196cc25469287da3e331d225a14e72c5cb927d668742100c4d9`

Run `make generate`, then `make check-api-specs`. Never edit generated clients
or use the spec to add publication fields. Module publication, definition/detail
and comparison snapshots all come from the exact current Core schema.
`output_schema` may be absent or a declared object, including `{}`; `resource_type`
remains a catalogue binding, not a publication-body field.
