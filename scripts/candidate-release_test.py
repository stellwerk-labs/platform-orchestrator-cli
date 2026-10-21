#!/usr/bin/env python3
import importlib.util
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import hashlib
import shutil
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import threading
import unittest
import urllib.parse


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


candidate = load("candidate", "candidate-release.py")
metadata = load("metadata", "candidate-release-metadata.py")
SHA = "a" * 40


class CandidateReleaseHandler(BaseHTTPRequestHandler):
    state = {}

    def log_message(self, format, *args):
        pass

    def respond(self, status, body=None):
        data = json.dumps(body).encode() if body is not None else b""
        self.send_response(status)
        if body is not None:
            self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        state = self.state
        if self.path.startswith(f"/repos/{metadata.REPOSITORY}/releases?"):
            state["events"].append("list")
            return self.respond(200, state["release_inventory"])
        if self.path == f"/repos/{metadata.REPOSITORY}/git/ref/tags/{state['tag']}":
            return self.respond(200, {"ref": state.get("returned_ref", f"refs/tags/{state['tag']}"),
                                      "object": {"sha": state["tag_object"], "type": "tag"}})
        if self.path == f"/repos/{metadata.REPOSITORY}/git/tags/{state['tag_object']}":
            return self.respond(200, {"sha": state.get("returned_tag_sha", state["tag_object"]),
                                      "tag": state.get("returned_tag_name", state["tag"]),
                                      "object": {"sha": state["source"], "type": "commit"}})
        if self.path == f"/repos/{metadata.REPOSITORY}/releases/{state['release_id']}":
            state["events"].append("get")
            return self.respond(200, state["release"])
        return self.respond(404, {"message": "not found"})

    def do_POST(self):
        state = self.state
        length = int(self.headers.get("Content-Length", "0"))
        payload = self.rfile.read(length)
        release_path = f"/repos/{metadata.REPOSITORY}/releases"
        if self.path == release_path:
            state["events"].append("create")
            request = json.loads(payload)
            state["create_request"] = request
            state["release"] = {"id": state["release_id"], "tag_name": request["tag_name"],
                                "name": request["name"], "body": request["body"],
                                "prerelease": request["prerelease"], "draft": request["draft"],
                                "assets": state.get("create_assets", [])}
            return self.respond(201, state["release"])
        prefix = f"/repos/{metadata.REPOSITORY}/releases/{state['release_id']}/assets?"
        if self.path.startswith(prefix):
            state["events"].append("upload")
            if state.get("fail_upload") and len(state["release"]["assets"]) >= state.get("fail_upload_after", 1):
                return self.respond(500, {"message": "fixture upload failure"})
            name = urllib.parse.parse_qs(urllib.parse.urlsplit(self.path).query)["name"][0]
            asset_id = 1000 + len(state["release"]["assets"])
            asset = {"id": asset_id, "name": name, "size": len(payload),
                     "digest": "sha256:" + hashlib.sha256(payload).hexdigest(), "state": "uploaded"}
            state["release"]["assets"].append(asset)
            return self.respond(201, asset)
        return self.respond(404, {"message": "not found"})

    def do_PATCH(self):
        state = self.state
        state["events"].append("patch")
        if self.path != f"/repos/{metadata.REPOSITORY}/releases/{state['release_id']}":
            return self.respond(404, {"message": "not found"})
        request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
        state["release"].update(request)
        if state.get("fail_finalize"):
            return self.respond(500, {"message": "fixture finalization failure"})
        return self.respond(200, state["release"])


class RedirectHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def redirect(self, method):
        self.server.requests.append((method, self.path, self.headers.get("Authorization")))
        self.send_response(self.server.status)
        self.send_header("Location", self.server.peer + "/received")
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_GET(self):
        self.redirect("GET")

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.redirect("POST")


class RedirectTargetHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def do_GET(self):
        self.server.requests.append(("GET", self.path, self.headers.get("Authorization")))
        self.send_response(200)
        self.end_headers()

    def do_POST(self):
        self.server.requests.append(("POST", self.path, self.headers.get("Authorization")))
        self.send_response(200)
        self.end_headers()

