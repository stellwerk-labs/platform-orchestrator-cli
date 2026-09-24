#!/usr/bin/env python3
"""Verify stable publication prerequisites without creating or modifying releases."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.request import Request, urlopen

spec = importlib.util.spec_from_file_location("candidate", Path(__file__).with_name("candidate-release.py"))
candidate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(candidate)


def validate_stable_identity(tag, expected_sha, head_sha, tag_sha):
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("stable tag must be canonical vX.Y.Z")
    if not re.fullmatch(r"[a-f0-9]{40}", expected_sha):
        raise ValueError("expected source must be a complete commit SHA")
    if expected_sha != head_sha or expected_sha != tag_sha:
        raise ValueError("stable tag, approved source and checkout must match")


def validate_channel_access(repository, metadata):
    if (metadata.get("full_name") != repository
            or metadata.get("archived") is not False
            or metadata.get("permissions", {}).get("push") is not True):
        raise ValueError(f"GH_PAT must have write access to the active {repository} channel")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("sha")
    args = parser.parse_args()
    validate_stable_identity(args.tag, args.sha, args.sha, args.sha)
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    tag = subprocess.check_output(["git", "rev-parse", "--verify", f"refs/tags/{args.tag}^{{commit}}"], text=True).strip()
    validate_stable_identity(args.tag, args.sha, head, tag)
    if not Path("docs/releases", args.tag + ".md").is_file():
        raise ValueError("reviewed stable release notes are required")
    candidate.assert_release_unreserved(args.tag)
    candidate.assert_image_absent("stellwerk-labs/octl", args.tag.removeprefix("v"))
    token = os.environ.get("GH_PAT")
    if not token:
        raise ValueError("GH_PAT is required for stable distribution channels")
    for repository in ("stellwerk-labs/homebrew-tap", "stellwerk-labs/scoop-bucket"):
        request = Request(f"https://api.github.com/repos/{repository}", headers={
            "Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json",
        })
        with urlopen(request, timeout=20) as response:
            validate_channel_access(repository, json.load(response))
    print(f"Stable publication prerequisites verified for {args.tag}; no writes performed.")


if __name__ == "__main__":
    main()
