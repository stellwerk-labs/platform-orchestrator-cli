#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root/scripts/release-tools.sh"
output="${1:?usage: install-release-tools.sh OUTPUT_DIRECTORY}"
mkdir -p "$output"
output="$(cd "$output" && pwd)"

# The Go checksum database authenticates the module graph; additionally bind
# this tool's source archive to the reviewed module checksum.
export GOTOOLCHAIN="$release_go_version" GOWORK=off GOSUMDB=sum.golang.org GOENV=off
unset GOFLAGS GOPRIVATE GONOSUMDB GONOPROXY GOOS GOARCH GOAMD64 GOARM GOARM64 GOEXPERIMENT
metadata="$(go mod download -json "$go_licenses_module@$go_licenses_version")"
jq -e --arg sum "$go_licenses_sum" '.Sum == $sum' <<<"$metadata" >/dev/null
GOBIN="$output" go install "$go_licenses_module@$go_licenses_version"
go version -m "$output/go-licenses" | grep -F "$go_licenses_module" | grep -F "$go_licenses_version" >/dev/null

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) platform=linux_x64; checksum=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb ;;
  Linux/aarch64) platform=linux_arm64; checksum=e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080 ;;
  Darwin/arm64) platform=darwin_arm64; checksum=b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5 ;;
  Darwin/x86_64) platform=darwin_x64; checksum=dfe101a4db2255fc85120ac7f3d25e4342c3c20cf749f2c20a18081af1952709 ;;
  *) echo "unsupported release-tool platform" >&2; exit 1 ;;
esac
archive="$output/gitleaks.tar.gz"
curl --fail --silent --show-error --location --max-time 120 \
  "https://github.com/gitleaks/gitleaks/releases/download/v$gitleaks_version/gitleaks_${gitleaks_version}_$platform.tar.gz" -o "$archive"
test "$(shasum -a 256 "$archive" | awk '{print $1}')" = "$checksum"
tar -xzf "$archive" -C "$output" gitleaks
test "$("$output/gitleaks" version)" = "$gitleaks_version"
rm -- "$archive"
