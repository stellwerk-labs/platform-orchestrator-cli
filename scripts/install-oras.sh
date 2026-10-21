#!/usr/bin/env bash
# Same ORAS version and official archive checksums as the runtime release tools.
set -euo pipefail
output="${1:?usage: install-oras.sh OUTPUT_DIRECTORY}"
mkdir -p "$output"
case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) arch=amd64; checksum=f27adb935022d94df8dc77719c322dda592c78a0d57a6f7dcdd8d900b248c454 ;;
  Linux/aarch64) arch=arm64; checksum=15702c6e3a4a56a8bd8ac5c17efdbcab56d9bada661ccbcf017f5b10c1d89399 ;;
  *) echo "ORAS release installer requires native Linux amd64/arm64" >&2; exit 1 ;;
esac
archive="$(mktemp)"
trap 'rm -- "$archive"' EXIT
curl --fail --silent --show-error --location --max-time 120 \
  "https://github.com/oras-project/oras/releases/download/v1.3.4/oras_1.3.4_linux_$arch.tar.gz" -o "$archive"
test "$(sha256sum "$archive" | awk '{print $1}')" = "$checksum"
tar -xzf "$archive" -C "$output" oras
"$output/oras" version | grep -E '^Version:[[:space:]]+1\.3\.4$' >/dev/null
