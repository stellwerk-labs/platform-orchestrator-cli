# Module authoring

A Draft lets you edit a Module before publishing its next Version. Its planned
SemVer remains available while you edit. Every save retains an authoring
revision, so you can inspect earlier source without overwriting it.

These commands use the same API contract as the Orchestrator. The server decides
whether the feature and your permissions are available. Saving does not execute
a deployment, and publishing does not promote the Version to Default.

Assume a Module named `volume` already exists. Create `draft.json`:

```json
{
  "reason": "Prepare the next storage implementation",
  "definition": {
    "semantic_version": "1.2.0",
    "module_source": "inline",
    "module_source_code": "output \"name\" { value = \"first attempt\" }",
    "module_params": {},
    "module_inputs": {},
    "provider_mapping": {},
    "dependencies": {},
    "coprovisioned": []
  }
}
```

If the Module's Resource Type declares outputs, include that exact
`output_schema` in `definition`. Use `resource_type_version` to select an exact
interface version. Omitting it selects version 1, even after later versions
have been published. An external source needs an exact `source_revision` before
publication. Its `artifact_digest` is optional; omit it for inline source.

```sh
octl create module-draft volume --set-json @draft.json \
  --idempotency-key volume-authoring-1
```

Keep the returned Draft UUID and `resource_version`. To save another attempt,
create `revision.json` with the new complete `definition`, a reason and
`expected_resource_version` set to the revision you last read. Keep
`semantic_version` at `1.2.0` if that is still the intended release.

```sh
octl update module-draft volume "$draft_id" --set-json @revision.json \
  --idempotency-key volume-authoring-2
octl get module-draft volume "$draft_id"
octl get module-draft volume "$draft_id" --revision 1
```

## Test a stored revision

Create a Module Rule scoped to the target Project and Environment before testing.
A Module without a Default requires both scopes on its Rule. It cannot use a
global Rule until it has a published Default.

Capture the reviewed revision for that Environment:

```sh
octl create module-draft-test example development volume "$draft_id" \
  --expected-version 2 --reason "Review the second storage attempt" \
  --idempotency-key volume-test-2 --out json > captured-test.json
module_uuid=$(jq -r .module_uuid captured-test.json)
snapshot_id=$(jq -r .id captured-test.json)
octl get module-draft-test example development volume "$snapshot_id"
```

The snapshot retains the exact definition, source and Resource Type interface.
Capture alone does not run anything. Deploy a manifest whose resources match the
Module's scoped Rule, explicitly selecting the captured snapshot:

```sh
octl deploy example development manifest.json \
  --draft-test "$module_uuid=$snapshot_id"
```

Repeat `--draft-test` for each Module you want to test. Use stable Module UUIDs
and captured snapshot UUIDs, not slugs or the moving Draft UUID. The CLI uses a
separate server operation for test deployments. An older server that lacks this
operation returns an error; the CLI never substitutes an ordinary deployment.

Save another attempt under the same intended `1.2.0`, then capture its new
reviewed revision with a new command key. Editing alone leaves the first test's
source and deployment unchanged. A queued test also keeps its original snapshot.
An external Git source must select a full commit matching `source_revision`
before capture; a moving branch is insufficient. A digest remains optional.

Fresh tests require both Module authoring and scoped deployment permissions.
The server checks them again when queued work starts. Pins remain authoritative;
a test cannot replace a pinned Version. Retained rollback requires a previously
successful execution of that exact snapshot in the same Environment.

## Publish after testing

Review the stored definition, then publish that exact revision. In this example,
one save followed creation, so its resource version is 2:

```sh
octl update module-draft-status volume "$draft_id" --action publish \
  --expected-version 2 --reason "Reviewed storage implementation" \
  --idempotency-key volume-publication-1
```

The result contains the closed Draft and its new immutable Proposed Version.
A stale revision or an existing Proposed rejects the entire operation. Published
content cannot be edited. To continue authoring, create another Draft.

To abandon an open Draft instead, use `--action discard` with its current
resource version and a reason. Its history remains available. List both open
and closed Drafts with `octl get module-drafts volume`.

## Resource Type interfaces

Read exact interfaces and their history:

```sh
octl get resource-type-version storage 1
octl get resource-type-versions storage
```

Publish the next interface with a complete JSON body containing
`version`, `expected_resource_version`, `reason`, `output_schema` and an optional
`module_contract`:

```sh
octl create resource-type-version storage --set-json @interface.json \
  --idempotency-key storage-interface-2
```

Choose the exact next interface number (starting at 2). The server rejects
skips, duplicate version identities and stale catalogue revisions. Existing Modules retain their
exact contract. Their definitions and historical deployments are not rewritten.
