#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GOTOOLCHAIN=go1.26.2 GOWORK=off GOENV=off GOSUMDB=sum.golang.org CGO_ENABLED=0
unset GOFLAGS GOEXPERIMENT GOOS GOARCH GOPRIVATE GONOSUMDB GONOPROXY
if [ "${1:-}" = --install-tool ]; then
  tools="${2:?usage: collect-licenses.sh --install-tool TOOL_DIRECTORY}"
  mkdir -p "$tools"
else
  tools="$(mktemp -d)"
  trap 'rm -f "$tools/go-licenses"; rmdir "$tools"' EXIT
fi
module=github.com/google/go-licenses/v2
version=v2.0.1
sum=h1:ti+9bi5o7DKbeeg5eBb/uZTgsaPNoJaLCh93cRcXsW8=
test "$(go mod download -json "$module@$version" | python3 -c 'import json,sys; print(json.load(sys.stdin)["Sum"])')" = "$sum"
GOBIN="$tools" go install "$module@$version"
go version -m "$tools/go-licenses" | grep -F "$module" | grep -F "$version" >/dev/null
if [ "${1:-}" = --install-tool ]; then exit 0; fi
export PATH="$tools:$PATH"
cd "$root"
go run ./scripts/licensecollector "$@"
