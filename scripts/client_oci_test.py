#!/usr/bin/env python3
"""Real OCI descriptor/layer fixtures, including secrets hidden by whiteouts."""
import hashlib
import ast
import io
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

import client_oci as oci
import client_image
from release_content_test import gzip_member


ROOT = Path(__file__).resolve().parents[1]
OWN = "example.com/client"


class OCITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if shutil.which("gitleaks") is None or subprocess.check_output(["gitleaks", "version"], text=True).strip() != "8.30.1":
            raise RuntimeError("pinned gitleaks 8.30.1 is mandatory")

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.layout = self.root / "layout"
        (self.layout / "blobs/sha256").mkdir(parents=True)
        (self.layout / "oci-layout").write_text('{"imageLayoutVersion":"1.0.0"}')

    def blob(self, data, media):
        if not isinstance(data, bytes):
            data = json.dumps(data, sort_keys=True).encode()
        digest = oci.digest(data)
        (self.layout / "blobs/sha256" / digest[7:]).write_bytes(data)
        return {"mediaType": media, "digest": digest, "size": len(data)}

    def layer(self, files):
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w") as archive:
            for name, value in files.items():
                entry = tarfile.TarInfo(name)
                entry.pax_headers = getattr(self, "pax_metadata", {})
                data, entry.mode = value
                entry.size = len(data)
                archive.addfile(entry, io.BytesIO(data))
        raw = output.getvalue() + getattr(self, "trailing_bytes", b"")
        if hasattr(self, "compress"):
            return self.blob(self.compress(raw), "application/vnd.oci.image.layer.v1.tar+gzip"), oci.digest(raw)
        return self.blob(raw, "application/vnd.oci.image.layer.v1.tar"), oci.digest(raw)

    def build(self, lower=None, extra=None, omit=None, user="65532", subject=None, arch="amd64", own=OWN):
        binary = bytearray(64)
        binary[:6] = b"\x7fELF\x02\x01"
        binary[18:20] = (62 if arch == "amd64" else 183).to_bytes(2, "little")
        notices = {own + "/LICENSE": b"Own legal notice", "go/LICENSE": b"Go legal notice",
                   "go/PATENTS": b"Go patent grant", "modules.json": json.dumps({"linux/" + arch: {}}).encode()}
        notices["notices.sha256.json"] = json.dumps({
            name: hashlib.sha256(data).hexdigest() for name, data in notices.items()
        }).encode()
        files = {"usr/local/bin/octl": (bytes(binary), 0o755)}
        files.update({"licenses/" + name: (data, 0o644) for name, data in notices.items()})
        if extra:
            files.update(extra)
        if omit:
            del files[omit]
        layers = [self.layer(value) for value in ([lower, files] if lower else [files])]
        config = self.blob({"os": "linux", "architecture": arch,
                            "config": {"Entrypoint": ["/usr/local/bin/octl"], "User": user},
                            "rootfs": {"type": "layers", "diff_ids": [value[1] for value in layers]}},
                           "application/vnd.oci.image.config.v1+json")
        platform = self.blob({"schemaVersion": 2, "config": config, "layers": [value[0] for value in layers]},
                             "application/vnd.oci.image.manifest.v1+json")
        platform["platform"] = {"os": "linux", "architecture": arch}
        attested = subject or platform["digest"][7:]
        attestations = [self.blob({"_type": "https://in-toto.io/Statement/v1",
                                  "subject": [{"name": "client", "digest": {"sha256": attested}}],
                                  "predicateType": predicate, "predicate": {}}, "application/vnd.in-toto+json")
                        for predicate in ("https://spdx.dev/Document", "https://slsa.dev/provenance/v1")]
        attestation = self.blob({"schemaVersion": 2, "config": self.blob({}, "application/vnd.oci.image.config.v1+json"),
                                 "layers": attestations}, "application/vnd.oci.image.manifest.v1+json")
        attestation.update(platform={"os": "unknown", "architecture": "unknown"},
                           annotations={"vnd.docker.reference.type": "attestation-manifest",
                                        "vnd.docker.reference.digest": platform["digest"]})
        top = self.blob({"schemaVersion": 2, "manifests": [platform, attestation]},
                        "application/vnd.oci.image.index.v1+json")
        top["annotations"] = {"org.opencontainers.image.ref.name": "qualified"}
        (self.layout / "index.json").write_text(json.dumps({"schemaVersion": 2, "manifests": [top]}))
        return top

    def multiarch(self):
        manifests = []
        for arch in ("amd64", "arm64"):
            top = self.build(arch=arch, own=client_image.release.module_path())
            manifests.extend(json.loads(oci.blob(self.layout, top))["manifests"])
        top = self.blob({"schemaVersion": 2, "manifests": manifests}, top["mediaType"])
        top["annotations"] = {"org.opencontainers.image.ref.name": "qualified"}
        (self.layout / "index.json").write_text(json.dumps({"schemaVersion": 2, "manifests": [top]}))

    def rewrite_metadata(self, kind, transform):
        """Rehash the real fixture graph, including original attestation subjects."""
        media = "application/vnd.oci.image." + kind + ".v1+json"
        rewritten, replacements = [], {}

        def rewrite(descriptor):
            value = json.loads(oci.blob(self.layout, descriptor))
            if "manifests" in value:
                value["manifests"] = [rewrite(item) for item in value["manifests"]]
            if descriptor["mediaType"] == "application/vnd.oci.image.manifest.v1+json":
                value["config"] = rewrite(value["config"])
                value["layers"] = [rewrite(item) if item["mediaType"].endswith("+json") else item
                                   for item in value["layers"]]
            raw = json.dumps(value, sort_keys=True).encode()
            for old, new in replacements.items():
                raw = raw.replace(old.encode(), new.encode())
            if descriptor["mediaType"] == media and not rewritten:
                raw = transform(raw)
                rewritten.append(raw)
            result = {**descriptor, **self.blob(raw, descriptor["mediaType"])}
            if "annotations" in result:
                result["annotations"] = {key: "sha256:" + replacements.get(value[7:], value[7:])
                                         if value.startswith("sha256:") else value
                                         for key, value in result["annotations"].items()}
            replacements[descriptor["digest"][7:]] = result["digest"][7:]
            return result

        index_path = self.layout / "index.json"
        index = json.loads(index_path.read_bytes())
        index["manifests"] = [rewrite(item) for item in index["manifests"]]
        index_path.write_text(json.dumps(index))
        self.assertEqual(len(rewritten), 1)
        return rewritten[0]

    def export(self):
        return oci.export(self.layout, self.root / "inspection", ("amd64",), OWN, ("usr/local/bin/octl",))

    def scan(self):
        return subprocess.run(["bash", str(ROOT / "scripts/scan-release-content.sh"),
                               str(self.root / "inspection"), "private-owner-fixture"],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)

    def test_exact_index_layers_binary_notices_and_attestations(self):
        top = self.build()
        report = self.export()
        self.assertEqual(report["index"], top["digest"])
        self.assertEqual(len(report["attestations"]), 1)
        self.assertEqual(oci.oci_reference(self.layout, top["digest"]), "qualified")
        self.assertEqual(self.scan().returncode, 0)

    def test_oras_platform_copy_preserves_manifest_media_type(self):
        if shutil.which("oras") is None:
            raise RuntimeError("pinned ORAS 1.3.4 is required, run scripts/install-oras.sh")
        self.build()
        report = self.export()
        manifest = report["payloads"]["amd64"]["manifest"]
        selected = self.root / "selected-platform"
        client_image.copy_platform_manifest(self.layout, selected, "amd64", report)
        descriptor = oci.strict_json((selected / "index.json").read_bytes())["manifests"][0]
        self.assertEqual(descriptor["digest"], manifest)
        self.assertEqual(descriptor["mediaType"], "application/vnd.oci.image.manifest.v1+json")
        legacy = self.root / "digest-selected"
        subprocess.run(["oras", "cp", "--no-tty", "--from-oci-layout", "--to-oci-layout",
                        str(self.layout) + "@" + manifest, str(legacy) + ":qualified"],
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
        with self.assertRaisesRegex(ValueError, "SBOM source differs"):
            oci.validate_sbom_layout(legacy, manifest)

    def test_builtin_sbom_and_daemon_use_verified_multiarch_index_pins(self):
        tools = client_image.image_tools()
        self.assertEqual(tools["buildkit"]["version"], "v0.33.1")
        self.assertEqual(tools["sbomGenerator"]["version"], "v1.12.0")
        self.assertEqual(tools["sbomGenerator"]["embeddedSyftModuleVersion"], "v1.51.0")
        workflow = (ROOT / ".github/workflows/ci.yaml").read_text()
        self.assertEqual(workflow.count("            image=" + tools["buildkit"]["image"]), 2)
        builder = (ROOT / "scripts/client_image.py").read_text()
        self.assertIn('"type=sbom,generator=" + tools["sbomGenerator"]["image"]', builder)
        publisher = (ROOT / "scripts/publish-qualified-image.py").read_text()
        self.assertNotIn('"buildx", "build"', publisher)
        self.assertIn("client_image.reinspect", publisher)

    def test_publisher_rechecks_status_before_copy_and_retains_signing(self):
        path = ROOT / "scripts/publish-qualified-image.py"
        source = path.read_text()
        tree = ast.parse(source)
        main = next(node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name == "main")
        publish_block = next(node for node in main.body if isinstance(node, ast.With))
        body = publish_block.body

        def call_name(call):
            return ast.unparse(call.func)

        def command_call(call, command, action):
            return (call_name(call) == "release.checked" and call.args and isinstance(call.args[0], ast.List)
                    and [item.value for item in call.args[0].elts[:2]
                         if isinstance(item, ast.Constant)] == [command, action])

        statements = [
            index for index, statement in enumerate(body)
            if any(isinstance(node, ast.Call) and call_name(node) == "candidate.assert_image_absent"
                   for node in ast.walk(statement))
        ]
        copy_statements = [
            index for index, statement in enumerate(body)
            if any(isinstance(node, ast.Call) and command_call(node, "oras", "cp")
                   for node in ast.walk(statement))
        ]
        self.assertEqual(len(statements), 1)
        self.assertEqual(len(copy_statements), 1)
        self.assertEqual(copy_statements[0], statements[0] + 1)

        calls = sorted((node for node in ast.walk(main) if isinstance(node, ast.Call)), key=lambda node: node.lineno)
        guard_line = next(node.lineno for node in calls if call_name(node) == "candidate.assert_image_absent")
        reinspection_line = next(node.lineno for node in calls if call_name(node) == "client_image.reinspect")
        source_reference_line = next(node.lineno for node in calls if call_name(node) == "client_oci.oci_reference")
        copy_line = next(node.lineno for node in calls if command_call(node, "oras", "cp"))
        sign_line = next(node.lineno for node in calls if command_call(node, "cosign", "sign"))
        attest_line = next(node.lineno for node in calls if command_call(node, "cosign", "attest"))
        verify_descriptor_line = next(
            node.lineno for node in calls if call_name(node) == "release.checked"
            and any(isinstance(item, ast.Constant) and item.value == "fetch" for item in ast.walk(node.args[0]))
        )
        self.assertLess(reinspection_line, source_reference_line)
        self.assertLess(source_reference_line, guard_line)
        self.assertLess(guard_line, copy_line)
        self.assertLess(copy_line, verify_descriptor_line)
        self.assertLess(verify_descriptor_line, sign_line)
        self.assertLess(sign_line, attest_line)
        self.assertNotIn("authenticated_absence", source)
        self.assertNotIn("MANIFEST_UNKNOWN", source)
        self.assertNotIn('"buildx", "build"', source)
        for required in (
            "client_image.reinspect", "client_oci.oci_reference", "--from-oci-layout",
            '"cosign", "sign"', '"cosign", "verify"', '"slsaprovenance1"', '"spdxjson"',
            "verify_attestation",
        ):
            self.assertIn(required, source)

    def test_original_json_bytes_are_retained_and_scanned(self):
        for kind in ("index", "manifest", "config"):
            for value in (b"private-owner-fixture", b"private-owner-fixtur\\u0065"):
                with self.subTest(kind=kind, escaped=b"\\u" in value):
                    self.build()
                    raw = self.rewrite_metadata(kind, lambda data: b'{ "audit":"' + value + b'",\n' + data[1:])
                    try:
                        self.export()
                        metadata = self.root / "inspection/metadata"
                        self.assertTrue(any(path.read_bytes() == raw for path in metadata.iterdir()))
                        result = self.scan()
                        self.assertNotEqual(result.returncode, 0)
                        self.assertNotIn(b"private-owner-fixture", result.stdout + result.stderr)
                    finally:
                        if (self.root / "inspection").exists():
                            shutil.rmtree(self.root / "inspection")

    def test_duplicate_json_keys_fail_at_producer_and_publisher_boundaries(self):
        for kind in ("index", "manifest", "config"):
            for prefix in (b'{"audit":"private-owner-fixture","audit":"public",',
                           b'{"nested":{"audit":"private-owner-fixture","aud\\u0069t":"public"},'):
                with self.subTest(kind=kind, prefix=prefix[:12]):
                    self.multiarch()
                    self.rewrite_metadata(kind, lambda data: prefix + data[1:])
                    producer = self.root / "producer"
                    with self.assertRaisesRegex(ValueError, "duplicate OCI JSON key"):
                        client_image.inspect(self.layout, producer, ("amd64", "arm64"), client_image.release.module_path())
                    dist, receipt = self.publisher_fixture()
                    with self.assertRaisesRegex(ValueError, "duplicate OCI JSON key"):
                        client_image.reinspect(dist, receipt, self.root / "publisher")
                    for path in (producer, dist, self.root / "publisher"):
                        if path.exists():
                            shutil.rmtree(path)

    def publisher_fixture(self, report=None):
        dist = self.root / "dist"
        layout = dist / "qualified-content/image-layout"
        layout.parent.mkdir(parents=True)
        shutil.copytree(self.layout, layout)
        source = {"sourceSHA": "1" * 40}
        (dist / "octl-image-content.json").write_text(json.dumps({"sourceSHA": source["sourceSHA"],
            "image": client_image.IMAGE, "buildTools": client_image.image_tools(), **(report or {})}))
        top = json.loads((layout / "index.json").read_bytes())["manifests"][0]
        return dist, {"source": source, "image": {"layout": "qualified-content/image-layout",
                                                 "report": "octl-image-content.json", "digest": top["digest"]}}

    def test_actual_producer_and_publisher_inspection_scan_metadata(self):
        self.multiarch()
        report = client_image.inspect(self.layout, self.root / "producer", ("amd64", "arm64"), client_image.release.module_path())
        dist, receipt = self.publisher_fixture(report)
        self.assertEqual(client_image.reinspect(dist, receipt, self.root / "publisher"), report)
        for path in (self.root / "producer", dist, self.root / "publisher"):
            shutil.rmtree(path)
        token = ("gh" + "p_" + "4Ax9eC2mF7qR5uT8vW3zB6dH1jK0nP9sY2aL").encode()
        for kind in ("index", "manifest", "config"):
            with self.subTest(kind=kind):
                self.multiarch()
                self.rewrite_metadata(kind, lambda data: b'{"audit":"' + token + b' # gitleaks:allow",' + data[1:])
                with self.assertRaisesRegex(ValueError, "release inspection tool failed"):
                    client_image.inspect(self.layout, self.root / "producer", ("amd64", "arm64"), client_image.release.module_path())
                dist, receipt = self.publisher_fixture()
                with self.assertRaisesRegex(ValueError, "release inspection tool failed"):
                    client_image.reinspect(dist, receipt, self.root / "publisher")
                for path in (self.root / "producer", dist, self.root / "publisher"):
                    shutil.rmtree(path)

    def test_deleted_lower_layer_secret_is_still_scanned(self):
        token = ("gh" + "p_" + "4Ax9eC2mF7qR5uT8vW3zB6dH1jK0nP9sY2aL").encode()
        self.build(lower={"deleted-credential": (b"\x7fELF\0" + token + b"\0", 0o644)},
                   extra={".wh.deleted-credential": (b"", 0o644)})
        self.export()
        self.assertFalse((self.root / "inspection/amd64/deleted-credential").exists())
        result = self.scan()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(token, result.stdout + result.stderr)

    def test_original_layer_path_metadata_is_scanned(self):
        self.build(lower={"private-owner-fixture/notice": (b"Public harmless notice", 0o644)},
                   extra={"private-owner-fixture/.wh.notice": (b"", 0o644)})
        self.export()
        self.assertNotEqual(self.scan().returncode, 0)

    def test_pax_headers_and_unparsed_trailing_bytes_are_scanned(self):
        token = ("gh" + "p_" + "4Ax9eC2mF7qR5uT8vW3zB6dH1jK0nP9sY2aL").encode()
        self.pax_metadata = {"public.metadata": token.decode() + " # gitleaks:allow"}
        self.build()
        self.export()
        result = self.scan()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(token, result.stdout + result.stderr)
        shutil.rmtree(self.root / "inspection")
        self.pax_metadata = {}
        self.trailing_bytes = b"\0" + token + b"\0"
        self.build()
        self.export()
        self.assertNotEqual(self.scan().returncode, 0)

    def test_ignore_file_is_rejected_before_any_scanner(self):
        self.build(extra={".gitleaksignore": (b"ignored fingerprint", 0o644)})
        with self.assertRaisesRegex(ValueError, "ignore files"):
            self.export()

    def test_gzip_optional_metadata_in_every_member_is_scanned(self):
        token = ("gh" + "p_" + "4Ax9eC2mF7qR5uT8vW3zB6dH1jK0nP9sY2aL").encode()
        for field in ("name", "comment", "extra"):
            with self.subTest(field=field):
                self.compress = lambda raw: gzip_member(raw[:512], name=b"public-layer", header_crc=True) + gzip_member(
                    raw[512:], header_crc=True, **{field: token + b" # gitleaks:allow"})
                self.build()
                self.export()
                result = self.scan()
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn(token, result.stdout + result.stderr)
                shutil.rmtree(self.root / "inspection")

    def test_missing_patent_notice_and_wrong_user_fail(self):
        self.build(omit="licenses/go/PATENTS")
        with self.assertRaisesRegex(ValueError, "missing required"):
            self.export()
        shutil.rmtree(self.root / "inspection")
        self.build(user="0")
        with self.assertRaisesRegex(ValueError, "entrypoint or user"):
            self.export()

    def test_wrong_attestation_subject_fails(self):
        self.build(subject="0" * 64)
        with self.assertRaisesRegex(ValueError, "attestation subject"):
            self.export()

    def test_blob_replacement_is_not_accepted(self):
        top = self.build()
        (self.layout / "blobs/sha256" / top["digest"][7:]).write_bytes(b"replaced")
        with self.assertRaisesRegex(ValueError, "digest or size"):
            self.export()

    def test_traversal_layer_path_is_not_materialized(self):
        self.build(extra={"../outside": (b"no host writes", 0o644)})
        with self.assertRaisesRegex(ValueError, "unsafe OCI layer path"):
            self.export()
        self.assertFalse((self.root / "outside").exists())


if __name__ == "__main__":
    unittest.main()
