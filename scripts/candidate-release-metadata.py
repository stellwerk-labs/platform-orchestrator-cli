#!/usr/bin/env python3
"""Create and publish one precisely bound candidate release."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import HTTPRedirectHandler, Request, build_opener


REPOSITORY = "stellwerk-labs/platform-orchestrator-cli"
API_ORIGIN = "https://api.github.com"
UPLOAD_ORIGIN = "https://uploads.github.com"
PLATFORMS = {
    "darwin_amd64": "tar.gz",
    "darwin_arm64": "tar.gz",
    "linux_amd64": "tar.gz",
    "linux_arm64": "tar.gz",
    "windows_amd64": "zip",
}
IMAGE_ARCHES = ("amd64", "arm64")
EXTRA_FILES = ("octl-image-content.json", "octl-image.provenance.json", "release-content.json")
SIDECARS = (".sig", ".pem")
SHA = re.compile(r"[0-9a-f]{40}")
CANDIDATE = re.compile(r"v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)-rc\.[1-9][0-9]*")


class RejectRedirects(HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, new_url):
        return None


class GitHub:
    """Small REST transport. Alternate origins are accepted only by direct test construction."""
    def __init__(self, token, api_origin=API_ORIGIN, upload_origin=UPLOAD_ORIGIN):
        self.token = token
        self.api_origin = api_origin.rstrip("/")
        self.upload_origin = upload_origin.rstrip("/")
        self.opener = build_opener(RejectRedirects())

    def request(self, method, path, body=None, upload=False, content_type="application/json"):
        origin = self.upload_origin if upload else self.api_origin
        data = body if isinstance(body, bytes) else json.dumps(body).encode() if body is not None else None
        headers = {"Authorization": f"Bearer {self.token}",
                   "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
        if data is not None:
            headers["Content-Type"] = content_type
        request = Request(origin + path, data=data, headers=headers, method=method)
        try:
            with self.opener.open(request, timeout=30) as response:
                payload = response.read()
        except HTTPError:
            raise ValueError("GitHub release request failed; inspect captured release state before acting") from None
        if not payload:
            return None
        try:
            return json.loads(payload)
        except (UnicodeDecodeError, json.JSONDecodeError):
            raise ValueError("GitHub returned an invalid release response") from None

    def source_ref(self, tag):
        ref = self.request("GET", f"/repos/{REPOSITORY}/git/ref/tags/{quote(tag, safe='')}")
        obj = ref.get("object") if isinstance(ref, dict) else None
        if (not isinstance(ref, dict) or ref.get("ref") != f"refs/tags/{tag}"
                or not isinstance(obj, dict) or not SHA.fullmatch(str(obj.get("sha", "")))):
            raise ValueError("remote candidate tag reference is invalid")
        if obj.get("type") == "commit":
            return obj["sha"], None
        if obj.get("type") != "tag":
            raise ValueError("remote candidate tag must resolve to an annotated tag or commit")
        tag_object = obj["sha"]
        annotated = self.request("GET", f"/repos/{REPOSITORY}/git/tags/{tag_object}")
        target = annotated.get("object") if isinstance(annotated, dict) else None
        if (not isinstance(annotated, dict) or annotated.get("sha") != tag_object or annotated.get("tag") != tag
                or not isinstance(target, dict) or target.get("type") != "commit"
                or not SHA.fullmatch(str(target.get("sha", "")))):
            raise ValueError("remote annotated candidate tag does not target a commit")
        return target["sha"], tag_object

    def assert_release_tag_unreserved(self, tag):
        for page in range(1, 101):
            releases = self.request("GET", f"/repos/{REPOSITORY}/releases?per_page=100&page={page}")
            if not isinstance(releases, list) or len(releases) > 100:
                raise ValueError("GitHub release inventory is invalid; tag availability is unknown")
            for release in releases:
                if (not isinstance(release, dict) or not isinstance(release.get("tag_name"), str)
                        or type(release.get("draft")) is not bool):
                    raise ValueError("GitHub release inventory is malformed")
                if release["tag_name"] == tag:
                    raise ValueError("candidate tag already has a draft or published release")
            if len(releases) < 100:
                return
        raise ValueError("GitHub release inventory limit reached; tag availability is unknown")


def expected_asset_names(version):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+-rc\.[1-9][0-9]*", version):
        raise ValueError("candidate version must be canonical X.Y.Z-rc.N")
    names = ["checksums.txt", *EXTRA_FILES]
    for target, extension in PLATFORMS.items():
        archive = f"platform-orchestrator-cli_{version}_{target}.{extension}"
        names.extend((archive, archive + ".provenance.json", archive + ".spdx.json"))
    for arch in IMAGE_ARCHES:
        names.append(f"octl-image-linux-{arch}.spdx.json")
    signed = ["checksums.txt"]
    for target, extension in PLATFORMS.items():
        archive = f"platform-orchestrator-cli_{version}_{target}.{extension}"
        signed.extend((archive, archive + ".spdx.json"))
    signed.extend(f"octl-image-linux-{arch}.spdx.json" for arch in IMAGE_ARCHES)
    for name in signed:
        names.extend(name + suffix for suffix in SIDECARS)
    if len(names) != 47 or len(set(names)) != 47:
        raise ValueError("candidate asset policy is internally inconsistent")
    return sorted(names)


def asset_inventory(dist, version):
    dist = Path(dist)
    inventory = []
    for name in expected_asset_names(version):
        path = dist / name
        if path.is_symlink() or not path.is_file():
            raise ValueError("candidate local asset set is incomplete or unsafe")
        data = path.read_bytes()
        inventory.append({"name": name, "size": len(data),
                          "digest": "sha256:" + hashlib.sha256(data).hexdigest()})
    return inventory


def local_source(repository, tag, expected_sha):
    if not CANDIDATE.fullmatch(tag) or not SHA.fullmatch(expected_sha):
        raise ValueError("approved source SHA is invalid")
    def git(*args):
        return subprocess.check_output(["git", "-C", str(repository), *args], text=True).strip()
    head = git("rev-parse", "HEAD")
    object_sha = git("rev-parse", "--verify", f"refs/tags/{tag}")
    object_type = git("cat-file", "-t", object_sha)
    commit_sha = git("rev-parse", "--verify", f"refs/tags/{tag}^{{commit}}")
    if head != expected_sha or commit_sha != expected_sha or object_type != "tag":
        raise ValueError("local HEAD and annotated candidate tag must equal the approved source")
    return {"sourceSHA": expected_sha, "tagObjectSHA": object_sha}


def verify_source(repository, tag, expected_sha, binding, github):
    local = local_source(repository, tag, expected_sha)
    if local != binding:
        raise ValueError("local candidate source or tag object differs from the captured binding")
    remote_commit, remote_tag_object = github.source_ref(tag)
    if remote_commit != expected_sha or remote_tag_object != binding["tagObjectSHA"]:
        raise ValueError("public candidate tag moved or does not match the approved annotated tag")


def validate_release_record(tag, release, notes, expected_id=None, state="draft", expected_assets=None):
    if not isinstance(release, dict):
        raise ValueError("unexpected GitHub release record")
    if (release.get("tag_name") != tag or type(release.get("id")) is not int or release["id"] <= 0
            or expected_id is not None and release["id"] != expected_id):
        raise ValueError("candidate release identity differs from the captured reservation")
    if release.get("name") != tag:
        raise ValueError("candidate release title differs from the exact tag")
    if type(release.get("prerelease")) is not bool or release["prerelease"] is not True:
        raise ValueError("candidate release must remain a prerelease")
    expected_draft = state == "draft"
    if state not in ("draft", "published") or release.get("draft") is not expected_draft:
        raise ValueError("candidate release is not in the expected publication phase")
    if release.get("body") != notes:
        raise ValueError("candidate release notes differ from the reviewed source bytes")
    assets = release.get("assets")
    if not isinstance(assets, list):
        raise ValueError("candidate release asset list is invalid")
    if expected_assets is not None:
        if not isinstance(expected_assets, list) or len(expected_assets) != 47:
            raise ValueError("expected candidate asset inventory is missing")
        expected = {}
        for item in expected_assets:
            if (not isinstance(item, dict) or not isinstance(item.get("name"), str)
                    or type(item.get("size")) is not int or item["size"] < 0
                    or not isinstance(item.get("digest"), str)
                    or not re.fullmatch(r"sha256:[0-9a-f]{64}", item["digest"])
                    or item["name"] in expected):
                raise ValueError("expected candidate asset inventory is malformed")
            expected[item["name"]] = {"name": item["name"], "size": item["size"], "digest": item["digest"]}
        if set(expected) != set(expected_asset_names(tag.removeprefix("v"))):
            raise ValueError("expected candidate asset inventory has the wrong names")
        actual = {}
        ids = set()
        for asset in assets:
            if (not isinstance(asset, dict) or not isinstance(asset.get("name"), str)
                    or type(asset.get("size")) is not int or asset["size"] < 0
                    or not isinstance(asset.get("digest"), str)
                    or not re.fullmatch(r"sha256:[0-9a-f]{64}", asset["digest"])
                    or type(asset.get("id")) is not int or asset["id"] <= 0 or asset["id"] in ids
                    or asset.get("state") != "uploaded" or asset["name"] in actual):
                raise ValueError("candidate release has malformed, duplicate or incomplete assets")
            ids.add(asset["id"])
            actual[asset["name"]] = {"name": asset["name"], "size": asset["size"], "digest": asset["digest"]}
        if actual != expected:
            raise ValueError("candidate release assets differ from exact locally signed bytes")
    return release["id"]


def write_new_json(path, value):
    with Path(path).open("x") as output:
        json.dump(value, output, sort_keys=True, indent=2)
        output.write("\n")


def reservation_record(path, tag, expected_sha, notes):
    captured = json.loads(Path(path).read_text())
    if (captured.get("tag") != tag or captured.get("sourceSHA") != expected_sha
            or type(captured.get("releaseID")) is not int or captured["releaseID"] <= 0
            or not SHA.fullmatch(str(captured.get("tagObjectSHA", "")))
            or captured.get("notesSHA256") != hashlib.sha256(notes.encode()).hexdigest()):
        raise ValueError("candidate reservation does not bind this release inputs")
    return captured


def release_path(release_id):
    if type(release_id) is not int or release_id <= 0:
        raise ValueError("captured candidate release ID is invalid")
    return f"/repos/{REPOSITORY}/releases/{release_id}"


def validate_empty_draft(tag, release, notes, release_id):
    validate_release_record(tag, release, notes, expected_id=release_id, state="draft")
    if release["assets"]:
        raise ValueError("candidate release already has assets; refuse to overwrite or retry")


def create_reservation(args, github):
    binding = local_source(args.repository, args.tag, args.source_sha)
    github.assert_release_tag_unreserved(args.tag)
    verify_source(args.repository, args.tag, args.source_sha, binding, github)
    release = github.request("POST", f"/repos/{REPOSITORY}/releases", {
        "tag_name": args.tag,
        "name": args.tag, "body": args.notes.read_bytes().decode("utf-8"),
        "prerelease": True, "draft": True, "make_latest": "false"})
    release_id = release.get("id") if isinstance(release, dict) else None
    if type(release_id) is not int or release_id <= 0:
        raise ValueError("GitHub draft creation returned no numeric release ID")
    receipt = {"releaseID": release_id, "tag": args.tag, **binding,
               "notesSHA256": hashlib.sha256(args.notes.read_bytes()).hexdigest()}
    write_new_json(args.output, receipt)
    validate_release_record(args.tag, release, args.notes.read_bytes().decode("utf-8"),
                            expected_id=release_id, state="draft")
    if release.get("assets") != []:
        raise ValueError("new candidate draft unexpectedly contains assets; preserve it for inspection")


def upload_assets(args, github):
    notes = args.notes.read_bytes().decode("utf-8")
    captured = reservation_record(args.receipt, args.tag, args.source_sha, notes)
    binding = {"sourceSHA": captured["sourceSHA"], "tagObjectSHA": captured["tagObjectSHA"]}
    verify_source(args.repository, args.tag, args.source_sha, binding, github)
    path = release_path(captured["releaseID"])
    release = github.request("GET", path)
    validate_empty_draft(args.tag, release, notes, captured["releaseID"])
    expected = json.loads(args.local_assets.read_text())
    if expected != asset_inventory(args.dist, args.tag.removeprefix("v")):
        raise ValueError("local candidate assets changed after the pre-reservation snapshot")
    upload_path = f"/repos/{REPOSITORY}/releases/{captured['releaseID']}/assets"
    for item in expected:
        data = (args.dist / item["name"]).read_bytes()
        if len(data) != item["size"] or "sha256:" + hashlib.sha256(data).hexdigest() != item["digest"]:
            raise ValueError("local candidate asset changed after inventory capture")
        github.request("POST", upload_path + "?name=" + quote(item["name"], safe=""),
                       data, upload=True, content_type="application/octet-stream")
    release = github.request("GET", path)
    validate_release_record(args.tag, release, notes, captured["releaseID"], "draft", expected)
    write_new_json(args.assets, expected)


def validate_existing(args, github, finalize=False):
    notes = args.notes.read_bytes().decode("utf-8")
    captured = reservation_record(args.receipt, args.tag, args.source_sha, notes)
    expected = json.loads(args.assets.read_text())
    binding = {"sourceSHA": captured["sourceSHA"], "tagObjectSHA": captured["tagObjectSHA"]}
    verify_source(args.repository, args.tag, args.source_sha, binding, github)
    path = release_path(captured["releaseID"])
    release = github.request("GET", path)
    validate_release_record(args.tag, release, notes, captured["releaseID"], "draft", expected)
    if asset_inventory(args.dist, args.tag.removeprefix("v")) != expected:
        raise ValueError("local candidate assets differ from the uploaded release inventory")
    if finalize:
        verify_source(args.repository, args.tag, args.source_sha, binding, github)
        github.request("PATCH", path, {"draft": False, "prerelease": True, "make_latest": "false"})
        published = github.request("GET", path)
        validate_release_record(args.tag, published, notes, captured["releaseID"], "published", expected)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    def source_args(command):
        command.add_argument("tag")
        command.add_argument("source_sha")
        command.add_argument("notes", type=Path)
        command.add_argument("receipt", type=Path)
        command.add_argument("--repository", type=Path, default=Path.cwd())
    reserve = commands.add_parser("reserve")
    reserve.add_argument("tag")
    reserve.add_argument("source_sha")
    reserve.add_argument("notes", type=Path)
    reserve.add_argument("output", type=Path)
    reserve.add_argument("--repository", type=Path, default=Path.cwd())
    snapshot = commands.add_parser("snapshot-assets")
    snapshot.add_argument("tag")
    snapshot.add_argument("dist", type=Path)
    snapshot.add_argument("output", type=Path)
    upload = commands.add_parser("upload")
    source_args(upload)
    upload.add_argument("dist", type=Path)
    upload.add_argument("local_assets", type=Path)
    upload.add_argument("assets", type=Path)
    validate = commands.add_parser("validate")
    source_args(validate)
    validate.add_argument("dist", type=Path)
    validate.add_argument("assets", type=Path)
    publish = commands.add_parser("publish")
    source_args(publish)
    publish.add_argument("dist", type=Path)
    publish.add_argument("assets", type=Path)
    args = parser.parse_args()
    if args.command == "snapshot-assets":
        write_new_json(args.output, asset_inventory(args.dist, args.tag.removeprefix("v")))
        print("Captured exact local candidate asset hashes and sizes.")
        return
    token = os.environ.get("GH_TOKEN")
    if not token:
        raise ValueError("GH_TOKEN is required for candidate release operations")
    github = GitHub(token)
    if args.command == "reserve":
        create_reservation(args, github)
    elif args.command == "upload":
        upload_assets(args, github)
    elif args.command == "validate":
        validate_existing(args, github)
    else:
        validate_existing(args, github, finalize=True)
    print(f"Completed candidate release phase {args.command} for {args.tag}.")


if __name__ == "__main__":
    main()
