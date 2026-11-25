# Author, publish and select project pipelines

> **RC feature:** This page describes Deployment Pipelines in the coordinated
> release candidate (CLI `2.6.0-rc.1`). Pipelines are not available in the
> current stable release from `main`. Use matching Pipeline RC components; Native
> execution requires explicit operator activation.

Pipeline documents use the same `{slug, name, definition}` shape as the Console and API. Creating or editing a pipeline only changes its draft. Validate, export and update the reviewed revision, then publish explicitly:

```sh
octl validate pipeline my-project --file reviewed.yaml
octl create pipeline my-project --file reviewed.yaml
octl get pipelines my-project
octl get pipeline my-project DRAFT_UUID --out yaml > pipeline.yaml
octl get pipeline my-project DRAFT_UUID --details
# Edit pipeline.yaml, then use the exact resource_version reviewed above.
octl update pipeline my-project DRAFT_UUID --file pipeline.yaml --expected-version 1
octl get pipeline my-project DRAFT_UUID --details
octl create pipeline-version my-project DRAFT_UUID --expected-version 2 --request-key reviewed-release-2
octl get pipeline-version my-project VERSION_UUID
```

Reuse the same revision/key after a lost response. A different reviewed revision needs a new publication key. A retired version remains readable; it cannot be reactivated by replaying publication.

Read policy before changing it, then write the reviewed CAS revision explicitly:

```sh
octl get pipeline-policy my-project
octl update pipeline-policy my-project --file policy.yaml
octl get pipeline-choices my-project my-environment
```

`policy.yaml` contains:

```yaml
expected_resource_version: 0 # Replace with the current policy resource_version.
enabled: true
allowed_version_ids:
  - 11111111-1111-4111-8111-111111111111
  - 22222222-2222-4222-8222-222222222222
default_version_id: 11111111-1111-4111-8111-111111111111
rollback_defaults:
  default_version_id: 22222222-2222-4222-8222-222222222222
```

Replace sample UUIDs with published versions from this project. Forward and rollback defaults are independent. Omitting `rollback_defaults` preserves existing rollback preferences; `{}` explicitly clears them. A CAS conflict requires reading and reviewing current policy again, not silently retrying a write.

Remove a version from the policy before retiring it, then archive the unused draft only after all its versions are retired:

```sh
octl delete pipeline-version my-project VERSION_UUID
octl delete pipeline my-project DRAFT_UUID --expected-version 2
```

Lifecycle and discovery commands return JSON by default; use `--out yaml` for YAML. Draft source limits remain 32 KiB; project policy accepts up to 256 KiB for bounded per-environment exceptions. Secrets belong in runner-side references, not pipeline documents.

Draft slugs are immutable. Export omits server identity unless `--details` is selected; YAML comments are not retained. Conflicts never auto-adopt an existing draft or overwrite a newer revision.

Choose an immutable published version for ordinary deployment entrypoints with one selector:

```sh
octl deploy my-project my-environment manifest.yaml --pipeline-version VERSION_UUID
octl deploy my-project my-environment manifest.yaml --plan-only --pipeline-version VERSION_UUID
octl score deploy my-project my-environment score.yaml --pipeline-version VERSION_UUID
octl rollback my-project my-environment PREVIOUS_DEPLOYMENT_UUID --pipeline-version ROLLBACK_VERSION_UUID
```

The same flag works with captured `--draft-test MODULE_UUID=SNAPSHOT_UUID` selections. Omitting it leaves directional default selection to the server. The CLI reads the exact Core publication, rejects retired/mismatched versions, and sends ordinary DP `pipeline: {id: VERSION_UUID, version: NUMERIC_VERSION}`. It never supplies executable recipes or digests; the server checks current policy, compatibility and admission. Core/plugin `{version_id, digest}` references use a different contract and cannot replace the ordinary selector. A policy change after lookup still fails safely at admission; no client fallback bypasses it.
