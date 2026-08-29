#!/usr/bin/env bash
set -euo pipefail

readonly cp_revision=v2.1.0
readonly dp_revision=0ae0aa5d9763e454e9a92e58b63b06afa0cb4170
readonly iam_revision=b32bbebbbbfc1781e1cdc7308e9af35d91ae0118
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
check_spec stellwerk-labs/platform-orchestrator-dp "$dp_revision" clients/platform-orchestrator-dp/spec.yaml
check_spec stellwerk-labs/platform-orchestrator-iam "$iam_revision" clients/platform-orchestrator-iam/spec.yaml
