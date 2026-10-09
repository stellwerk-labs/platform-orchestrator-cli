#!/usr/bin/env python3
"""Publish only the locally qualified OCI index, then verify keyless evidence."""
import base64
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

import client_image
import client_oci
import qualify_release as release


def verify_attestation(reference, kind, predicate, producer):
    output = release.checked([
        "cosign", "verify-attestation", "--type", kind,
        "--certificate-identity", "https://github.com/" + producer["workflowRef"],
        "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", reference,
    ])
    statements = []
    # Cosign emits JSON objects as a stream (not necessarily one JSON array).
    decoder, remaining = json.JSONDecoder(), output.decode().strip()
    while remaining:
        value, end = decoder.raw_decode(remaining)
        statements.extend(value if isinstance(value, list) else [value])
        remaining = remaining[end:].lstrip()
    expected_digest = reference.rsplit("@sha256:", 1)[1]
    for envelope in statements:
        statement = json.loads(base64.b64decode(envelope["payload"], validate=True))
        if (statement.get("predicate") == predicate and
                any(subject.get("digest", {}).get("sha256") == expected_digest
                    for subject in statement.get("subject", []))):
            return
    raise ValueError("verified attestation does not bind the qualified predicate and subject")


def authenticated_absence(reference):
    result = subprocess.run(["oras", "manifest", "fetch", "--descriptor", reference],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode == 0:
        raise ValueError("immutable image version already exists; never overwrite or retry blindly")
    if not re.search(rb"(?:\b404\b|MANIFEST_UNKNOWN|NAME_UNKNOWN)", result.stderr):
        raise ValueError("authenticated image absence is not proven")


def main():
    dist = Path(sys.argv[1]).resolve()
    receipt = json.loads((dist / "release-content.json").read_text())
    producer = receipt["source"]
    os.environ.update(QUALIFY_SOURCE_SHA=producer["sourceSHA"], QUALIFY_TAG=producer["tag"],
                      QUALIFY_VERSION=producer["version"])
    if (os.environ.get("GITHUB_ACTIONS") != "true" or release.identity() != producer or
            producer["repository"] != "stellwerk-labs/platform-orchestrator-cli"):
        raise ValueError("only the existing public CLI workflow may publish qualified images")
    if release.checked(["gh", "api", "repos/" + producer["repository"], "--jq", ".private"]).strip() != b"false":
        raise ValueError("public source repository must already exist")
    release.recheck(dist, dist / "release-content.json")
    tag = producer["version"]
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", tag):
        raise ValueError("version cannot be represented as an immutable OCI tag")
    spec = importlib.util.spec_from_file_location("candidate", release.ROOT / "scripts/candidate-release.py")
    candidate = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(candidate)
    candidate.assert_image_absent("stellwerk-labs/octl", tag)
    reference = client_image.IMAGE + ":" + tag
    authenticated_absence(reference)
    with tempfile.TemporaryDirectory(prefix="client-image-publish-") as temporary:
        temporary = Path(temporary)
        report = client_image.reinspect(dist, receipt, temporary / "inspection")
        layout = dist / receipt["image"]["layout"]
        source_tag = client_oci.oci_reference(layout, report["index"])
        # No build command exists in this publisher. Copy the inspected index,
        # including its original platform manifests and BuildKit attestations.
        release.checked(["oras", "cp", "--no-tty", "--from-oci-layout",
                         str(layout) + ":" + source_tag, reference])
        descriptor = json.loads(release.checked(["oras", "manifest", "fetch", "--descriptor", reference]))
        if descriptor["digest"] != report["index"]:
            raise ValueError("published index differs from qualified bytes")
        immutable = client_image.IMAGE + "@" + report["index"]
        subjects = [immutable] + [client_image.IMAGE + "@" + value["manifest"] for value in report["payloads"].values()]
        for subject in subjects:
            release.checked(["cosign", "sign", "--yes", subject])
            release.checked(["cosign", "verify", "--certificate-identity", "https://github.com/" + producer["workflowRef"],
                             "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", subject])
        provenance = json.loads((dist / receipt["image"]["provenance"]).read_text())["predicate"]
        predicate_file = temporary / "producer.json"
        release.write_json(predicate_file, provenance)
        release.checked(["cosign", "attest", "--yes", "--type", "slsaprovenance1", "--predicate", str(predicate_file), immutable])
        verify_attestation(immutable, "slsaprovenance1", provenance, producer)
        for arch, payload in report["payloads"].items():
            predicate_file = dist / ("octl-image-linux-" + arch + ".spdx.json")
            subject = client_image.IMAGE + "@" + payload["manifest"]
            release.checked(["cosign", "attest", "--yes", "--type", "spdxjson", "--predicate", str(predicate_file), subject])
            verify_attestation(subject, "spdxjson", json.loads(predicate_file.read_text()), producer)
        # Preserve the pre-existing stable-channel aliases only after the exact
        # immutable version, signatures and predicates have all been verified.
        if "-" not in tag:
            major, minor, _ = tag.split(".")
            for alias in (major, major + "." + minor, "latest"):
                release.checked(["oras", "tag", immutable, alias])
                actual = json.loads(release.checked(["oras", "manifest", "fetch", "--descriptor", client_image.IMAGE + ":" + alias]))
                if actual["digest"] != report["index"]:
                    raise ValueError("stable channel alias differs from qualified index")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError, subprocess.SubprocessError):
        print("qualified image publication failed; inspect receipts before any retry", file=sys.stderr)
        sys.exit(1)
