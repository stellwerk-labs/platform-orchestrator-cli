#!/usr/bin/env python3
"""The first image copy and its readback, with the pinned ORAS against a real loopback registry.

The registry can lag like GHCR did for Data Plane v4.5.0-rc.3 (404 for the first read of a manifest it
has just accepted), lag the same way on the first read of a tag it has just set, or deny every push.
ORAS talks plain HTTP to localhost, so no TLS fixture is needed.
"""
import contextlib
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import io
import json
from pathlib import Path
import re
import shutil
import tempfile
import threading
import unittest


spec = importlib.util.spec_from_file_location("publisher", Path(__file__).with_name("publish-qualified-image.py"))
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)
UNKNOWN = b'{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}'
DENIED = b'{"errors":[{"code":"DENIED","message":"denied"}]}'


class RegistryHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_):
        pass

    def reply(self, code, headers=None, body=b""):
        self.send_response(code)
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def read_body(self):
        length = int(self.headers.get("Content-Length", "0"))
        return self.rfile.read(length) if length else b""

    def route(self):
        state, path = self.server.state, self.path.split("?")[0]
        if path == "/v2/":
            return self.reply(200)
        if state["mode"] == "deny" and self.command in ("POST", "PUT"):
            self.read_body()
            return self.reply(403, {"Content-Type": "application/json"}, DENIED)
        upload = re.fullmatch(r"/v2/(.+)/blobs/uploads/(.*)", path)
        if upload and self.command == "POST":
            self.read_body()
            return self.reply(202, {"Location": f"/v2/{upload[1]}/blobs/uploads/session"})
        if upload and self.command == "PUT":
            data, digest = self.read_body(), re.search(r"digest=([^&]+)", self.path)[1].replace("%3A", ":")
            state["blobs"][digest] = data
            return self.reply(201, {"Docker-Content-Digest": digest})
        match = re.fullmatch(r"/v2/(.+)/(blobs|manifests)/(.+)", path)
        if not match:
            return self.reply(404)
        kind, reference = match[2], match[3]
        if kind == "blobs":
            if reference not in state["blobs"]:
                return self.reply(404)
            return self.reply(200, {"Docker-Content-Digest": reference}, state["blobs"][reference])
        if self.command == "PUT":
            data = self.read_body()
            digest = "sha256:" + hashlib.sha256(data).hexdigest()
            state["manifests"][digest] = (self.headers["Content-Type"], data)
            if not reference.startswith("sha256:"):
                state["tags"][reference] = digest
                if state["mode"] == "tag-lag":
                    state["lagging_tags"].add(reference)
            if state["mode"] == "lag":
                state["lagging"].add(digest)
            return self.reply(201, {"Docker-Content-Digest": digest})
        if reference in state["lagging_tags"]:
            state["lagging_tags"].discard(reference)
            return self.reply(404, {"Content-Type": "application/json"}, UNKNOWN)
        digest = state["tags"].get(reference, reference)
        if digest not in state["manifests"] or (self.command == "GET" and digest in state["lagging"]):
            state["lagging"].discard(digest)
            return self.reply(404, {"Content-Type": "application/json"}, UNKNOWN)
        media, data = state["manifests"][digest]
        return self.reply(200, {"Docker-Content-Digest": digest, "Content-Type": media}, data)

    do_GET = do_HEAD = do_PUT = do_POST = route


def write_layout(root):
    """A minimal index -> manifest -> config + layer layout tagged v1; returns the index digest."""
    (root / "blobs/sha256").mkdir(parents=True)

    def add(data, media):
        digest = "sha256:" + hashlib.sha256(data).hexdigest()
        (root / "blobs/sha256" / digest[7:]).write_bytes(data)
        return {"mediaType": media, "digest": digest, "size": len(data)}

    def encode(value):
        return json.dumps(value, separators=(",", ":")).encode()

    config = add(b'{"architecture":"amd64","os":"linux"}', "application/vnd.oci.image.config.v1+json")
    layer = add(b"layer", "application/vnd.oci.image.layer.v1.tar+gzip")
    manifest = add(encode({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json",
                           "config": config, "layers": [layer]}), "application/vnd.oci.image.manifest.v1+json")
    manifest["platform"] = {"architecture": "amd64", "os": "linux"}
    index = add(encode({"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
                        "manifests": [manifest]}), "application/vnd.oci.image.index.v1+json")
    index["annotations"] = {"org.opencontainers.image.ref.name": "v1"}
    (root / "index.json").write_bytes(encode({"schemaVersion": 2, "manifests": [index]}))
    (root / "oci-layout").write_text('{"imageLayoutVersion":"1.0.0"}')
    return index["digest"]


class PublishIndexTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if shutil.which("oras") is None:
            raise RuntimeError("pinned ORAS 1.3.4 is required, run scripts/install-oras.sh")

    def setUp(self):
        self.directory = self.enterContext(tempfile.TemporaryDirectory())
        self.layout, self.root = self.directory + ":v1", write_layout(Path(self.directory))
        self.stderr = self.enterContext(contextlib.redirect_stderr(io.StringIO()))

    def registry(self, mode):
        server = ThreadingHTTPServer(("127.0.0.1", 0), RegistryHandler)
        server.state = {"mode": mode, "blobs": {}, "manifests": {}, "tags": {}, "lagging": set(), "lagging_tags": set()}
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(thread.join, 2)
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return server.state, f"localhost:{server.server_port}/stellwerk/octl:v1"

    def publish(self, reference, expected):
        """The copy and its readback, as main() runs them, without pauses."""
        publisher.copy_index(self.layout, reference, pause=0)
        publisher.read_back_index(reference, expected, pause=0)

    def test_copy_repeats_while_the_registry_lags_behind_its_push(self):
        registry, reference = self.registry("lag")
        self.publish(reference, self.root)
        self.assertEqual(registry["tags"], {"v1": self.root})
        self.assertIn("copy attempt 1 of 6, retrying", self.stderr.getvalue())

    def test_readback_waits_while_the_new_tag_lags(self):
        registry, reference = self.registry("tag-lag")
        self.publish(reference, self.root)
        self.assertEqual(registry["tags"], {"v1": self.root})
        self.assertEqual(registry["lagging_tags"], set(), "the readback never met the lagging tag")
        self.assertNotIn("retrying", self.stderr.getvalue())

    def test_denied_push_is_not_repeated(self):
        registry, reference = self.registry("deny")
        with self.assertRaises(publisher.release.CheckedToolFailure) as failure:
            self.publish(reference, self.root)
        self.assertEqual((failure.exception.tool, failure.exception.returncode), ("oras", 1))
        self.assertNotIn("retrying", self.stderr.getvalue())
        self.assertEqual(registry["tags"], {})

    def test_missing_layout_tag_is_not_mistaken_for_registry_lag(self):
        # ORAS also ends this deterministic source error with "not found"; only the registry form retries.
        registry, reference = self.registry("normal")
        with self.assertRaises(publisher.release.CheckedToolFailure) as failure:
            publisher.copy_index(self.directory + ":v2", reference, pause=0)
        self.assertEqual((failure.exception.tool, failure.exception.returncode), ("oras", 1))
        self.assertNotIn("retrying", self.stderr.getvalue())
        self.assertEqual(registry["tags"], {})

    def test_different_published_digest_fails_after_one_copy(self):
        registry, reference = self.registry("normal")
        with self.assertRaisesRegex(ValueError, "published index differs from qualified bytes"):
            self.publish(reference, "sha256:" + "0" * 64)
        self.assertEqual(registry["tags"], {"v1": self.root})
        self.assertNotIn("retrying", self.stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
