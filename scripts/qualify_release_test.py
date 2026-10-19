#!/usr/bin/env python3
"""Exercise real Go archives and exact legal/provenance inspection boundaries."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
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

    def test_buildx_git_discovery_is_subprocess_scoped(self):
        import client_image
        parent = {"PATH": "/retained/tool/path", "BUILDX_GIT_INFO": "1"}
        environment = client_image.build_environment(parent)
        self.assertEqual(parent["BUILDX_GIT_INFO"], "1")
        self.assertEqual(environment, {"PATH": "/retained/tool/path", "BUILDX_GIT_INFO": "0"})
        self.assertEqual(client_image.build_environment({}), {"BUILDX_GIT_INFO": "0"})
        actual = release.checked([sys.executable, "-c", "import os; print(os.environ['BUILDX_GIT_INFO']); "
                                  "print(os.environ['PATH'])"], env=environment)
        self.assertEqual(actual.decode(), "0\n/retained/tool/path\n")


class DiagnosticTest(unittest.TestCase):
    SENTINEL = "credential-value-must-never-appear"

    def test_real_child_exit_and_all_output_are_closed(self):
        command = [sys.executable, "-c", "import sys; print(sys.argv[1]); print(sys.argv[1],file=sys.stderr); sys.exit(37)", self.SENTINEL]
        with self.assertRaises(release.CheckedToolFailure) as direct:
            release.checked(command)
        self.assertEqual(direct.exception.returncode, 37)
        self.assertEqual(direct.exception.tool, "other")
        self.assertEqual(direct.exception.args, ("release inspection tool failed",))
        self.assertNotIn(self.SENTINEL, str(direct.exception) + repr(direct.exception))
        with self.assertRaises(release.QualificationStageFailure) as staged:
            release.at_stage("archive-spdx:2/5", release.checked, command)
        self.assertEqual((staged.exception.category, staged.exception.tool, staged.exception.returncode), ("checked-tool", "other", 37))
        self.assertIsNone(staged.exception.rule_id)
        self.assertNotIn(self.SENTINEL, str(staged.exception) + repr(staged.exception))

    def test_dual_loaded_checked_retains_original_exit(self):
        spec = importlib.util.spec_from_file_location("qualification_entry_point", release.__file__)
        entry_point = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(entry_point)
        self.assertIsNot(entry_point.CheckedToolFailure, release.CheckedToolFailure)
        with self.assertRaises(release.QualificationStageFailure) as caught:
            release.at_stage("oci-image", entry_point.checked,
                             [sys.executable, "-c", "import sys; sys.exit(37)", self.SENTINEL])
        self.assertEqual((caught.exception.category, caught.exception.returncode), ("checked-tool", 37))
        self.assertNotIn(self.SENTINEL, str(caught.exception))

    def test_dynamic_exception_and_os_path_are_never_rules_or_output(self):
        def invalid():
            raise ValueError(self.SENTINEL)
        with self.assertRaises(release.QualificationStageFailure) as dynamic:
            release.at_stage("source-notices", invalid)
        self.assertEqual(dynamic.exception.category, "ValueError")
        self.assertIsNone(dynamic.exception.rule_id)
        with tempfile.TemporaryDirectory() as temporary:
            with self.assertRaises(release.QualificationStageFailure) as path:
                release.at_stage("archive-inspection", (Path(temporary) / self.SENTINEL).read_bytes)
        self.assertEqual(path.exception.category, "OSError")
        self.assertIsNone(path.exception.rule_id)
        self.assertNotIn(self.SENTINEL, str(dynamic.exception) + str(path.exception))

    def test_only_actual_source_literal_is_hashed(self):
        with tempfile.TemporaryDirectory() as temporary:
            legal = Path(temporary)
            (legal / "notices.sha256.json").write_text("{}")
            with self.assertRaises(release.QualificationStageFailure) as caught:
                release.at_stage("source-notices", release.notices, legal, "example/client")
        self.assertEqual(caught.exception.category, "source-literal")
        self.assertEqual(caught.exception.rule_id, "qualify_release.py:sha256:" + hashlib.sha256(
            b"notice checksum manifest does not cover exact shipped content").hexdigest())

    def test_diagnostic_fields_refuse_unbounded_or_arbitrary_values(self):
        for kwargs in ({"stage": self.SENTINEL}, {"category": self.SENTINEL}, {"tool": self.SENTINEL},
                       {"rule_id": self.SENTINEL}, {"returncode": self.SENTINEL}, {"returncode": 10 ** 100},
                       {"stage": "archive-spdx:100000/5"}):
            values = {"stage": "qualify", **kwargs}
            with self.subTest(field=next(iter(kwargs))), self.assertRaises(ValueError) as caught:
                release.QualificationStageFailure(**values)
            self.assertNotIn(self.SENTINEL, str(caught.exception))

    def test_imported_run_and_real_main_emit_closed_nonzero_diagnostics(self):
        cases = ([sys.executable, "-B", str(release.ROOT / "scripts/qualify_release.py"), "qualify", self.SENTINEL],
                 [sys.executable, "-B", "-c", "import qualify_release as q,sys; sys.argv=['qualify_release.py',sys.argv[1]]; sys.exit(q.run())", self.SENTINEL])
        for command in cases:
            with self.subTest(entrypoint=command[2]):
                result = subprocess.run(command, env={**os.environ, "QUALIFY_SOURCE_SHA": "0" * 40,
                    "PYTHONPATH": str(release.ROOT / "scripts")}, capture_output=True, text=True, timeout=20)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")
                self.assertRegex(result.stderr, r"^client release qualification failed at (source-identity|entrypoint) class=source-literal rule=qualify_release.py:sha256:[0-9a-f]{64}\n$")
                self.assertNotIn(self.SENTINEL, result.stderr)

    def test_actual_entrypoints_report_real_git128_without_repo_or_path_leak(self):
        with tempfile.TemporaryDirectory(prefix="client-diagnostic-no-git-") as temporary:
            source = Path(temporary) / self.SENTINEL / "source"
            scripts = source / "scripts"
            scripts.mkdir(parents=True)
            for name in ("qualify_release.py", "release_content.py"):
                shutil.copyfile(release.ROOT / "scripts" / name, scripts / name)
            environment = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
            environment.update(GIT_CEILING_DIRECTORIES=str(Path(temporary).resolve()),
                               GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
                               PYTHONPATH=str(scripts), PYTHONDONTWRITEBYTECODE="1")
            commands = ([sys.executable, "-B", str(scripts / "qualify_release.py"), "qualify", self.SENTINEL],
                        [sys.executable, "-B", "-c", "import qualify_release as q,sys; sys.argv=['qualify_release.py','qualify',sys.argv[1]]; sys.exit(q.run())", self.SENTINEL])
            for command in commands:
                with self.subTest(entrypoint=command[2]):
                    result = subprocess.run(command, cwd=source, env=environment,
                                            capture_output=True, text=True, timeout=20)
                    self.assertEqual(result.returncode, 1)
                    self.assertEqual(result.stdout, "")
                    self.assertEqual(result.stderr, "client release qualification failed at source-identity class=checked-tool tool=git exit=128\n")
                    self.assertNotIn(self.SENTINEL, result.stderr)
                    self.assertNotIn(str(source), result.stderr)
                    self.assertNotIn("not a git repository", result.stderr)


if __name__ == "__main__":
    unittest.main()