class CandidateReleaseHTTPFixture:
    def __init__(self, test, fail_upload=False, fail_finalize=False):
        self.root = Path(tempfile.mkdtemp(prefix="candidate-release-http-"))
        self.repository = self.root / "repo"
        self.repository.mkdir()
        self.git("init", "-q")
        self.git("config", "user.name", "Release fixture")
        self.git("config", "user.email", "release-fixture@example.invalid")
        (self.repository / "file").write_text("fixture")
        self.git("add", "file")
        self.git("commit", "-q", "-m", "fixture")
        self.source = self.git("rev-parse", "HEAD")
        self.tag = "v2.0.0-rc.1"
        self.git("tag", "-a", self.tag, "-m", "fixture candidate")
        self.tag_object = self.git("rev-parse", f"refs/tags/{self.tag}")
        self.notes = self.root / "notes.md"
        self.notes.write_text("# fixture candidate\n")
        self.dist = self.root / "dist"
        self.dist.mkdir()
        for name in metadata.expected_asset_names("2.0.0-rc.1"):
            (self.dist / name).write_bytes(("real fixture bytes:" + name).encode())
        CandidateReleaseHandler.state = {
            "tag": self.tag, "source": self.source, "tag_object": self.tag_object,
            "release_id": 9001, "release": None, "events": [],
            "unrelated": {"id": 77, "name": self.tag, "tag_name": "v1.9.0"},
            "release_inventory": [{"id": 77, "name": self.tag, "tag_name": "v1.9.0", "draft": False}],
            "fail_upload": fail_upload, "fail_upload_after": 1, "fail_finalize": fail_finalize,
        }
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), CandidateReleaseHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.github = metadata.GitHub("fixture-token", f"http://127.0.0.1:{self.server.server_port}",
                                     f"http://127.0.0.1:{self.server.server_port}")
        test.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        test.addCleanup(self.close)

    def git(self, *args):
        return subprocess.check_output(["git", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false",
                                        "-C", str(self.repository), *args], text=True).strip()

    def args(self, output=None, assets=None):
        return argparse.Namespace(tag=self.tag, source_sha=self.source, notes=self.notes,
                                  repository=self.repository, output=output or self.root / "receipt.json",
                                  receipt=self.root / "receipt.json", dist=self.dist,
                                  local_assets=self.root / "local-assets.json",
                                  assets=assets or self.root / "assets.json")

    def snapshot_assets(self):
        metadata.write_new_json(self.root / "local-assets.json", metadata.asset_inventory(self.dist, "2.0.0-rc.1"))

    def reserve_upload(self):
        self.snapshot_assets()
        metadata.create_reservation(self.args(), self.github)
        metadata.upload_assets(self.args(), self.github)

    def close(self):
        self.server.shutdown()
        self.thread.join(timeout=5)
        self.server.server_close()


class RegistryStatusHandler(BaseHTTPRequestHandler):
    manifest_status = 404
    requests = []

    def log_message(self, format, *args):
        pass

    def do_GET(self):
        self.__class__.requests.append(("GET", self.path, self.headers.get("Authorization")))
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        body = json.dumps({"token": "synthetic-registry-fixture-token"}).encode()
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_HEAD(self):
        self.__class__.requests.append(("HEAD", self.path, self.headers.get("Authorization"),
                                        self.headers.get("Accept")))
        self.send_response(self.__class__.manifest_status)
        self.end_headers()


