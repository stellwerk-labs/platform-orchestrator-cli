#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/release-tools.sh"
directory="${1:?usage: scan-release-content.sh DIRECTORY [FORBIDDEN_LITERAL ...]}"
shift
test ! -L "$directory" || { echo "symlinked scan root" >&2; exit 1; }
directory="$(cd "$directory" && pwd)"
test "$(gitleaks version)" = "$gitleaks_version" || { echo "incorrect gitleaks version" >&2; exit 1; }
workspace="$(mktemp -d "${TMPDIR:-/tmp}/client-scan.XXXXXX")"
trap 'rm -rf -- "$workspace"' EXIT
mkdir "$workspace/empty"
# Compressed archives are shipping content, not opaque binary blobs. Extract
# only bounded regular members, never links/devices/traversal or unbounded bombs.
if ! python3 "$root/scripts/release_content.py" unpack "$directory" "$workspace/archives" "$workspace/metadata" >/dev/null 2>&1; then
  echo "archive inspection failed (unsafe content or scan limit)" >&2
  exit 1
fi
scan_tree() {
  local tree="$1"
  shift
  # Reject source-controlled ignore files before the scanner can load them.
  python3 "$root/scripts/release_content.py" scan "$tree" "$@"
  # Avoid logging finding values, even on scanner failure. No findings report is
  # uploaded. Shipping content cannot provide inline or fingerprint exemptions.
  if ! python3 "$root/scripts/release_content.py" secret-scan "$tree" "$workspace/empty"; then
    echo "secret scan failed (finding or scanner error); inspect locally with redaction" >&2
    exit 1
  fi
  # Gitleaks intentionally skips binary files. Inspect their printable strings
  # and shipping paths without writing credentials into logs or reports.
  if ! python3 "$root/scripts/release_content.py" binary-scan "$tree" "$workspace/empty"; then
    echo "binary/path secret scan failed (finding or scanner error)" >&2
    exit 1
  fi
}
scan_tree "$directory" "$@"
scan_tree "$workspace/archives" "$@"
scan_tree "$workspace/metadata" "$@"
