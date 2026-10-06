# Progressive Rollouts plugin API contract

`spec.yaml` is a byte-identical copy of the `v1alpha` client contract of the
Enterprise plugin Progressive Rollouts (`dev.stellwerk.progressive-rollouts`),
"Stellwerk Progressive Rollouts plugin API". The plugin owns this contract.
Core publishes no Rollouts type.

- Contract: `progressive-rollouts-v1alpha.openapi.yaml`
- Contract level: `v1alpha`
- File SHA-256: `4d93b69cbbe2fbfe78f63364a1a59146ce6f51a1e8a77a095524be1ef5b69b23`

The document has no `paths`. Its `x-stellwerk-operations` list holds the 15
plugin operations as OpenAPI Operation Objects with their method, mutation flag,
licence feature and safety path values. The CLI invokes them by `operationId`
through Core's `invokePluginOperation` after Core plugin discovery, so this
package contains models only.

`ModuleVersionSemanticStatus` and `ModuleVersionPinStatus` mirror Core value
sets and carry `x-stellwerk-core-schema`. Tests require them to equal the
vendored Core contract.

Replace `spec.yaml` only with a released revision of the contract, never by
hand. Then update the SHA-256 here and run `make generate` and
`make check-api-specs`. The plugin implementation is commercial and not part
of this repository.
