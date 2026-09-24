# CLI release publication

Semantic-release assigns the next version and pushes its Git tag. It does not
create a GitHub release. GoReleaser uploads the signed assets to the draft
reserved by the workflow, publishes it, then updates Homebrew and Scoop.

The stable preflight rejects an already reserved release or existing versioned
image, checks that the checkout and tag match, requires reviewed release notes,
and verifies the package-channel token's access. A nonempty token is not proof
of authorization. Candidate publication retains its independent approval and
does not update stable channels.

## Local checks

Run generation before tests in a clean checkout. Generated test doubles are not
committed:

```sh
make generate
make test
make lint
python3 scripts/candidate-release_test.py
python3 scripts/stable-release-preflight_test.py
```

The pinned semantic-release action can run a real, network-isolated Git fixture:

```sh
docker run --rm --network none \
  --mount "type=bind,source=$PWD,target=/checkout,readonly" \
  --entrypoint sh SEMANTIC_RELEASE_ACTION_IMAGE \
  /checkout/scripts/test-release-versioning.sh
```

Build that image from the exact action revision in `.github/workflows/ci.yaml`.
The fixture checks major, minor, patch and skipped releases against a disposable
bare remote. No GitHub credentials or public writes are involved.

## Partial publication

Do not rerun the whole publisher after any asset or image was published. Record
the source revision, release IDs, image digest, asset checksums and signing
identity first. Recover only missing outputs from the existing verified build.
Never replace a published tag, rebuild its image under the same version or
overwrite an existing release asset to clear a red workflow.

The September 2026 failure came from two publishers: semantic-release created a
public release, then GoReleaser attempted to publish a second draft for that tag.
The explicit analyzer-only configuration prevents that collision. The Git
fixture fails if a GitHub publisher is reintroduced into semantic-release.
