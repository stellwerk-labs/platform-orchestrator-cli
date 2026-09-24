# Orchestrator Module Management

These commands require an Orchestrator version that supports Module Management.
Authenticate and select your organization through the
normal CLI configuration; keep access tokens out of command history and files
committed to source control.

## Publish and explicitly promote

Create a stable catalogue identity bound to an existing immutable Resource Type:

```sh
octl create module-catalogue-entry --set-json '{
  "slug": "redis", "display_name": "Persistent Redis", "resource_type": "redis"
}' --idempotency-key redis-catalogue

octl create module-version redis --set-json @redis-1.0.0.json \
  --idempotency-key redis-publish-1.0.0
```

The publication file must contain the complete immutable definition, including
`semantic_version`, source, fixed inputs, parameters, provider mappings,
dependencies and co-provisioning. Use `octl create module-version --help` for the
current fields. External sources require an exact `source_revision`;
`artifact_digest` is optional, canonical and immutable when supplied. Inline
source must omit `artifact_digest`. An external digest is a publisher claim,
not proof of trusted verification in this release; omission remains Unverified.
The stable version metadata response represents an absent digest claim as
`artifact_digest: ""`. Omit the field on publication; do not send that empty
read sentinel as a digest claim.

When the Resource Type has a nonempty `output_schema`, every new publication
must include the same author-declared `output_schema`. Equality ignores object
key order and whitespace, but not array order. Existing versions are not rewritten.
Resource Types can also declare an optional immutable `module_contract`, an
offline, bounded OpenAPI 3.0 Schema Object over `module_inputs`, `module_params`,
`provider_mapping`, `dependencies`, `coprovisioned` and `output_schema`. This
validates declared interfaces, not the contents or runtime outputs of external
artifacts. `create resource-type --help` exposes this field; unknown publication
fields fail locally instead of being silently dropped. JSON/YAML version detail
and comparison responses retain the declaration; comparison tables show its diff.

Publication creates **Proposed**, not Default. To promote an eligible stable
version, read its current command revision and supply an audit reason:

```sh
octl get module-version redis 1.0.0
octl update module-version redis 1.0.0 --action promote \
  --expected-version 1 --reason 'Ready for default adoption' \
  --idempotency-key redis-promote-1.0.0
```

Replace the example revision `1` with the value you actually read. A stale command
fails rather than overwriting another operator's decision. Reuse an idempotency
key only for the exact same request, never for a later lifecycle cycle.

## Inspect history, comparison and adoption

```sh
octl get module-catalogue --include-archived
octl get module-versions redis --include-deprecated --include-defective
octl get module-version-history redis 1.0.0
octl get module-version-comparison redis 1.0.0 1.1.0
octl get module-version-usage redis 1.0.0
```

Default tables show lifecycle, identities, revisions and actual before/after
definition values. `--out json` or `--out yaml` retains the complete server response
for automation. Empty adoption does not prove a module has never been deployed;
the response includes observation time and unknown Environments.

Prereleases cannot become Default. Use `octl create stable-module-version` with
the exact prerelease revision, a reason and a complete stable definition to
publish a separate stable Proposed version and deprecate the prerelease atomically.
Its table output shows both immutable identities and their correlation ID.

## Protect an exact effective version

```sh
octl get module-version-pins --environment-uuid ENVIRONMENT_UUID
octl create module-version-pin --set-json '{
  "environment_uuid": "ENVIRONMENT_UUID",
  "module_uuid": "MODULE_UUID",
  "version_uuid": "EFFECTIVE_VERSION_UUID",
  "reason": "Hold the checkout migration baseline"
}' --idempotency-key checkout-pin
```

Replace the uppercase placeholders with real UUIDs. A Pin protects the version
already effective in that Environment; it does not deploy an arbitrary historical
version. Current scope-based permissions control notes, Unpin and Discard, not the
identity of the Pin's creator. Notes are append-only and do not change protection
or its activation boundary.

`get module-version-pin-bulk-preview` accepts a frozen `environment_uuids` set,
Module UUID and `pin`, `unpin` or `discard` action through `--set-json` or
`--set-yaml`. Review every returned row, then pass the exact preview fingerprint,
same set and reason to `create module-version-pin-bulk`. Later-created Environments
never inherit this snapshot. The server enforces all-or-nothing permissions and
concurrency, so the CLI does not reinterpret failed rows as partial success.

## Retention and migration

Published versions and their SemVer identities are retained. Archive a Module with
`update module-catalogue-status --action archive`, an expected catalogue revision
and reason. Only an unused empty shell can be hard-deleted. Resource Type contracts
are immutable; `update resource-type` archives or reactivates them, it does not
rewrite their interface.

Old `create module` and `update module` commands are compatibility authoring paths,
not automatic promotion. Prefer explicit catalogue/version/lifecycle commands.
Existing legacy versions remain readable as `v0`; never invent historical SemVer
or digest metadata to make an old configuration appear newly managed.

## Reproduce the CLI acceptance journey locally

Build with `make build`, configure `PO_API_URL` to a disposable loopback Orchestrator server
and set its test organization and credentials through the usual environment.
Then run:

```sh
bash scripts/test-module-management.sh "$PWD/octl"
```

The script exercises real publication, replay, comparison, promotion, Defective,
exact predecessor restoration, stable graduation, history, adoption and default
tables. Its random catalogue is archived afterward with permanent history retained.
It deliberately refuses non-loopback targets and does not reset a database.
