#!/usr/bin/env python3
"""Exercise the exact pinned Cosign producer with local-key, offline fixtures."""
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


COSIGN_RUNTIME = "ghcr.io/sigstore/cosign/cosign@sha256:9e5c2f2edc34351160407ca3416c61855bdf9403c3c5936e0f0be7fc261611b8"


class CosignOfflineAttestationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cosign = os.environ.get("COSIGN_BINARY") or shutil.which("cosign")
        if not cls.cosign or not Path(cls.cosign).is_file():
            raise RuntimeError("Cosign v2.6.1 is required; install it with the pinned workflow action")
        if shutil.which("docker") is None:
            raise RuntimeError("Docker is required to enforce network isolation for Cosign fixtures")

    def run_cosign(self, directory, *args, expected=0):
        result = subprocess.run([
            "docker", "run", "--rm", "--pull=never", "--network", "none", "--read-only",
            "--user", f"{os.getuid()}:{os.getgid()}",
            "--tmpfs", f"/tmp:rw,noexec,nosuid,uid={os.getuid()},gid={os.getgid()},mode=0700",
            "--env", "HOME=/tmp", "--env", "COSIGN_PASSWORD=",
            "--mount", f"type=bind,source={directory},target=/fixture",
            "--mount", f"type=bind,source={self.cosign},target=/cosign,readonly",
            "--entrypoint", "/cosign", COSIGN_RUNTIME, *args,
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False, timeout=60)
        self.assertEqual(result.returncode, expected,
                         f"pinned offline Cosign command returned {result.returncode}")
        return result.stdout

    def test_actual_slsa_and_spdx_producer_round_trip(self):
        with tempfile.TemporaryDirectory(prefix="cosign-offline-") as temporary:
            root = Path(temporary)
            version = self.run_cosign(root, "version").decode(errors="replace")
            self.assertIn("v2.6.1", version)
            predicate = {
                "buildDefinition": {"buildType": "https://example.invalid/local-fixture"},
                "runDetails": {"builder": {"id": "local-fixture"},
                               "metadata": {"invocationId": "fixture-run-1"}},
            }
            spdx = {"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0",
                    "SPDXID": "SPDXRef-DOCUMENT", "name": "local fixture",
                    "documentNamespace": "https://example.invalid/local-fixture"}
            (root / "slsa.json").write_text(json.dumps(predicate, sort_keys=True))
            (root / "spdx.json").write_text(json.dumps(spdx, sort_keys=True))
            (root / "subject").write_bytes(b"synthetic offline subject, not a published artifact\n")
            (root / "wrong-subject").write_bytes(b"different synthetic subject\n")

            self.run_cosign(root, "generate-key-pair", "--output-key-prefix", "/fixture/local-key")
            key_stat = (root / "local-key.key").stat()
            self.assertEqual((key_stat.st_uid, key_stat.st_gid), (os.getuid(), os.getgid()))
            slsa_output = self.run_cosign(
                root, "attest-blob", "--key", "/fixture/local-key.key", "--bundle", "/fixture/slsa.bundle",
                "--type", "https://slsa.dev/provenance/v1", "--predicate", "/fixture/slsa.json",
                "--tlog-upload=false", "/fixture/subject",
            )
            slsa_envelope = json.loads(slsa_output)
            slsa_statement = json.loads(base64.b64decode(slsa_envelope["payload"], validate=True))
            self.assertEqual(slsa_statement["predicate"], predicate)
            self.assertEqual(slsa_statement["predicateType"], "https://slsa.dev/provenance/v1")
            self.run_cosign(root, "verify-blob-attestation", "--key", "/fixture/local-key.pub",
                            "--bundle", "/fixture/slsa.bundle", "--type", "https://slsa.dev/provenance/v1",
                            "--offline", "--insecure-ignore-tlog", "/fixture/subject")
            self.run_cosign(root, "verify-blob-attestation", "--key", "/fixture/local-key.pub",
                            "--bundle", "/fixture/slsa.bundle", "--type", "https://slsa.dev/provenance/v1",
                            "--offline", "--insecure-ignore-tlog", "/fixture/wrong-subject", expected=1)

            spdx_output = self.run_cosign(
                root, "attest-blob", "--key", "/fixture/local-key.key", "--bundle", "/fixture/spdx.bundle",
                "--type", "spdxjson", "--predicate", "/fixture/spdx.json", "--tlog-upload=false",
                "/fixture/subject",
            )
            spdx_envelope = json.loads(spdx_output)
            spdx_statement = json.loads(base64.b64decode(spdx_envelope["payload"], validate=True))
            self.assertEqual(spdx_statement["predicate"], spdx)
            self.assertEqual(spdx_statement["predicateType"], "https://spdx.dev/Document")
            self.run_cosign(root, "verify-blob-attestation", "--key", "/fixture/local-key.pub",
                            "--bundle", "/fixture/spdx.bundle", "--type", "spdxjson",
                            "--offline", "--insecure-ignore-tlog", "/fixture/subject")

    def test_nonwritable_fixture_refuses_key_creation(self):
        with tempfile.TemporaryDirectory(prefix="cosign-offline-readonly-") as temporary:
            root = Path(temporary)
            root.chmod(0o500)
            try:
                self.run_cosign(root, "generate-key-pair", "--output-key-prefix", "/fixture/blocked-key",
                                expected=1)
                self.assertFalse((root / "blocked-key.key").exists())
                self.assertFalse((root / "blocked-key.pub").exists())
            finally:
                root.chmod(0o700)


if __name__ == "__main__":
    unittest.main()
