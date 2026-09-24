#!/usr/bin/env python3
"""Fail-closed checks for explicitly approved, immutable release candidates."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import Request, urlopen

CANDIDATE = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.([1-9][0-9]*)")
SHA = re.compile(r"[0-9a-f]{40}")
REPOSITORY = "stellwerk-labs/platform-orchestrator-cli"


def validate_identity(tag, expected_sha, head_sha, tag_sha):
    if not CANDIDATE.fullmatch(tag):
        raise ValueError("candidate tag must be canonical vX.Y.Z-rc.N with N >= 1")
    if not SHA.fullmatch(expected_sha):
        raise ValueError("candidate SHA must be exactly 40 lowercase hexadecimal characters")
    if head_sha != expected_sha or tag_sha != expected_sha:
        raise ValueError("candidate tag, checkout and approved SHA must identify the same commit")


def validate_environment(environment):
    if environment.get("name") != "public-release-candidate":
        raise ValueError("the pre-provisioned public-release-candidate environment is required")
    rules = environment.get("protection_rules", [])
    if not any(rule.get("type") == "required_reviewers" and rule.get("reviewers") for rule in rules):
        raise ValueError("public-release-candidate must have at least one configured required reviewer")


def validate_absence_status(status):
    if status != 404:
        raise ValueError(f"candidate image absence is not proven (HTTP {status}); refuse publication")


def validate_release_page(tag, releases):
    if not isinstance(releases, list) or len(releases) > 100:
        raise ValueError("unexpected GitHub release list")
    for release in releases:
        if (not isinstance(release, dict)
                or not isinstance(release.get("tag_name"), str)
                or not isinstance(release.get("draft"), bool)):
            raise ValueError("unexpected GitHub release record")
        if release["tag_name"] == tag:
            raise ValueError("a draft or published GitHub release already reserves this tag")


def assert_release_unreserved(tag):
    token = os.environ.get("GH_TOKEN")
    if not token:
        raise ValueError("GH_TOKEN is required to detect existing draft releases as well as public releases")
    for page in range(1, 101):
        request = Request(
            f"https://api.github.com/repos/{REPOSITORY}/releases?per_page=100&page={page}",
            headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json"},
        )
        with urlopen(request, timeout=20) as response:
            releases = json.load(response)
        validate_release_page(tag, releases)
        if len(releases) < 100:
            return
    raise ValueError("release-list safety limit reached; absence is not proven")


def assert_image_absent(repository, tag):
    query = urlencode({"service": "ghcr.io", "scope": f"repository:{repository}:pull"})
    with urlopen(f"https://ghcr.io/token?{query}", timeout=20) as response:
        token = json.load(response)["token"]
    request = Request(
        f"https://ghcr.io/v2/{repository}/manifests/{tag}",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json",
        },
        method="HEAD",
    )
    try:
        with urlopen(request, timeout=20) as response:
            status = response.status
    except HTTPError as error:
        status = error.code
    validate_absence_status(status)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("sha")
    parser.add_argument("--environment-json", type=Path, required=True)
    parser.add_argument("--check-image-absent", metavar="OWNER/REPOSITORY")
    args = parser.parse_args()
    # Validate before using the tag as a Git revision or pathname.
    validate_identity(args.tag, args.sha, args.sha, args.sha)
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    tag = subprocess.check_output(["git", "rev-parse", "--verify", f"refs/tags/{args.tag}^{{commit}}"], text=True).strip()
    validate_identity(args.tag, args.sha, head, tag)
    validate_environment(json.loads(args.environment_json.read_text()))
    if not Path("docs/releases", args.tag + ".md").is_file():
        raise ValueError("reviewed candidate notes must exist in docs/releases/<candidate-tag>.md")
    if args.check_image_absent:
        if not re.fullmatch(r"stellwerk-labs/octl", args.check_image_absent):
            raise ValueError("only the reviewed public CLI image destination is supported")
        assert_release_unreserved(args.tag)
        assert_image_absent(args.check_image_absent, args.tag.removeprefix("v"))
    print(f"Verified release candidate {args.tag} at {args.sha}; no publication performed.")


if __name__ == "__main__":
    main()