class CandidateReleaseTests(unittest.TestCase):
    def test_canonical_candidate_and_exact_commit(self):
        for tag in ("v2.0.0-rc.1", "v2.4.0-rc.12", "v0.7.0-rc.1"):
            with self.subTest(tag=tag):
                candidate.validate_identity(tag, SHA, SHA, SHA)

    def test_stable_tags_and_noncanonical_or_injected_input_fail(self):
        for tag in ("v2.0.0", "latest", "v02.0.0-rc.1", "v2.0.0-rc.0", "v2.0.0-rc.01",
                    "v2.0.0-rc.1+build", "../notes", "v2.0.0-rc.1\nother", "v2.0.0-rc.1;echo unsafe"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                candidate.validate_identity(tag, SHA, SHA, SHA)

    def test_retargeted_tags_and_mismatched_checkout_fail(self):
        for values in (("a" * 39, SHA, SHA), (SHA, "b" * 40, SHA),
                       (SHA, SHA, "b" * 40), ("A" * 40, SHA, SHA)):
            with self.subTest(values=values), self.assertRaises(ValueError):
                candidate.validate_identity("v2.0.0-rc.1", *values)

    def test_preexisting_environment_requires_reviewers(self):
        candidate.validate_environment({
            "name": "public-release-candidate",
            "protection_rules": [{"type": "required_reviewers", "reviewers": [{"type": "User", "reviewer": {"id": 1}}]}],
        })
        for environment in ({}, {"name": "public-release-candidate"},
                            {"name": "public-release-candidate", "protection_rules": [{"type": "required_reviewers", "reviewers": []}]},
                            {"name": "public-release-candidate", "protection_rules": [{"type": "wait_timer", "wait_timer": 5}]}):
            with self.subTest(environment=environment), self.assertRaises(ValueError):
                candidate.validate_environment(environment)

    def test_only_explicit_not_found_proves_image_absence(self):
        candidate.validate_absence_status(404)
        for status in (200, 401, 403, 429, 500, 503):
            with self.subTest(status=status), self.assertRaises(ValueError):
                candidate.validate_absence_status(status)

    def test_real_http_fixture_fails_closed_before_post_guard_work(self):
        RegistryStatusHandler.requests = []
        server = ThreadingHTTPServer(("127.0.0.1", 0), RegistryStatusHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        def stop_server():
            server.shutdown()
            thread.join(timeout=5)
            server.server_close()
        self.addCleanup(stop_server)
        origin = f"http://127.0.0.1:{server.server_port}"
        candidate_path = Path(__file__).with_name("candidate-release.py")
        probe = (
            "import importlib.util,sys; "
            "spec=importlib.util.spec_from_file_location('candidate',sys.argv[1]); "
            "module=importlib.util.module_from_spec(spec); spec.loader.exec_module(module); "
            "module._REGISTRY_ORIGIN=sys.argv[2]; "
            "module.assert_image_absent(sys.argv[3],sys.argv[4]); "
            "print('POST_ABSENCE_GUARD')"
        )

        for status in (404, 200, 401, 403, 500, 503):
            with self.subTest(status=status):
                RegistryStatusHandler.manifest_status = status
                result = subprocess.run(
                    [sys.executable, "-c", probe, str(candidate_path), origin,
                     "stellwerk-labs/octl", "protocol-fixture"],
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=10, check=False,
                )
                if status == 404:
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertIn("POST_ABSENCE_GUARD", result.stdout)
                else:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertNotIn("POST_ABSENCE_GUARD", result.stdout)
                    self.assertIn(f"HTTP {status}", result.stderr)

        self.assertEqual(len(RegistryStatusHandler.requests), 12)
        for index in range(0, len(RegistryStatusHandler.requests), 2):
            token_request, manifest_request = RegistryStatusHandler.requests[index:index + 2]
            self.assertEqual(token_request[0], "GET")
            self.assertTrue(token_request[1].startswith("/token?service=ghcr.io&scope=repository%3Astellwerk-labs%2Foctl%3Apull"))
            self.assertIsNone(token_request[2])
            self.assertEqual(manifest_request[0], "HEAD")
            self.assertEqual(manifest_request[1], "/v2/stellwerk-labs/octl/manifests/protocol-fixture")
            self.assertEqual(manifest_request[2], "Bearer synthetic-registry-fixture-token")
            self.assertIn("application/vnd.oci.image.index.v1+json", manifest_request[3])

    def test_registry_origin_is_fixed_to_ghcr_in_production(self):
        self.assertEqual(candidate._REGISTRY_ORIGIN, "https://ghcr.io")
        self.assertEqual(metadata.API_ORIGIN, "https://api.github.com")
        self.assertEqual(metadata.UPLOAD_ORIGIN, "https://uploads.github.com")

    def test_empty_or_other_release_page(self):
        candidate.validate_release_page("v2.0.0-rc.1", [])
        candidate.validate_release_page("v2.0.0-rc.1", [{"tag_name": "v1.2.1", "draft": False}])

    def test_drafts_and_published_releases_reserve_candidate_tag(self):
        for draft in (True, False):
            with self.subTest(draft=draft), self.assertRaises(ValueError):
                candidate.validate_release_page("v2.0.0-rc.1", [{"tag_name": "v2.0.0-rc.1", "draft": draft}])

    def test_malformed_or_unbounded_release_page_fails_closed(self):
        for page in ({"message": "not found"}, [None], [{}], [{"tag_name": "v1.2.1", "draft": "false"}],
                     [{"tag_name": "v1.2.1", "draft": False}] * 101):
            with self.subTest(page=page), self.assertRaises(ValueError):
                candidate.validate_release_page("v2.0.0-rc.1", page)

    def test_workflow_is_manual_and_candidate_never_receives_cross_repository_pat(self):
        text = Path(__file__).parents[1].joinpath(".github/workflows/ci.yaml").read_text()
        jobs = dict(re.findall(r"^  ([a-z-]+):\n(.*?)(?=^  [a-z-]+:\n|\Z)", text, re.M | re.S))
        publication = jobs["candidate-release"]
        for required in ("github.repository == 'stellwerk-labs/platform-orchestrator-cli'",
                         "github.event_name == 'workflow_dispatch'", "environment: public-release-candidate",
                         "- candidate-preflight", "- test",
                         "ref: ${{ inputs.candidate_sha }}", "--check-image-absent stellwerk-labs/octl",
                         "args: release --clean --skip=publish --release-notes=docs/releases/${{ inputs.release_tag }}.md",
                         "GORELEASER_CURRENT_TAG: ${{ inputs.release_tag }}"):
            self.assertIn(required, publication)
        for forbidden in ("secrets.GH_PAT", "semantic-release-action@", "git tag ", ":latest"):
            self.assertNotIn(forbidden, publication)
        self.assertIn("/environments/public-release-candidate", jobs["candidate-preflight"])
        self.assertIn("inputs.candidate_sha", jobs["test"])
        stable_condition = next(line for line in jobs["release"].splitlines() if line.strip().startswith("if:"))
        self.assertIn("!contains(inputs.release_tag, '-rc.')", stable_condition)
        self.assertIn("!contains(github.ref_name, '-rc.')", stable_condition)
        self.assertIn("secrets.GH_PAT", jobs["release"])
        goreleaser = publication.index("args: release --clean --skip=publish")
        release_capture = publication.index("candidate-release-metadata.py reserve", goreleaser)
        asset_upload = publication.index("candidate-release-metadata.py upload", release_capture)
        draft_check = publication.index("candidate-assets.json", asset_upload)
        image_publish = publication.index("scripts/publish-qualified-image.py dist", draft_check)
        native_smoke = publication.index("docker buildx imagetools inspect", image_publish)
        finalize = publication.index("candidate-release-metadata.py publish", native_smoke)
        self.assertLess(goreleaser, release_capture)
        self.assertLess(release_capture, asset_upload)
        self.assertLess(asset_upload, draft_check)
        self.assertLess(draft_check, image_publish)
        self.assertLess(image_publish, native_smoke)
        self.assertLess(native_smoke, finalize)
        self.assertIn("/releases/{release_id}", (Path(__file__).with_name("candidate-release-metadata.py")).read_text())
        stable = jobs["release"]
        self.assertNotIn("--draft --release-notes", stable)
        self.assertIn("args: release --clean --release-notes=docs/releases/${{ steps.version.outputs.tag }}.md", stable)
        original = subprocess.check_output(["git", "show", "HEAD:.github/workflows/ci.yaml"], text=True)
        original_jobs = dict(re.findall(r"^  ([a-z-]+):\n(.*?)(?=^  [a-z-]+:\n|\Z)", original, re.M | re.S))
        self.assertEqual(stable, original_jobs["release"])

    def test_cli_image_uses_version_without_v_prefix(self):
        text = Path(__file__).with_name("candidate-release.py").read_text()
        self.assertIn('assert_image_absent(args.check_image_absent, args.tag.removeprefix("v"))', text)

    def test_candidate_release_metadata_and_signed_assets(self):
        with tempfile.TemporaryDirectory() as temporary:
            dist = Path(temporary) / "dist"
            dist.mkdir()
            names = metadata.expected_asset_names("2.0.0-rc.1")
            self.assertEqual(len(names), 47)
            for name in names:
                (dist / name).write_bytes(("fixture:" + name).encode())
            expected = metadata.asset_inventory(dist, "2.0.0-rc.1")
            notes = "# candidate notes\n"
            reservation_release = self.release_metadata(notes, [])
            metadata.validate_release_record("v2.0.0-rc.1", reservation_release, notes, state="draft")
            self.assertEqual(reservation_release["assets"], [])
            draft = self.release_metadata(notes, expected, draft=True)
            metadata.validate_release_record("v2.0.0-rc.1", draft, notes, 9001, "draft", expected)
            published = {**draft, "draft": False}
            metadata.validate_release_record("v2.0.0-rc.1", published, notes, 9001, "published", expected)

            failures = (
                ({**draft, "id": 9002}, "draft"),
                ({**draft, "tag_name": "v2.0.0-rc.2"}, "draft"),
                ({**draft, "prerelease": False}, "draft"),
                ({**draft, "draft": False}, "draft"),
                ({**draft, "body": "changed notes"}, "draft"),
                ({**draft, "assets": draft["assets"][:-1]}, "draft"),
                ({**draft, "assets": [*draft["assets"][:-1],
                                       {**draft["assets"][-1], "digest": "sha256:" + "0" * 64}]}, "draft"),
                ({**draft, "assets": [*draft["assets"][:-1],
                                       {**draft["assets"][-1], "size": draft["assets"][-1]["size"] + 1}]}, "draft"),
                ({**draft, "assets": [*draft["assets"][:-1],
                                       {**draft["assets"][-1], "name": "replacement.bin"}]}, "draft"),
                ({**draft, "assets": [*draft["assets"], draft["assets"][0]]}, "draft"),
            )
            for release, state in failures:
                with self.subTest(release=release.get("id"), state=state), self.assertRaises(ValueError):
                    metadata.validate_release_record("v2.0.0-rc.1", release, notes,
                                                     9001, state, expected)

    def test_metadata_cli_captures_reservation_and_refuses_same_count_replacement(self):
        fixture = CandidateReleaseHTTPFixture(self)
        fixture.reserve_upload()
        receipt = json.loads((fixture.root / "receipt.json").read_text())
        self.assertEqual(receipt["releaseID"], 9001)
        self.assertEqual(receipt["sourceSHA"], fixture.source)
        self.assertEqual(receipt["tagObjectSHA"], fixture.tag_object)
        self.assertEqual(len(json.loads((fixture.root / "assets.json").read_text())), 47)
        self.assertEqual(CandidateReleaseHandler.state["unrelated"],
                         {"id": 77, "name": fixture.tag, "tag_name": "v1.9.0"})
        self.assertNotIn("patch", CandidateReleaseHandler.state["events"])

    def test_http_lifecycle_source_release_and_asset_drift_fail_closed(self):
        fixture = CandidateReleaseHTTPFixture(self)
        fixture.reserve_upload()
        state = CandidateReleaseHandler.state
        self.assertNotIn("target_commitish", state["create_request"])
        release = state["release"]
        notes = fixture.notes.read_text()
        expected = json.loads((fixture.root / "assets.json").read_text())
        for changed in (
            {**release, "id": 9002},
            {**release, "tag_name": "v2.0.0-rc.2"},
            {**release, "name": "unrelated title"},
            {**release, "body": "changed notes"},
            {**release, "assets": [*release["assets"][:-1],
                                   {**release["assets"][-1], "digest": "sha256:" + "0" * 64}]},
            {**release, "assets": [*release["assets"][:-1],
                                   {**release["assets"][-1], "size": release["assets"][-1]["size"] + 1}]},
            {**release, "assets": [*release["assets"][:-1],
                                   {**release["assets"][-1], "id": release["assets"][0]["id"]}]},
        ):
            with self.subTest(release=changed.get("id")):
                state["release"] = changed
                with self.assertRaises(ValueError):
                    metadata.validate_release_record(fixture.tag, state["release"], notes,
                                                     9001, "draft", expected)
        state["release"] = release

        state["release"] = {**release, "id": 9002}
        with self.assertRaisesRegex(ValueError, "identity differs"):
            metadata.validate_existing(fixture.args(), fixture.github)
        state["release"] = release

        state["source"] = "b" * 40
        before = len(state["events"])
        with self.assertRaisesRegex(ValueError, "tag moved"):
            metadata.validate_existing(fixture.args(), fixture.github)
        self.assertNotIn("patch", state["events"][before:])
        state["source"] = fixture.source
        state["tag_object"] = "c" * 40
        with self.assertRaisesRegex(ValueError, "tag moved"):
            metadata.validate_existing(fixture.args(), fixture.github)
        self.assertNotIn("patch", state["events"][before:])

    def test_http_success_publishes_only_after_exact_asset_validation(self):
        fixture = CandidateReleaseHTTPFixture(self)
        fixture.reserve_upload()
        state = CandidateReleaseHandler.state
        self.assertNotIn("patch", state["events"])
        metadata.validate_existing(fixture.args(), fixture.github, finalize=True)
        self.assertEqual(state["events"].count("patch"), 1)
        self.assertIs(state["release"]["draft"], False)
        self.assertIs(state["release"]["prerelease"], True)
        self.assertEqual(state["release"]["make_latest"], "false")

    def test_http_source_mismatch_prevents_draft_creation(self):
        fixture = CandidateReleaseHTTPFixture(self)
        CandidateReleaseHandler.state["source"] = "b" * 40
        with self.assertRaisesRegex(ValueError, "tag moved"):
            metadata.create_reservation(fixture.args(), fixture.github)
        self.assertNotIn("create", CandidateReleaseHandler.state["events"])
        self.assertEqual(CandidateReleaseHandler.state["unrelated"],
                         {"id": 77, "name": fixture.tag, "tag_name": "v1.9.0"})

    def test_http_existing_same_tag_release_prevents_creation(self):
        fixture = CandidateReleaseHTTPFixture(self)
        CandidateReleaseHandler.state["release_inventory"].append(
            {"id": 78, "name": "existing", "tag_name": fixture.tag, "draft": True})
        with self.assertRaisesRegex(ValueError, "already has"):
            metadata.create_reservation(fixture.args(), fixture.github)
        self.assertNotIn("create", CandidateReleaseHandler.state["events"])

    def test_http_invalid_create_response_keeps_direct_numeric_id_receipt(self):
        fixture = CandidateReleaseHTTPFixture(self)
        CandidateReleaseHandler.state["create_assets"] = [{"id": 55, "name": "foreign"}]
        with self.assertRaisesRegex(ValueError, "unexpectedly contains assets"):
            metadata.create_reservation(fixture.args(), fixture.github)
        receipt = json.loads((fixture.root / "receipt.json").read_text())
        self.assertEqual(receipt["releaseID"], 9001)

    def test_http_tag_ref_and_annotated_tag_schema_are_exact(self):
        fixture = CandidateReleaseHTTPFixture(self)
        state = CandidateReleaseHandler.state
        for key, value, pattern in (
            ("returned_ref", "refs/tags/v2.0.0-rc.2", "reference is invalid"),
            ("returned_tag_sha", "d" * 40, "annotated candidate tag"),
            ("returned_tag_name", "v2.0.0-rc.2", "annotated candidate tag"),
        ):
            state[key] = value
            with self.subTest(field=key), self.assertRaisesRegex(ValueError, pattern):
                fixture.github.source_ref(fixture.tag)
            del state[key]

    def test_http_redirects_are_rejected_without_following_or_retrying(self):
        source = ThreadingHTTPServer(("127.0.0.1", 0), RedirectHandler)
        target = ThreadingHTTPServer(("127.0.0.1", 0), RedirectTargetHandler)
        source.requests = []
        target.requests = []
        source.status = 302
        source.peer = f"http://127.0.0.1:{target.server_port}"
        source_thread = threading.Thread(target=source.serve_forever, daemon=True)
        target_thread = threading.Thread(target=target.serve_forever, daemon=True)
        source_thread.start()
        target_thread.start()
        self.addCleanup(source.server_close)
        self.addCleanup(target.server_close)
        self.addCleanup(source_thread.join, 5)
        self.addCleanup(target_thread.join, 5)
        self.addCleanup(source.shutdown)
        self.addCleanup(target.shutdown)
        github = metadata.GitHub("fixture-token", f"http://127.0.0.1:{source.server_port}",
                                f"http://127.0.0.1:{source.server_port}")
        for status in (301, 302, 303, 307, 308):
            source.status = status
            for method, upload, body in (("GET", False, None), ("POST", False, {"fixture": True}),
                                         ("POST", True, b"fixture upload bytes")):
                with self.subTest(status=status, method=method, upload=upload):
                    with self.assertRaisesRegex(ValueError, "request failed"):
                        github.request(method, "/redirect", body, upload=upload,
                                       content_type="application/octet-stream" if upload else "application/json")
        self.assertEqual(len(source.requests), 15)
        self.assertEqual(target.requests, [])
        self.assertEqual([item[0] for item in source.requests].count("GET"), 5)
        self.assertEqual([item[0] for item in source.requests].count("POST"), 10)
        self.assertTrue(all(item[2] == "Bearer fixture-token" for item in source.requests))

    def test_http_upload_failure_is_not_retried_or_published(self):
        fixture = CandidateReleaseHTTPFixture(self, fail_upload=True)
        fixture.snapshot_assets()
        metadata.create_reservation(fixture.args(), fixture.github)
        with self.assertRaisesRegex(ValueError, "request failed"):
            metadata.upload_assets(fixture.args(), fixture.github)
        state = CandidateReleaseHandler.state
        self.assertEqual(state["events"].count("upload"), 2)
        self.assertNotIn("patch", state["events"])
        state["fail_upload"] = False
        with self.assertRaisesRegex(ValueError, "already has assets"):
            metadata.upload_assets(fixture.args(), fixture.github)
        self.assertEqual(state["events"].count("upload"), 2)

    def test_http_finalization_is_id_bound_and_not_retried(self):
        fixture = CandidateReleaseHTTPFixture(self, fail_finalize=True)
        fixture.reserve_upload()
        state = CandidateReleaseHandler.state
        with self.assertRaisesRegex(ValueError, "request failed"):
            metadata.validate_existing(fixture.args(), fixture.github, finalize=True)
        self.assertEqual(state["events"].count("patch"), 1)
        with self.assertRaisesRegex(ValueError, "publication phase"):
            metadata.validate_existing(fixture.args(), fixture.github, finalize=True)
        self.assertEqual(state["events"].count("patch"), 1)

    @staticmethod
    def release_metadata(notes, expected, draft=True):
        assets = [{**item, "id": index + 1000, "state": "uploaded"}
                  for index, item in enumerate(expected)]
        return {"id": 9001, "tag_name": "v2.0.0-rc.1", "name": "v2.0.0-rc.1", "prerelease": True,
                "draft": draft, "body": notes, "assets": assets}


if __name__ == "__main__":
    unittest.main()
