# Core contract and rollouts compatibility overlay

`spec.yaml` records the API contract for module management, plugin discovery and
invocation. It will be re-sourced against the public CP plugin-foundation export
commit and re-verified once that branch is published.

- spec SHA-256: `a8e547bdaaa17c544330c1b68f034b65673f095f935e8e8a94d1aa7a17cf7777`

`legacy-enterprise-spec.yaml` provides the Rollouts path family for the
compatibility overlay. The generator extracts only `/orgs/{orgId}/rollouts` paths
and their missing referenced components, and refuses to overwrite any current Core
path or component. Its 14 paths, 43 schemas and 3 parameters contain no new server
implementation or authorization.

- legacy-spec SHA-256: `41a1f9a231d9dc0c5850e381a44caa938330099b7ff730b0b834f8d950e2f1e1`

`scripts/legacy-rollouts-overlay.go` verifies both source byte pins, extracts
only the Rollouts path family, and refuses to overwrite any current Core path or
component. The existing oapi-codegen v2.4.1 overlay support applies this layer
before normal generation. Both source contracts are checked against immutable
content pins; tests verify exact fragments, reference reachability and no Core
overwrite.

`legacy-rollouts-overlay.yaml` SHA-256:
`8f04c9a645cde540cf5b4be00156b71f257fc254f478163a30d2fc03ca531afc`

Run `make generate`, then `make check-api-specs`. Never edit generated clients
or use the legacy overlay to add publication fields. Module publication,
definition/detail and comparison snapshots all come from the exact current Core
schema. `output_schema` may be absent or a declared object, including `{}`;
`resource_type` remains a catalogue binding, not a publication-body field.
