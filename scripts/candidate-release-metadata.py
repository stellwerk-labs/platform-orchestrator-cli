#!/usr/bin/env python3
"""Verify the published candidate contract without modifying a release."""
import json
from pathlib import Path
import sys


def validate_metadata(expected_tag, release):
    if release.get("tagName") != expected_tag or release.get("isPrerelease") is not True:
        raise ValueError("release must identify the exact candidate tag and be a prerelease")
    if release.get("isDraft") is not False:
        raise ValueError("candidate release must not remain a draft")
    names = {asset.get("name") for asset in release.get("assets", [])}
    if len(names) < 18 or not {"checksums.txt", "checksums.txt.sig", "checksums.txt.pem"}.issubset(names):
        raise ValueError("candidate is missing the expected signed release assets")


if __name__ == "__main__":
    validate_metadata(sys.argv[1], json.loads(Path(sys.argv[2]).read_text()))
    print("Candidate release metadata and signed-asset presence verified.")
