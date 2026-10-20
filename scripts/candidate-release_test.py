#!/usr/bin/env python3
import importlib.util
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import re
import subprocess
import sys
import threading
import unittest


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


candidate = load("candidate", "candidate-release.py")
metadata = load("metadata", "candidate-release-metadata.py")
SHA = "a" * 40


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
                         "- candidate-preflight", "- test", "--verify-tag --prerelease --latest=false --draft",
                         "ref: ${{ inputs.candidate_sha }}", "--check-image-absent stellwerk-labs/octl",
                         "args: release --clean --release-notes=docs/releases/${{ inputs.release_tag }}.md",
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

    def test_cli_image_uses_version_without_v_prefix(self):
        text = Path(__file__).with_name("candidate-release.py").read_text()
        self.assertIn('assert_image_absent(args.check_image_absent, args.tag.removeprefix("v"))', text)

    def test_candidate_release_metadata_and_signed_assets(self):
        release = self.release_metadata()
        metadata.validate_metadata("v2.0.0-rc.1", release)
        for changes in ({"tagName": "v1.2.0"}, {"isPrerelease": False}, {"isDraft": True},
                        {"assets": release["assets"][:-1]}, {"assets": []}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                metadata.validate_metadata("v2.0.0-rc.1", {**release, **changes})

    @staticmethod
    def release_metadata():
        names = ["checksums.txt", "checksums.txt.sig", "checksums.txt.pem"]
        for platform in ("linux_amd64", "linux_arm64", "windows_amd64", "darwin_amd64", "darwin_arm64"):
            for suffix in ("", ".sig", ".pem"):
                names.append(platform + ".zip" + suffix)
        return {"tagName": "v2.0.0-rc.1", "isPrerelease": True, "isDraft": False,
                "assets": [{"name": name} for name in names]}


if __name__ == "__main__":
    unittest.main()
