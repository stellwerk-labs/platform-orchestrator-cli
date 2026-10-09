#!/usr/bin/env python3
"""Exercise real Go archives and exact legal/provenance inspection boundaries."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

import qualify_release as release


class QualificationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory()
        cls.root = Path(cls.temporary.name)
        cls.module = "example.com/client"
        source = cls.root / "source"
        source.mkdir()
        (source / "go.mod").write_text("module " + cls.module + "\n\ngo 1.26.0\n")
        (source / "main.go").write_text("package main\nfunc main() {}\n")
        cls.binary = cls.root / "client"
        environment = {**os.environ, "GOTOOLCHAIN": "go1.26.2", "GOWORK": "off", "GOENV": "off",
                       "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0", "GOFLAGS": "", "GOEXPERIMENT": ""}
        subprocess.run(["go", "build", "-trimpath", "-o", str(cls.binary), "."], cwd=source,
                       env=environment, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.files = {"example.com/client/LICENSE": b"Own legal notice\n",
                      "go/LICENSE": b"Go legal notice\n", "go/PATENTS": b"Go patent grant\n",
                      "modules.json": json.dumps({"linux/amd64": {}}).encode()}

    def bundle(self, directory):
        directory.mkdir()
        for name, data in self.files.items():
            path = directory / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        (directory / "notices.sha256.json").write_text(json.dumps({
            name: hashlib.sha256(data).hexdigest() for name, data in self.files.items()
        }))

    def archive(self, kind="zip"):
        tree = self.root / "payload"
        tree.mkdir()
        self.bundle(tree / "licenses")
        shutil.copyfile(self.binary, tree / "client")
        (tree / "LICENSE").write_text("Own legal notice\n")
        (tree / "README.md").write_text("Public instructions\n")
        archive = self.root / ("payload." + kind)
        if kind == "zip":
            with zipfile.ZipFile(archive, "w") as output:
                for path in tree.rglob("*"):
                    if path.is_file():
                        output.write(path, path.relative_to(tree).as_posix())
        else:
            with tarfile.open(archive, "w:gz") as output:
                for path in tree.rglob("*"):
                    if path.is_file():
                        output.add(path, arcname=path.relative_to(tree).as_posix())
        return archive

    def test_actual_trimpath_binary_and_complete_notice_graph(self):
        for kind in ("zip", "tar.gz"):
            with self.subTest(kind=kind):
                archive = self.archive(kind)
                inspection = release.inspect_archive(archive, self.root / ("extract-" + kind), self.module)
                self.assertEqual(inspection["target"], "linux/amd64")
                self.assertEqual(inspection["dependencies"], {})
                self.assertEqual(inspection["binarySHA256"], release.sha(self.binary))
                shutil.rmtree(self.root / "payload")

    def test_notice_manifest_covers_nested_same_named_file(self):
        self.files["dependency/notices.sha256.json"] = b"Nested required bytes"
        bundle = self.root / "licenses"
        self.bundle(bundle)
        release.notices(bundle, self.module)
        (bundle / "dependency/notices.sha256.json").write_text("changed")
        with self.assertRaisesRegex(ValueError, "checksum manifest"):
            release.notices(bundle, self.module)

    def test_missing_or_extra_legal_bytes_fail(self):
        bundle = self.root / "licenses"
        self.bundle(bundle)
        (bundle / "go/PATENTS").unlink()
        with self.assertRaises(ValueError):
            release.notices(bundle, self.module)
        (bundle / "go/PATENTS").write_bytes(self.files["go/PATENTS"])
        (bundle / "untracked").write_bytes(b"extra")
        with self.assertRaises(ValueError):
            release.notices(bundle, self.module)

    def test_archive_notice_bytes_match_source_collected_union(self):
        expected = {name: hashlib.sha256(data).hexdigest() for name, data in self.files.items()}
        self.files["go/PATENTS"] = b"replacement grant"
        with self.assertRaisesRegex(ValueError, "source-collected notice union"):
            release.inspect_archive(self.archive(), self.root / "extract", self.module, expected)

    def test_checksums_bind_every_shipping_asset_without_duplicates(self):
        receipt_file = self.root / "release-content.json"
        receipt_file.write_text("qualified receipt")
        archive = self.root / "client.zip"
        archive.write_bytes(b"actual archive bytes")
        receipt = {"files": {archive.name: release.sha(archive)}, "source": {"version": "1.0.0"}}
        checksum = self.root / "checksums.txt"
        valid = (release.sha(archive) + "  client.zip\n" +
                 release.sha(receipt_file) + "  release-content.json\n")
        checksum.write_text(valid)
        release.verify_checksums(checksum, receipt, self.root)
        for changed in (valid + valid.splitlines()[0] + "\n", valid.splitlines()[0] + "\n",
                        valid.replace(release.sha(archive), "0" * 64)):
            checksum.write_text(changed)
            with self.assertRaises(ValueError):
                release.verify_checksums(checksum, receipt, self.root)

    def test_modified_receipt_is_rejected_before_signing_or_identity_tools(self):
        artifact = self.root / "release-content.json"
        artifact.write_text('{"source":"changed"}')
        stage = self.root / "qualified-content"
        stage.mkdir()
        (stage / "receipt.sha256").write_text("0" * 64 + "\n")
        with self.assertRaisesRegex(ValueError, "receipt changed"):
            release.recheck(self.root, artifact)

    def test_compiled_target_must_match_shipped_graph(self):
        self.files["modules.json"] = json.dumps({"linux/arm64": {}}).encode()
        with self.assertRaisesRegex(ValueError, "wrong source/toolchain/target/flags"):
            release.inspect_archive(self.archive(), self.root / "extract", self.module)

    def test_compiled_dependency_graph_cannot_be_invented(self):
        self.files["modules.json"] = json.dumps({"linux/amd64": {"example.com/not-compiled": "v1.0.0"}}).encode()
        self.files["example.com/not-compiled/LICENSE"] = b"Public notice"
        with self.assertRaisesRegex(ValueError, "compiled binary dependencies"):
            release.inspect_archive(self.archive(), self.root / "extract", self.module)

    def test_producer_statement_binds_actual_bytes_and_workflow(self):
        archive = self.archive()
        inspection = release.inspect_archive(archive, self.root / "extract", self.module)
        spdx = self.root / "payload.spdx.json"
        spdx.write_text('{"spdxVersion":"SPDX-2.3"}')
        producer = {"sourceSHA": "1" * 40, "workflowSHA": "2" * 40, "repository": "example/client",
                    "workflowRef": "example/client/.github/workflows/release.yml@refs/tags/v1.0.0",
                    "version": "1.0.0", "tag": "v1.0.0", "runID": "123", "runAttempt": "2"}
        statement = release.producer_statement(archive, spdx, inspection, producer)
        self.assertEqual(statement["subject"][0]["digest"]["sha256"], release.sha(archive))
        self.assertEqual(statement["subject"][1]["digest"]["sha256"], release.sha(spdx))
        definition = statement["predicate"]["buildDefinition"]
        self.assertEqual(definition["resolvedDependencies"][0]["digest"]["gitCommit"], producer["sourceSHA"])
        self.assertEqual(definition["resolvedDependencies"][1]["digest"]["gitCommit"], producer["workflowSHA"])
        self.assertNotIn("slsaLevel", json.dumps(statement))

    def test_pinned_syft_catalogs_real_go_binary(self):
        filesystem = self.root / "filesystem"
        filesystem.mkdir()
        shutil.copyfile(self.binary, filesystem / "client")
        output = self.root / "client.spdx.json"
        producer = {"created": "2026-10-09T00:00:00Z"}
        document = release.generate_spdx(filesystem, output, release.sha(self.binary), producer, dependencies={})
        self.assertEqual(document["creationInfo"]["created"], producer["created"])
        self.assertTrue(document["documentNamespace"].endswith(release.sha(self.binary)))
        self.assertTrue(any(package.get("name") == "stdlib" and package.get("versionInfo") == "go1.26.2"
                            for package in document["packages"]))
        with self.assertRaisesRegex(ValueError, "missing compiled dependency"):
            release.generate_spdx(filesystem, self.root / "missing.spdx.json", release.sha(self.binary), producer,
                                  dependencies={"example.com/not-compiled": "v1.0.0"})


if __name__ == "__main__":
    unittest.main()
