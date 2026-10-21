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
candidate tag, builds and signs the complete local release with GoReleaser's
publisher skipped, then snapshots the exact 47 local asset names, sizes and
SHA-256 digests. The workflow verifies the local annotated tag and public tag
object identify the approved source, creates a draft with the GitHub Releases
API, and captures the ID from that exact create response. It uploads each asset
to that numeric ID without replacement, then verifies the complete remote
inventory against the local snapshot. The exact image, signatures,
attestations and native CLI smoke are qualified while the release remains a
draft. Immediately before publishing it rechecks local and public tag identity,
then re-fetches and validates the same numeric release ID, tag, title, notes,
draft state, all 47 asset names, sizes and SHA-256 digests, and unique asset IDs
in that readback. Asset IDs are not persisted as cross-readback identity pins.
Only then does it publish that release ID as a prerelease with latest disabled
and verify the final state. Keeping the release a draft during qualification
also supports GitHub's immutable release model.

If a step fails before draft creation, no draft exists and the already-signed
Git tag remains consumed. Image-absence preflight only observes that the image
tag is absent; it does not reserve the tag. Inspect current release and image
state before acting. If asset upload or qualification fails after draft
creation, the numeric draft ID and any partial assets/image remain occupied. If
finalization or its readback fails, inspect the captured numeric release ID
and image because the release may already be published. Never rerun the
publisher, replace the release ID, move/delete the old tag, overwrite assets or
delete evidence. Prepare a separately approved `rc.N+1`. Existing stable
channels remain unchanged; the stable GoReleaser flow and its existing-draft
behavior are not changed by the candidate lifecycle.

The candidate checks do not replace independent verification of download
signatures, anonymous pulls, both image architectures and the matching server
contracts.

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
