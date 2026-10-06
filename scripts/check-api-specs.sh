#!/usr/bin/env bash
set -euo pipefail

readonly dp_revision=0ae0aa5d9763e454e9a92e58b63b06afa0cb4170
readonly iam_revision=b32bbebbbbfc1781e1cdc7308e9af35d91ae0118
temp_dir="$(mktemp -d)"
readonly temp_dir
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

# source_value FILE LABEL prints the value of the single "- LABEL: `value`" line.
source_value() {
  local value
  value="$(sed -n "s/^- $2: \`\([^\`]*\)\`\$/\1/p" "$1")"
  if [[ -z "$value" || "$value" == *$'\n'* ]]; then
    echo "$1 must record exactly one $2" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

check_spec_sha() {
  local vendored_spec="$1"
  local expected_sha="$2"
  local label="$3"
  local actual_sha
  actual_sha="$(shasum -a 256 "$vendored_spec" | awk '{print $1}')"
  if [[ "$actual_sha" != "$expected_sha" ]]; then
    echo "$label: $vendored_spec SHA-256 $actual_sha does not match recorded $expected_sha" >&2
    return 1
  fi
}

# Each vendored contract records its source and digest in SOURCE.md.
cp_revision="$(source_value clients/platform-orchestrator-cp/SOURCE.md Commit)"
cp_spec_sha="$(source_value clients/platform-orchestrator-cp/SOURCE.md 'File SHA-256')"
check_spec_sha clients/platform-orchestrator-cp/spec.yaml "$cp_spec_sha" "Core contract"
rollouts_spec_sha="$(source_value clients/progressive-rollouts/SOURCE.md 'File SHA-256')"
check_spec_sha clients/progressive-rollouts/spec.yaml "$rollouts_spec_sha" "Progressive Rollouts contract"

check_spec stellwerk-labs/platform-orchestrator-cp "$cp_revision" clients/platform-orchestrator-cp/spec.yaml
check_spec stellwerk-labs/platform-orchestrator-dp "$dp_revision" clients/platform-orchestrator-dp/spec.yaml
check_spec stellwerk-labs/platform-orchestrator-iam "$iam_revision" clients/platform-orchestrator-iam/spec.yaml
