# CLI release candidates

The normal stable release keeps its signed downloads, exact/version-family/latest
image tags and Homebrew/Scoop publication. Candidates use a separate manually
approved job. Pushing an RC tag alone never publishes a candidate.

## Publication prerequisites

Obtain specific approval for the source/tag, GitHub release and public GHCR
publication. The workflow must first be available on the default branch. Export
workflow-only changes separately with `[skip release]` on every bootstrap and
merge/squash commit, and check for other pending releasable commits before merging.
This documentation does not authorize any public action.

An administrator must provision `public-release-candidate` with required reviewers
and the repository's intended branch/tag restrictions. Its absence, absent
reviewers, existing draft/published release reservation or ambiguous registry
access fail closed. No environment or package visibility setting is created by
the workflow.

The candidate tag must already exist, canonically `vX.Y.Z-rc.N` with `N >= 1`.
Include reviewed `docs/releases/<tag>.md` in the tagged revision. Record its exact
40-character SHA. Dispatch CI using `release_tag` and `candidate_sha`. All test
jobs check out that exact SHA; mismatched tags and checkouts are rejected before
publication. The existing stable job does not run for RC tags or RC dispatch.

## Candidate outputs and isolation

The candidate job preserves the existing Cosign signing and checksum assets. It
publishes a GitHub prerelease without changing GitHub latest and only
`ghcr.io/stellwerk-labs/octl:X.Y.Z-rc.N` for Linux amd64/arm64, without the `v` tag
prefix. It does not publish moving major/minor/latest image tags or update
Homebrew/Scoop. It never receives the cross-repository `GH_PAT` secret; its token
is limited to this repository. GoReleaser is pinned to 2.18.1 and receives the
explicit approved current tag so another tag on the same commit cannot select a
different release version.

These policies use the official GoReleaser
[empty Docker-tag handling](https://goreleaser.com/customization/package/dockers_v2/),
[Homebrew](https://goreleaser.com/customization/publish/homebrew_casks/) and
[Scoop](https://goreleaser.com/customization/publish/scoop/) prerelease upload rules,
and [release metadata](https://goreleaser.com/customization/publish/scm/).

The job first verifies that no draft or published release already reserves the
candidate tag, then reserves an existing-tag GitHub draft before publishing.
GoReleaser reuses that draft, uploads signed assets and then publishes it as a
prerelease. Keeping it a draft during upload also supports GitHub's immutable
release model. Existing stable channels remain unchanged; GoReleaser may also
reuse an existing stable draft instead of attempting to create a second release
for that tag. An existing candidate image or release is not overwritten. If
publication then fails, inspect the evidence and prepare a separately approved
`rc.N+1`; do not move/delete the old tag, delete its evidence or silently reuse
the consumed identifier.

The post-publication check verifies exact tag, prerelease/non-draft status and
signed-asset presence, then inspects/runs the exact candidate image. This is not a
replacement for independently verifying download signatures, anonymous pulls,
both image architectures and the matching server contracts.

## Local gates

```sh
go test ./scripts -count=1
PYTHONDONTWRITEBYTECODE=1 python3 scripts/candidate-release_test.py
goreleaser check
```

Go tests evaluate the real configured templates for stable and candidate tags.
Python tests cover input/environment guards, workflow separation and metadata
validation. `goreleaser check` validates configuration only. None of these commands
publishes, builds images or proves remote environment/registry configuration.
Normal generated API-spec checks must also use the final public CP/DP/IAM source
revisions before the candidate is publishable.
