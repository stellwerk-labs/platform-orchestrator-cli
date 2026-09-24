#!/usr/bin/env python3
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("stable", Path(__file__).with_name("stable-release-preflight.py"))
stable = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stable)
SHA = "a" * 40


class StableReleaseTests(unittest.TestCase):
    def test_exact_canonical_stable_source(self):
        stable.validate_stable_identity("v2.0.0", SHA, SHA, SHA)
        for tag in ("v02.0.0", "v2.0.0-rc.1", "latest", "v2.0.0+build", "../notes"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                stable.validate_stable_identity(tag, SHA, SHA, SHA)
        for values in ((SHA, "b" * 40, SHA), (SHA, SHA, "b" * 40), ("abc", "abc", "abc")):
            with self.subTest(values=values), self.assertRaises(ValueError):
                stable.validate_stable_identity("v2.0.0", *values)

    def test_semantic_release_only_assigns_versions(self):
        config = json.loads(Path(__file__).parents[1].joinpath(".releaserc.json").read_text())
        self.assertEqual([plugin[0] for plugin in config["plugins"]], ["@semantic-release/commit-analyzer"])
        rules = config["plugins"][0][1]["releaseRules"]
        self.assertIn({"breaking": True, "release": "major"}, rules)
        self.assertIn({"type": "feat", "release": "minor"}, rules)

    def test_nonempty_token_alone_does_not_prove_channel_access(self):
        repo = "stellwerk-labs/homebrew-tap"
        stable.validate_channel_access(repo, {"full_name": repo, "archived": False, "permissions": {"push": True}})
        for metadata in ({}, {"full_name": repo, "archived": False, "permissions": {"push": False}},
                         {"full_name": repo, "archived": True, "permissions": {"push": True}},
                         {"full_name": "another/repo", "archived": False, "permissions": {"push": True}}):
            with self.subTest(metadata=metadata), self.assertRaises(ValueError):
                stable.validate_channel_access(repo, metadata)

    def test_drafts_and_completed_releases_block_fresh_publication(self):
        for draft in (True, False):
            with self.subTest(draft=draft), self.assertRaises(ValueError):
                stable.candidate.validate_release_page("v2.0.0", [{"tag_name": "v2.0.0", "draft": draft}])


if __name__ == "__main__":
    unittest.main()
