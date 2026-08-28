#!/usr/bin/env bash
set -euo pipefail

readonly cp_version=v2.1.0
readonly dp_version=v3.1.0
readonly iam_version=v2.3.0
readonly temp_dir="$(mktemp -d)"
trap 'rm -rf -- "$temp_dir"' EXIT

check_spec() {
  local repository="$1"
  local version="$2"
  local vendored_spec="$3"
  local downloaded_spec="$temp_dir/$repository.yaml"
  local normalized_vendored="$temp_dir/$repository-vendored.yaml"
  local normalized_downloaded="$temp_dir/$repository-downloaded.yaml"

  curl --fail --silent --show-error --location \
    "https://raw.githubusercontent.com/stellwerk-labs/$repository/$version/openapi/spec.yaml" \
    --output "$downloaded_spec"

  awk 'NF { while (blank > 0) { print ""; blank-- } print; next } { blank++ }' "$vendored_spec" >"$normalized_vendored"
  awk 'NF { while (blank > 0) { print ""; blank-- } print; next } { blank++ }' "$downloaded_spec" >"$normalized_downloaded"

  if ! cmp --silent "$normalized_vendored" "$normalized_downloaded"; then
    echo "$vendored_spec differs from stellwerk-labs/$repository@$version/openapi/spec.yaml" >&2
    return 1
  fi
}

check_spec platform-orchestrator-cp "$cp_version" clients/platform-orchestrator-cp/spec.yaml
check_spec platform-orchestrator-dp "$dp_version" clients/platform-orchestrator-dp/spec.yaml
check_spec platform-orchestrator-iam "$iam_version" clients/platform-orchestrator-iam/spec.yaml
