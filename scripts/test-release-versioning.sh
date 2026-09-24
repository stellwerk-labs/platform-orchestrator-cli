#!/usr/bin/env sh
# Run inside the pinned semantic-release action image, with no network or secrets.
set -eu

fixture=$(mktemp -d)
trap 'rm -r "$fixture"' EXIT
git init --bare -b main "$fixture/remote.git" >/dev/null
git init -b main "$fixture/work" >/dev/null
cd "$fixture/work"
git config user.name 'Release fixture'
git config user.email 'release-fixture@example.invalid'
git config commit.gpgsign false
git remote add origin "$fixture/remote.git"
cp /checkout/.releaserc.json .releaserc.json
git add .releaserc.json
git commit -qm 'chore: initial fixture'
git tag v1.2.1
git push -qu origin main --tags

release() {
  /action/node_modules/.bin/semantic-release --no-ci --repository-url "$fixture/remote.git"
}

verify_version() {
  test "$(git rev-parse HEAD)" = "$(git --git-dir="$fixture/remote.git" rev-parse "refs/tags/$1")"
  # shellcheck source=outputs.sh
  . /action/outputs.sh
  test "$(release_version_at_head)" = "$1"
}

git commit --allow-empty -qm 'feat(modules)!: publish immutable versions'
git push -q origin main
release
verify_version v2.0.0

git commit --allow-empty -qm 'feat: extend commands'
git push -q origin main
release
verify_version v2.1.0

git commit --allow-empty -qm 'fix: correct a command'
git push -q origin main
release
verify_version v2.1.1

git commit --allow-empty -qm 'ci: correct publication wiring [skip release]'
git push -q origin main
release
test -z "$(release_version_at_head)"
echo 'PASS: semantic-release assigns major/minor/patch tags and honors skip, without a release publisher.'
