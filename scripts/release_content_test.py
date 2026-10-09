#!/usr/bin/env python3
"""Real scanner and bounded archive regressions, no skipped required behavior."""
import io
import gzip
import json
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile
import zlib

import release_content as content


ROOT = Path(__file__).resolve().parents[1]


def gzip_member(raw, name=b"", comment=b"", extra=b"", header_crc=False):
    flags = (8 if name else 0) | (16 if comment else 0) | (4 if extra else 0) | (2 if header_crc else 0)
    header = b"\x1f\x8b\x08" + bytes([flags]) + b"\0" * 6
    if extra:
        header += len(extra).to_bytes(2, "little") + extra
    if name:
        header += name + b"\0"
    if comment:
        header += comment + b"\0"
    if header_crc:
        header += (zlib.crc32(header) & 0xffff).to_bytes(2, "little")
    compressor = zlib.compressobj(wbits=-15)
    return (header + compressor.compress(raw) + compressor.flush()
            + zlib.crc32(raw).to_bytes(4, "little") + (len(raw) & 0xffffffff).to_bytes(4, "little"))


class ReleaseContentTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if shutil.which("gitleaks") is None:
            raise RuntimeError("pinned gitleaks 8.30.1 is required, run scripts/install-release-tools.sh")
        if subprocess.check_output(["gitleaks", "version"], text=True).strip() != "8.30.1":
            raise RuntimeError("pinned gitleaks 8.30.1 is required")

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.shipping = self.root / "shipping"
        self.shipping.mkdir()

    def scan(self, *literals, script=None):
        return subprocess.run(
            ["bash", str(script or ROOT / "scripts/scan-release-content.sh"), str(self.shipping), *literals],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )

    @staticmethod
    def token():
        return ("gh" + "p_" + "4Ax9eC2mF7qR5uT8vW3zB6dH1jK0nP9sY2aL").encode()

    def test_real_scanner_rejects_binary_secret_and_inline_bypass(self):
        path = self.shipping / "application"
        path.write_bytes(b"\x7fELF\x02\x01\0" + self.token() + b" # gitleaks:allow\0")
        result = self.scan()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(self.token(), result.stdout + result.stderr)

    def test_real_scanner_ignores_source_controlled_ignore_file(self):
        path = self.shipping / "fixture.txt"
        path.write_bytes(self.token() + b"\n")
        report = self.root / "report.json"
        result = subprocess.run(
            ["gitleaks", "dir", "--redact", "--no-banner", "--log-level", "error",
             "--report-format", "json", "--report-path", str(report), "."],
            cwd=self.shipping, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )
        self.assertEqual(result.returncode, 1)
        findings = json.loads(report.read_text())
        self.assertTrue(findings)
        (self.shipping / ".gitleaksignore").write_text("\n".join(f["Fingerprint"] for f in findings))
        self.assertNotEqual(self.scan().returncode, 0)

    def test_real_scanner_rejects_text_inline_allow(self):
        (self.shipping / "fixture.txt").write_bytes(self.token() + b" # gitleaks:allow\n")
        self.assertNotEqual(self.scan().returncode, 0)

    def test_invalid_scanner_configuration_fails_closed(self):
        scripts = self.root / "tool/scripts"
        scripts.mkdir(parents=True)
        for name in ("scan-release-content.sh", "release-tools.sh", "release_content.py"):
            shutil.copyfile(ROOT / "scripts" / name, scripts / name)
        (scripts / "gitleaks.toml").write_text("this is not TOML [")
        self.assertNotEqual(self.scan(script=scripts / "scan-release-content.sh").returncode, 0)

    def test_paths_metadata_and_archive_contents_are_scanned(self):
        (self.shipping / "release.provenance.json").write_text('{"source":"private-owner-fixture"}')
        self.assertNotEqual(self.scan("private-owner-fixture").returncode, 0)
        (self.shipping / "release.provenance.json").unlink()
        with zipfile.ZipFile(self.shipping / "payload.zip", "w") as archive:
            archive.writestr("licenses/private-owner-fixture/NOTICE", "Public harmless notice")
        self.assertNotEqual(self.scan("private-owner-fixture").returncode, 0)
        (self.shipping / "payload.zip").unlink()
        with tarfile.open(self.shipping / "payload.tar.gz", "w:gz") as archive:
            member = tarfile.TarInfo("application")
            payload = b"\x7fELF\0" + self.token() + b"\0"
            member.size = len(payload)
            archive.addfile(member, io.BytesIO(payload))
        self.assertNotEqual(self.scan().returncode, 0)

    def test_bounded_unpack_rejects_unsafe_members_and_size(self):
        for kind in ("traversal", "link", "duplicate"):
            path = self.shipping / "payload.tar"
            with tarfile.open(path, "w") as archive:
                member = tarfile.TarInfo("../outside" if kind == "traversal" else "entry")
                if kind == "link":
                    member.type = tarfile.SYMTYPE
                    member.linkname = "../outside"
                archive.addfile(member)
                if kind == "duplicate":
                    archive.addfile(member)
            with self.assertRaisesRegex(ValueError, "unsupported archive scan entry" if kind == "link" else "unsafe or duplicate"):
                content.unpack(self.shipping, self.root / kind, max_bytes=20 * 1024)
            path.unlink()
        with zipfile.ZipFile(self.shipping / "payload.zip", "w", compression=zipfile.ZIP_DEFLATED) as archive:
            archive.writestr("oversized", b"x" * 251)
        with self.assertRaisesRegex(ValueError, "content exceeds scan limit"):
            content.unpack(self.shipping, self.root / "bounded", max_bytes=250)

    def test_gzip_optional_metadata_in_every_member_is_scanned(self):
        raw = io.BytesIO()
        with tarfile.open(fileobj=raw, mode="w") as archive:
            entry = tarfile.TarInfo("notice")
            entry.size = 6
            archive.addfile(entry, io.BytesIO(b"public"))
        for field in ("name", "comment", "extra"):
            with self.subTest(field=field):
                data = gzip_member(raw.getvalue()[:512], name=b"public-base-layer", header_crc=True) + gzip_member(
                    raw.getvalue()[512:], header_crc=True, **{field: self.token() + b" # gitleaks:allow"})
                (self.shipping / "payload.tar.gz").write_bytes(data)
                result = self.scan()
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn(self.token(), result.stdout + result.stderr)

    def test_gzip_member_validation_and_bounds(self):
        first = gzip_member(b"first", name=b"base", comment=b"public", extra=b"metadata", header_crc=True)
        second = gzip_member(b"second")
        raw, metadata = content.decode_gzip_members(first + b"\0\0" + second + b"\0")
        self.assertEqual(raw, b"firstsecond")
        self.assertIn(b"base\0public\0", metadata)
        self.assertIn(b"metadata", metadata)
        self.assertTrue(metadata.endswith(b"\0"))
        for malformed in (b"", b"\0" + first, first[:-1], first + b"not-gzip", first[:3] + b"\xe0" + first[4:],
                          first[:-8] + bytes([first[-8] ^ 1]) + first[-7:],
                          first[:-4] + bytes([first[-4] ^ 1]) + first[-3:],
                          first[:10] + b"\xff\xff" + first[12:],
                          gzip_member(b"data", name=b"name")[:12]):
            with self.subTest(malformed=hashlib.sha256(malformed).hexdigest()), self.assertRaises(ValueError):
                content.decode_gzip_members(malformed)
        for options in ({"max_output": 4}, {"max_metadata": 8}, {"max_members": 1}, {"max_members": 0}):
            with self.subTest(options=options), self.assertRaises(ValueError):
                content.decode_gzip_members(first + second, **options)


    def test_compressed_tar_pax_and_trailing_bytes_are_scanned(self):
        for kind in ("pax", "trailing"):
            with self.subTest(kind=kind):
                raw = io.BytesIO()
                with tarfile.open(fileobj=raw, mode="w") as archive:
                    entry = tarfile.TarInfo("notice")
                    if kind == "pax":
                        entry.pax_headers = {"public.metadata": self.token().decode() + " # gitleaks:allow"}
                    entry.size = 6
                    archive.addfile(entry, io.BytesIO(b"public"))
                data = raw.getvalue() + (b"\0" + self.token() + b"\0" if kind == "trailing" else b"")
                (self.shipping / "payload.tar.gz").write_bytes(gzip.compress(data, mtime=0))
                result = self.scan()
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn(self.token(), result.stdout + result.stderr)

    def test_clean_content_and_shipping_paths_pass(self):
        path = self.shipping / "licenses/example.com/module"
        path.mkdir(parents=True)
        (path / "LICENSE").write_text("Public legal notice\n")
        self.assertEqual(self.scan("private-owner-fixture").returncode, 0)

    def test_checksum_exception_requires_complete_actual_content_proof(self):
        file = self.shipping / "api_key"
        file.write_text("Public redistribution file\\n")
        manifest = self.shipping / "notices.sha256.json"
        manifest.write_text(json.dumps({file.name: hashlib.sha256(file.read_bytes()).hexdigest()}))
        result = self.scan()
        self.assertEqual(result.returncode, 0)
        self.assertIn(b"checksum findings", result.stderr)
        file.write_text("changed public bytes\\n")
        self.assertNotEqual(self.scan().returncode, 0)
        file.write_bytes(self.token())
        manifest.write_text(json.dumps({file.name: hashlib.sha256(file.read_bytes()).hexdigest()}))
        self.assertNotEqual(self.scan().returncode, 0)

    @staticmethod
    def public_binary_context():
        # Each public source literal is independently proven by the release
        # review, the finding crosses all six compiler-adjacent strings.
        return b"".join((b"secret", b"STREET", b" 15:04", b"gb2312", b"LC_ALL", b"nil key"))

    def test_binary_exception_requires_every_exact_proof_boundary(self):
        context = self.public_binary_context()
        policy = json.loads((ROOT / "scripts/known-binary-strings.json").read_text())
        self.assertEqual(policy["policy"], content.BINARY_POLICY)
        self.assertEqual(policy["module"], content.BINARY_MODULE)
        self.assertEqual(policy["version"], content.BINARY_VERSION)
        self.assertEqual(policy["sum"], content.BINARY_SUM)
        self.assertEqual(policy["matchSHA256"], content.BINARY_MATCH)
        self.assertEqual(policy["secretSHA256"], content.BINARY_SECRET)
        self.assertEqual(policy["contextSHA256"], content.BINARY_CONTEXT)
        self.assertEqual(sum(source["length"] for source in policy["sources"]), 37)
        self.assertEqual(hashlib.sha256(context).hexdigest(), content.BINARY_CONTEXT)
        binary = bytearray(64)
        binary[:6] = b"\x7fELF\x02\x01"
        binary[18:20] = (62).to_bytes(2, "little")
        data = bytes(binary) + b"\0" + context + b"\0"
        finding = {"RuleID": "generic-api-key", "Match": context[:34].decode(), "Secret": context[16:33].decode()}
        self.assertEqual(hashlib.sha256(finding["Secret"].encode()).hexdigest(), content.BINARY_SECRET)
        sums = " ".join((content.BINARY_MODULE, content.BINARY_VERSION, content.BINARY_SUM))
        metadata = {"GoVersion": "go1.26.2", "Path": "github.com/stellwerk-labs/platform-orchestrator-cli",
                    "Settings": [{"Key": "GOOS", "Value": "linux"}, {"Key": "GOARCH", "Value": "amd64"}],
                    "Deps": [{"Path": content.BINARY_MODULE, "Version": content.BINARY_VERSION, "Sum": content.BINARY_SUM}]}
        self.assertTrue(content.known_binary_finding(data, finding, sums, metadata))
        for kind in ("rule", "match", "secret", "context", "extra-occurrence", "header", "sum", "version", "replace", "toolchain", "target"):
            changed_data, changed_finding, changed_sums = data, dict(finding), sums
            changed_info = json.loads(json.dumps(metadata))
            if kind in ("rule", "match", "secret"):
                changed_finding[{"rule": "RuleID", "match": "Match", "secret": "Secret"}[kind]] += "changed"
            elif kind == "context":
                changed_data = data[:-2] + b"X\0"
            elif kind == "extra-occurrence":
                changed_data += context[:34] + b"wrong boundary"
            elif kind == "header":
                changed_data = b"invalid" + data[7:]
            elif kind == "sum":
                changed_sums += "changed"
            elif kind == "version":
                changed_info["Deps"][0]["Version"] = "v0.0.17"
            elif kind == "replace":
                changed_info["Deps"][0]["Replace"] = {"Path": "example.com/replacement"}
            elif kind == "toolchain":
                changed_info["GoVersion"] = "go1.26.1"
            elif kind == "target":
                changed_info["Settings"][1]["Value"] = "arm64"
            with self.subTest(kind=kind):
                self.assertFalse(content.known_binary_finding(changed_data, changed_finding, changed_sums, changed_info))

    def test_real_elf_known_public_boundary_and_additional_secret(self):
        source = self.root / "source"
        source.mkdir()
        (source / "go.mod").write_text("module github.com/stellwerk-labs/platform-orchestrator-cli\n\ngo 1.26.0\n\nrequire github.com/mattn/go-runewidth v0.0.16\n")
        shutil.copyfile(ROOT / "go.sum", source / "go.sum")
        context_literal = json.dumps(self.public_binary_context().decode())
        (source / "main.go").write_text('package main\nimport ("fmt"; "github.com/mattn/go-runewidth")\nfunc main(){fmt.Print(' +
                                       context_literal + ', runewidth.StringWidth("x"))}\n')
        binary = self.shipping / "application"
        environment = {**os.environ, "GOTOOLCHAIN": "go1.26.2", "GOWORK": "off", "GOENV": "off",
                       "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0", "GOFLAGS": "", "GOEXPERIMENT": ""}
        subprocess.run(["go", "build", "-mod=mod", "-trimpath", "-o", str(binary), "."], cwd=source,
                       env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
        original = binary.read_bytes()
        result = self.scan()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertIn(b"source-proven binary findings", result.stderr)
        binary.write_bytes(original + b"\0" + self.token() + b" # gitleaks:allow\0")
        result = self.scan()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(self.token(), result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
