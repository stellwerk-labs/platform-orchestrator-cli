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
import time

import client_image
import client_oci
import qualify_release as release


SLSA_PREDICATE_TYPE = "https://slsa.dev/provenance/v1"
# Every failing attempt still lands at least one manifest, so six attempts cover the published graph.
COPY_ATTEMPTS = 6
REGISTRY_LAG = re.compile(r"Error response from registry: sha256:[0-9a-f]{64}: not found")
READBACK_ATTEMPTS = 12
# Only the remote tag is fetched on readback, so no layout error can reach this bare suffix.
TAG_MISSING = ": not found"


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


def run_oras(*arguments):
    """Run one ORAS attempt and return its exit code, stdout and last stderr line.

    ORAS stderr is only classified, never printed, so `CheckedToolFailure` stays free of child output.
    Each attempt is bounded at 120 s so 6 copies plus pauses fit the 30 min publisher jobs; a timeout
    raises at once.
    """
    result = subprocess.run(["oras", *arguments], stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=120, check=False)
    return result.returncode, result.stdout, (result.stderr.decode(errors="replace").splitlines() or [""])[-1]


def copy_index(source, reference, pause=10):
    """Copy the inspected index to its new tag, repeating only while the registry lags behind its push.

    `oras cp --no-tty` reads every manifest it has just pushed back from the destination to print its
    status, and GHCR can answer that read with 404 for a moment (Data Plane v4.5.0-rc.3 lost a candidate
    to it). The copy is content-addressed and only ever points the tag at the inspected index, so it is
    repeated for exactly that error; oras already retries 408, 429 and 5xx itself. Anything else, a
    timeout included, fails at once. A failed attempt may already have written the correct tag, so the
    destination's absence is checked once, before the first copy, never between attempts.
    """
    for attempt in range(1, COPY_ATTEMPTS + 1):
        returncode, _, last_line = run_oras("cp", "--no-tty", "--from-oci-layout", source, reference)
        if returncode == 0:
            return
        if returncode != 1 or not REGISTRY_LAG.fullmatch(last_line) or attempt == COPY_ATTEMPTS:
            raise release.CheckedToolFailure("oras", returncode)
        print(f"registry did not yet serve a manifest it had just accepted; copy attempt {attempt} of {COPY_ATTEMPTS}, retrying",
              file=sys.stderr)
        time.sleep(pause)


def read_back_index(reference, digest, pause=5):
    """Wait for the new tag to read back the qualified index; it was proven absent before the copy.

    The tag can lag behind the copy the same way, so the fetch repeats only while ORAS exits 1 with a
    last line ending in `: not found`. Any other failure, and any other digest, fails at once.
    """
    for attempt in range(1, READBACK_ATTEMPTS + 1):
        returncode, descriptor, last_line = run_oras("manifest", "fetch", "--descriptor", reference)
        if returncode == 0:
            if json.loads(descriptor)["digest"] != digest:
                raise ValueError("published index differs from qualified bytes")
            return
        if returncode != 1 or not last_line.endswith(TAG_MISSING) or attempt == READBACK_ATTEMPTS:
            raise release.CheckedToolFailure("oras", returncode)
        time.sleep(pause)


