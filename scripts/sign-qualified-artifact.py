#!/usr/bin/env python3
"""Recheck qualified bytes before invoking the existing release signer."""
import os
from pathlib import Path
import re
import subprocess
import sys
import shutil
import tempfile

import qualify_release as release


def main():
    mode, source, tag, version, artifact, signature, *remaining = sys.argv[1:]
    os.environ.update(QUALIFY_SOURCE_SHA=source, QUALIFY_TAG=tag, QUALIFY_VERSION=version)
    producer = release.identity()
    expected = "stellwerk-labs/" + ("platform-orchestrator-cli" if mode == "cosign" else "terraform-provider-platform-orchestrator")
    if os.environ.get("GITHUB_ACTIONS") != "true" or producer["repository"] != expected:
        raise ValueError("release signing requires the existing public GitHub workflow authority")
    artifact, signature = Path(artifact).resolve(), Path(signature).resolve()
    shipping_evidence = [signature]
    if signature.parent != artifact.parent or signature.exists():
        raise ValueError("signature must be a new adjacent artifact")
    release.recheck(artifact.parent, artifact)
    if mode == "cosign":
        certificate, = remaining
        certificate = Path(certificate).resolve()
        shipping_evidence.append(certificate)
        if certificate.parent != artifact.parent or certificate.exists():
            raise ValueError("certificate must be a new adjacent artifact")
        subprocess.run(["cosign", "sign-blob", "--oidc-provider=github-actions",
                        "--output-certificate=" + str(certificate), "--output-signature=" + str(signature),
                        str(artifact), "--yes"], check=True)
        subprocess.run(["cosign", "verify-blob", "--certificate", str(certificate), "--signature", str(signature),
                        "--certificate-identity", "https://github.com/" + producer["workflowRef"],
                        "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", str(artifact)], check=True)
    elif mode == "gpg":
        fingerprint, = remaining
        if not re.fullmatch(r"[A-Fa-f0-9]{40,64}", fingerprint):
            raise ValueError("exact Registry signing fingerprint required")
        # Keep the Registry's detached GPG checksum signature contract. The
        # passphrase goes to GPG's stdin, never a process argument or log.
        subprocess.run(["gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase-fd", "0",
                        "--local-user", fingerprint, "--output", str(signature), "--detach-sign", str(artifact)],
                       input=os.environ["GPG_PASSPHRASE"].encode() + b"\n", check=True)
        status = release.checked(["gpg", "--status-fd", "1", "--verify", str(signature), str(artifact)]).decode()
        if not any(line.startswith("[GNUPG:] VALIDSIG " + fingerprint.upper() + " ") for line in status.splitlines()):
            raise ValueError("Registry signature does not bind the expected key")
    else:
        raise ValueError("unsupported release signer")
    with tempfile.TemporaryDirectory(prefix="client-signature-scan-") as temporary:
        for path in shipping_evidence:
            if path.is_symlink() or not path.is_file() or not path.stat().st_size:
                raise ValueError("signer did not produce exact regular evidence")
            shutil.copyfile(path, Path(temporary) / path.name)
        release.checked([str(release.ROOT / "scripts/scan-release-content.sh"), temporary, "jayonthenet"])


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError, subprocess.SubprocessError):
        print("qualified release signing failed", file=sys.stderr)
        sys.exit(1)
