#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 VENDORED_SPEC SOURCE_METADATA" >&2
  exit 2
fi

readonly vendored_spec="$1"
readonly source_metadata="$2"

extracted_metadata="$(grep -E '^- extracted .* SHA-256: .[0-9a-f]{64}.$' "$source_metadata" || true)"
expected_extracted_sha="${extracted_metadata##* }"
expected_extracted_sha="${expected_extracted_sha#\`}"
expected_extracted_sha="${expected_extracted_sha%\`}"
if [[ ! "$expected_extracted_sha" =~ ^[0-9a-f]{64}$ ]]; then
  echo "$source_metadata must record exactly one extracted spec SHA-256" >&2
  exit 1
fi
actual_extracted_sha="$(shasum -a 256 "$vendored_spec" | awk '{print $1}')"
if [[ "$actual_extracted_sha" != "$expected_extracted_sha" ]]; then
  echo "$vendored_spec differs from the neutral native Core contract recorded in $source_metadata" >&2
  exit 1
fi

if [[ -n "${NATIVE_CORE_SPEC_FILE:-}" ]]; then
  source_metadata_line="$(grep -E '^- source file SHA-256: .[0-9a-f]{64}.$' "$source_metadata" || true)"
  expected_source_sha="${source_metadata_line##* }"
  expected_source_sha="${expected_source_sha#\`}"
  expected_source_sha="${expected_source_sha%\`}"
  actual_source_sha="$(shasum -a 256 "$NATIVE_CORE_SPEC_FILE" | awk '{print $1}')"
  if [[ "$actual_source_sha" != "$expected_source_sha" ]]; then
    echo "$NATIVE_CORE_SPEC_FILE does not match the native Core source snapshot recorded in $source_metadata" >&2
    exit 1
  fi
fi