def main():
    phase = "entrypoint"
    try:
        phase = "source-receipt"
        dist = Path(sys.argv[1]).resolve()
        receipt = json.loads((dist / "release-content.json").read_text())
        producer = receipt["source"]
        os.environ.update(QUALIFY_SOURCE_SHA=producer["sourceSHA"], QUALIFY_TAG=producer["tag"],
                          QUALIFY_VERSION=producer["version"])
        if (os.environ.get("GITHUB_ACTIONS") != "true" or release.identity() != producer or
                producer["repository"] != "stellwerk-labs/platform-orchestrator-cli"):
            raise ValueError("only the existing public CLI workflow may publish qualified images")
        phase = "repository-visibility"
        if release.checked(["gh", "api", "repos/" + producer["repository"], "--jq", ".private"]).strip() != b"false":
            raise ValueError("public source repository must already exist")
        phase = "receipt-recheck"
        release.recheck(dist, dist / "release-content.json")
        tag = producer["version"]
        if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", tag):
            raise ValueError("version cannot be represented as an immutable OCI tag")
        spec = importlib.util.spec_from_file_location("candidate", release.ROOT / "scripts/candidate-release.py")
        candidate = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(candidate)
        reference = client_image.IMAGE + ":" + tag
        with tempfile.TemporaryDirectory(prefix="client-image-publish-") as temporary:
            temporary = Path(temporary)
            phase = "image-reinspection"
            report = client_image.reinspect(dist, receipt, temporary / "inspection")
            layout = dist / receipt["image"]["layout"]
            source_tag = client_oci.oci_reference(layout, report["index"])
            phase = "destination-absence"
            candidate.assert_image_absent("stellwerk-labs/octl", tag)
            # No build command exists in this publisher. Copy the inspected index,
            # including its original platform manifests and BuildKit attestations.
            phase = "image-copy"
            copy_index(str(layout) + ":" + source_tag, reference)
            phase = "published-index-check"
            read_back_index(reference, report["index"])
            immutable = client_image.IMAGE + "@" + report["index"]
            subjects = [immutable] + [client_image.IMAGE + "@" + value["manifest"] for value in report["payloads"].values()]
            for subject in subjects:
                phase = "image-signature"
                release.checked(["cosign", "sign", "--yes", subject])
                phase = "image-signature-verification"
                release.checked(["cosign", "verify", "--certificate-identity", "https://github.com/" + producer["workflowRef"],
                                 "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", subject])
            phase = "slsa-attestation"
            provenance = json.loads((dist / receipt["image"]["provenance"]).read_text())["predicate"]
            predicate_file = temporary / "producer.json"
            release.write_json(predicate_file, provenance)
            release.checked(["cosign", "attest", "--yes", "--type", SLSA_PREDICATE_TYPE,
                             "--predicate", str(predicate_file), immutable])
            phase = "slsa-attestation-verification"
            verify_attestation(immutable, SLSA_PREDICATE_TYPE, provenance, producer)
            for arch, payload in report["payloads"].items():
                phase = "spdx-attestation"
                predicate_file = dist / ("octl-image-linux-" + arch + ".spdx.json")
                subject = client_image.IMAGE + "@" + payload["manifest"]
                release.checked(["cosign", "attest", "--yes", "--type", "spdxjson", "--predicate", str(predicate_file), subject])
                phase = "spdx-attestation-verification"
                verify_attestation(subject, "spdxjson", json.loads(predicate_file.read_text()), producer)
            # Preserve the pre-existing stable-channel aliases only after the exact
            # immutable version, signatures and predicates have all been verified.
            if "-" not in tag:
                major, minor, _ = tag.split(".")
                for alias in (major, major + "." + minor, "latest"):
                    phase = "stable-channel-alias"
                    release.checked(["oras", "tag", immutable, alias])
                    actual = json.loads(release.checked(["oras", "manifest", "fetch", "--descriptor", client_image.IMAGE + ":" + alias]))
                    if actual["digest"] != report["index"]:
                        raise ValueError("stable channel alias differs from qualified index")
    except Exception as error:
        category = type(error).__name__
        if isinstance(error, release.CheckedToolFailure):
            category = "checked-tool"
            tool = error.tool if error.tool in release.TOOLS else "other"
            code = error.returncode if type(error.returncode) is int and -4294967295 <= error.returncode <= 4294967295 else "invalid"
            detail = f" tool={tool} exit={code}"
        else:
            category = category if category in {"ValueError", "OSError", "KeyError", "TypeError", "SubprocessError"} else "unclassified"
            detail = ""
        print(f"qualified image publication failed phase={phase} category={category}{detail}; inspect receipts before any retry",
              file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
