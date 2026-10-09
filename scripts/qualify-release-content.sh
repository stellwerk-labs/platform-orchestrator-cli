#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# GoReleaser's OSS SBOM pipe runs once after archives, before checksums/signing.
exec python3 "$root/scripts/qualify_release.py" qualify "$PWD"
