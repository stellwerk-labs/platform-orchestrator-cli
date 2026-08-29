#!/usr/bin/env bash
set -euo pipefail

readonly cp_revision=v2.1.0
readonly dp_revision=46d92a65f50c2bb4f44d61493c30d875c1a36d17
readonly iam_revision=342c1b1f9d06e801319c198587e6d337235c2091
readonly temp_dir="$(mktemp -d)"
trap 'rm -rf -- "$temp_dir"' EXIT

check_spec() {
  local repository="$1"
  local revision="$2"
  local vendored_spec="$3"
  local repository_name="${repository##*/}"
  local downloaded_spec="$temp_dir/$repository_name.yaml"
  local normalized_vendored="$temp_dir/$repository_name-vendored.yaml"
  local normalized_downloaded="$temp_dir/$repository_name-downloaded.yaml"

  curl --fail --silent --show-error --location \
    "https://raw.githubusercontent.com/$repository/$revision/openapi/spec.yaml" \
    --output "$downloaded_spec"

  awk 'NF { while (blank > 0) { print ""; blank-- } print; next } { blank++ }' "$vendored_spec" >"$normalized_vendored"
  awk 'NF { while (blank > 0) { print ""; blank-- } print; next } { blank++ }' "$downloaded_spec" >"$normalized_downloaded"

  if ! cmp --silent "$normalized_vendored" "$normalized_downloaded"; then
    echo "$vendored_spec differs from $repository@$revision/openapi/spec.yaml" >&2
    return 1
  fi
}

check_spec stellwerk-labs/platform-orchestrator-cp "$cp_revision" clients/platform-orchestrator-cp/spec.yaml
check_spec jayonthenet/platform-orchestrator-dp "$dp_revision" clients/platform-orchestrator-dp/spec.yaml
check_spec jayonthenet/platform-orchestrator-iam "$iam_revision" clients/platform-orchestrator-iam/spec.yaml
