#!/usr/bin/env python3
"""Inspect all CLI OCI layers using the reviewed server/runtime rules."""
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import re
import tarfile
from release_content import decode_gzip_members


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def strict_json(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError("duplicate OCI JSON key")
            result[key] = value
        return result

    return json.loads(data, object_pairs_hook=unique)


def blob(layout, descriptor):
    value, size = descriptor.get("digest", ""), descriptor.get("size")
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", value) or type(size) is not int or not 0 <= size <= 512 * 1024 * 1024:
        raise ValueError("invalid OCI descriptor")
    path = layout / "blobs" / "sha256" / value[7:]
    if (layout / "blobs").is_symlink() or path.parent.is_symlink() or path.is_symlink() or not path.is_file():
        raise ValueError("missing or symlinked OCI blob")
    data = path.read_bytes()
    if len(data) != size or digest(data) != value:
        raise ValueError("OCI blob digest or size mismatch")
    return data


def oci_reference(directory, expected):
    layout = Path(directory)
    index_file = layout / "index.json"
    if index_file.is_symlink() or not index_file.is_file():
        raise ValueError("missing or symlinked OCI index")
    descriptors = strict_json(index_file.read_bytes()).get("manifests", [])
    if (len(descriptors) != 1 or descriptors[0].get("digest") != expected
            or descriptors[0].get("mediaType") != "application/vnd.oci.image.index.v1+json"):
        raise ValueError("OCI reference differs from inspected index")
    blob(layout, descriptors[0])
    tag = descriptors[0].get("annotations", {}).get("org.opencontainers.image.ref.name", "")
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", tag):
        raise ValueError("missing or invalid verified OCI tag")
    return tag


def validate_sbom_layout(directory, expected):
    layout = Path(directory)
    index_file = layout / "index.json"
    if index_file.is_symlink() or not index_file.is_file():
        raise ValueError("missing or symlinked SBOM OCI index")
    descriptors = strict_json(index_file.read_bytes()).get("manifests", [])
    if (len(descriptors) != 1 or descriptors[0].get("digest") != expected
            or descriptors[0].get("mediaType") != "application/vnd.oci.image.manifest.v1+json"):
        raise ValueError("SBOM source differs from inspected platform manifest")
    manifest = strict_json(blob(layout, descriptors[0]))
    for descriptor in [manifest["config"]] + manifest["layers"]:
        blob(layout, descriptor)


def safe_name(name):
    path = PurePosixPath(name)
    normalized = str(path)
    if path.is_absolute() or ".." in path.parts or not path.parts or "\\" in name or any(ord(character) < 32 for character in name):
        raise ValueError("unsafe OCI layer path")
    if ".gitleaksignore" in path.parts:
        raise ValueError("shipping scanner ignore files are forbidden")
    return normalized


def notices(files, arch, own_module):
    bundle = {name[9:]: data for name, data in files.items() if name.startswith("licenses/")}
    required = (own_module + "/LICENSE", "go/LICENSE", "go/PATENTS", "modules.json", "notices.sha256.json")
    for name in required:
        if not isinstance(bundle.get(name), bytes) or not bundle[name].strip():
            raise ValueError("missing required client license/graph notice")
    manifest = strict_json(bundle["notices.sha256.json"])
    actual = {name: hashlib.sha256(data).hexdigest() for name, data in bundle.items()
              if name != "notices.sha256.json" and isinstance(data, bytes)}
    if manifest != actual or len(bundle) != len(actual) + 1:
        raise ValueError("client notice manifest does not cover all regular image files")
    graphs = strict_json(bundle["modules.json"])
    if "linux/" + arch not in graphs:
        raise ValueError("client image has no matching dependency graph")
    for module in graphs["linux/" + arch]:
        if not any(name.startswith(module + "/") and re.match(r"(?i)(LICENSE|LICENCE|COPYING)", PurePosixPath(name).name)
                   for name in bundle):
            raise ValueError("image dependency has no shipped legal notice")
    return {name: digest(data) for name, data in sorted(bundle.items())}


def export(layout, output, architectures, own_module, binaries):
    layout, output = Path(layout), Path(output)
    if output.exists() or layout.is_symlink():
        raise ValueError("inspection output must be new")
    if (layout / "oci-layout").is_symlink() or (layout / "index.json").is_symlink():
        raise ValueError("symlinked OCI metadata")
    marker_bytes = (layout / "oci-layout").read_bytes()
    if strict_json(marker_bytes) != {"imageLayoutVersion": "1.0.0"}:
        raise ValueError("unsupported OCI layout")
    index_bytes = (layout / "index.json").read_bytes()
    index = strict_json(index_bytes)
    if len(index["manifests"]) != 1:
        raise ValueError("layout must contain one release index")
    top = index["manifests"][0]
    image_index_bytes = blob(layout, top)
    image_index = strict_json(image_index_bytes)
    manifests = image_index.get("manifests")
    if image_index.get("schemaVersion") != 2 or not isinstance(manifests, list):
        raise ValueError("release must be an OCI image index")
    payloads = {}
    attestations = []
    output.mkdir()
    metadata = output / "metadata"
    metadata.mkdir()
    # Scan the exact transport and subject bytes, not a lossy JSON projection.
    (metadata / "oci-layout.json").write_bytes(marker_bytes)
    (metadata / "layout-index.json").write_bytes(index_bytes)
    (metadata / "index.json").write_bytes(image_index_bytes)
    (metadata / "layout-index-decoded.json").write_text(json.dumps(index, sort_keys=True))
    (metadata / "index-decoded.json").write_text(json.dumps(image_index, sort_keys=True))
    for descriptor in manifests:
        platform = descriptor.get("platform", {})
        manifest_bytes = blob(layout, descriptor)
        manifest = strict_json(manifest_bytes)
        config_bytes = blob(layout, manifest["config"])
        config = strict_json(config_bytes)
        identity = descriptor["digest"][7:]
        (metadata / (identity + "-manifest.json")).write_bytes(manifest_bytes)
        (metadata / (manifest["config"]["digest"][7:] + "-config.json")).write_bytes(config_bytes)
        (metadata / (identity + ".json")).write_text(json.dumps({"descriptor": descriptor, "manifest": manifest, "config": config}, sort_keys=True))
        if platform.get("os") == "unknown" and platform.get("architecture") == "unknown":
            if descriptor.get("annotations", {}).get("vnd.docker.reference.type") != "attestation-manifest":
                raise ValueError("unknown non-attestation platform")
            reference = descriptor["annotations"].get("vnd.docker.reference.digest", "")
            predicates = set()
            for number, layer in enumerate(manifest["layers"]):
                data = blob(layout, layer)
                if layer["mediaType"] != "application/vnd.in-toto+json":
                    raise ValueError("unexpected attestation layer")
                statement = strict_json(data)
                if statement.get("_type") not in ("https://in-toto.io/Statement/v0.1", "https://in-toto.io/Statement/v1"):
                    raise ValueError("unsupported attestation statement")
                predicates.add(statement.get("predicateType"))
                if not any(subject.get("digest", {}).get("sha256") == reference.removeprefix("sha256:")
                           for subject in statement.get("subject", [])):
                    raise ValueError("attestation subject does not bind platform manifest")
                (metadata / (identity + "-" + str(number) + ".json")).write_bytes(data)
            if "https://spdx.dev/Document" not in predicates or not predicates.intersection(
                    {"https://slsa.dev/provenance/v0.2", "https://slsa.dev/provenance/v1"}):
                raise ValueError("platform lacks SPDX SBOM or SLSA provenance")
            attestations.append(descriptor["digest"])
            continue
        arch = platform.get("architecture")
        if platform.get("os") != "linux" or arch not in architectures or arch in payloads:
            raise ValueError("unexpected or duplicate platform")
        image_config = config.get("config", {})
        if (config.get("os"), config.get("architecture")) != ("linux", arch):
            raise ValueError("image config architecture mismatch")
        if image_config.get("Entrypoint") != ["/usr/local/bin/octl"] or image_config.get("User") not in ("65532", "65532:65532", "nonroot"):
            raise ValueError("unexpected client entrypoint or user")
        files, modes, diff_ids, layers = {}, {}, [], []
        for number, layer in enumerate(manifest["layers"]):
            data = blob(layout, layer)
            media = layer["mediaType"]
            if media in ("application/vnd.oci.image.layer.v1.tar+gzip", "application/vnd.docker.image.rootfs.diff.tar.gzip"):
                raw, wrapper_metadata = decode_gzip_members(data)
            elif media == "application/vnd.oci.image.layer.v1.tar":
                raw = data
                wrapper_metadata = b""
                if len(raw) > 256 * 1024 * 1024:
                    raise ValueError("OCI layer exceeds inspection bound")
            else:
                raise ValueError("unsupported image layer compression")
            diff_ids.append(digest(raw))
            layers.append(layer["digest"])
            raw_root = output / "layers" / arch / str(number)
            raw_root.mkdir(parents=True)
            entries, seen, metadata_chunks, cursor = [], set(), [wrapper_metadata], 0
            with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as archive:
                for entry_number, entry in enumerate(archive):
                    if entry_number >= 100000 or not 0 <= entry.size <= 256 * 1024 * 1024:
                        raise ValueError("OCI layer member exceeds inspection bound")
                    if entry.isdir() and entry.name in (".", "./"):
                        continue
                    name = safe_name(entry.name)
                    if name in seen:
                        raise ValueError("duplicate layer entry")
                    seen.add(name)
                    record = {"name": name, "mode": entry.mode, "type": entry.type.decode("ascii"), "link": entry.linkname}
                    entries.append(record)
                    if entry.isfile():
                        start, end = entry.offset_data, entry.offset_data + entry.size
                        if start < cursor or end > len(raw):
                            raise ValueError("invalid layer content offsets")
                        # Preserve every header/PAX/padding byte without mixing
                        # executable bodies into a generic binary exception.
                        metadata_chunks.append(raw[cursor:start])
                        cursor = end
                    path = PurePosixPath(name)
                    if path.name.startswith(".wh."):
                        if not entry.isfile() or entry.size:
                            raise ValueError("invalid whiteout")
                        if path.name == ".wh..wh..opq":
                            removed = "" if str(path.parent) == "." else str(path.parent) + "/"
                            files = {key: value for key, value in files.items() if not key.startswith(removed)}
                        else:
                            removed = str(path.with_name(path.name[4:]))
                            files = {key: value for key, value in files.items() if key != removed and not key.startswith(removed + "/")}
                    elif entry.isfile():
                        content = archive.extractfile(entry).read()
                        if len(content) != entry.size:
                            raise ValueError("layer member size mismatch")
                        # Retain original bytes, including files hidden by later whiteouts.
                        (raw_root / str(entry_number)).write_bytes(content)
                        files[name] = content
                        modes[name] = entry.mode
                    elif entry.isdir():
                        files.pop(name, None)
                        continue
                    elif entry.issym() or entry.islnk():
                        # Never create attacker-controlled links on the host. Their
                        # names/targets remain in scanned metadata; required payloads
                        # below must resolve to actual regular files, not links.
                        files[name] = None
                    else:
                        raise ValueError("unsupported non-file OCI entry")
            (raw_root / "entries.json").write_text(json.dumps(entries, sort_keys=True))
            metadata_chunks.append(raw[cursor:])
            (raw_root / "metadata-bytes.bin").write_bytes(b"".join(metadata_chunks))
        if config.get("rootfs") != {"type": "layers", "diff_ids": diff_ids}:
            raise ValueError("layer diff ID mismatch")
        binary_hashes = {}
        destination = output / arch
        for name in list(binaries) + [name for name in files if name.startswith("licenses/")]:
            if any(str(parent) in files for parent in PurePosixPath(name).parents):
                raise ValueError("required payload has a non-directory ancestor")
        for name in binaries:
            content = files.get(name)
            machine = 62 if arch == "amd64" else 183
            if not isinstance(content, bytes) or not modes.get(name, 0) & 0o111 or len(content) < 20 or content[:6] != b"\x7fELF\x02\x01" or int.from_bytes(content[18:20], "little") != machine:
                raise ValueError("missing or wrong-architecture client executable")
            target = destination / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(content)
            binary_hashes[name] = digest(content)
        license_hashes = notices(files, arch, own_module)
        for name, content in files.items():
            if name.startswith("licenses/") and isinstance(content, bytes):
                target = destination / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(content)
        payloads[arch] = {"manifest": descriptor["digest"], "config": manifest["config"]["digest"],
                          "layers": layers, "diffIds": diff_ids, "binaries": binary_hashes, "licenses": license_hashes}
    if set(payloads) != set(architectures):
        raise ValueError("missing required image platform")
    # SBOM/provenance stay attached. Presence is checked here; their predicates
    # are scanned but are not claimed cryptographically verified by this helper.
    subjects = {d.get("annotations", {}).get("vnd.docker.reference.digest") for d in manifests
                if d.get("annotations", {}).get("vnd.docker.reference.type") == "attestation-manifest"}
    if subjects != {value["manifest"] for value in payloads.values()}:
        raise ValueError("missing platform BuildKit attestations")
    report = {"schemaVersion": 1, "index": top["digest"], "payloads": payloads, "attestations": sorted(attestations),
              "reproducibilityBoundary": "Compare platform manifests/configs/layers/binaries/notices. Attestations and full index may vary; they are preserved, not removed."}
    (output / "report.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    return report
