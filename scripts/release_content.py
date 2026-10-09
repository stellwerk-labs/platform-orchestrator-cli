#!/usr/bin/env python3
"""Bounded inspection of actual client release content, shared runtime semantics."""
from collections import Counter
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import zipfile
import zlib


def decode_gzip_members(data, *, max_output=256 * 1024 * 1024,
                        max_metadata=1024 * 1024, max_members=1024):
    """Validate bounded RFC 1952 members and retain their non-payload bytes."""
    if min(max_output, max_metadata, max_members) < 1:
        raise ValueError("invalid gzip bounds")
    payloads, metadata, offset, output_size, metadata_size, members = [], [], 0, 0, 0, 0

    def retain(value):
        nonlocal metadata_size
        metadata_size += len(value)
        if metadata_size > max_metadata:
            raise ValueError("gzip metadata exceeds bound")
        metadata.append(value)

    while offset < len(data):
        padding_end = offset
        while padding_end < len(data) and data[padding_end] == 0:
            padding_end += 1
        if padding_end != offset:
            if not members:
                raise ValueError("gzip starts with padding")
            retain(data[offset:padding_end])
            offset = padding_end
        if offset == len(data):
            break
        members += 1
        if members > max_members:
            raise ValueError("gzip member count exceeds bound")
        if data[offset:offset + 3] != b"\x1f\x8b\x08" or offset + 10 > len(data):
            raise ValueError("invalid gzip member header or trailing bytes")
        flags, header_end = data[offset + 3], offset + 10
        if flags & 0xe0:
            raise ValueError("reserved gzip header flags")
        if flags & 4:
            if header_end + 2 > len(data):
                raise ValueError("truncated gzip extra length")
            size = int.from_bytes(data[header_end:header_end + 2], "little")
            header_end += 2 + size
        if header_end > len(data):
            raise ValueError("truncated gzip extra field")
        for flag in (8, 16):
            if flags & flag:
                end = data.find(b"\0", header_end,
                                min(len(data), offset + max_metadata - metadata_size + 1))
                if end < 0:
                    raise ValueError("unterminated or oversized gzip metadata")
                header_end = end + 1
        if flags & 2:
            header_end += 2
        if header_end > len(data):
            raise ValueError("truncated gzip header checksum")
        retain(data[offset:header_end])
        decoder = zlib.decompressobj(wbits=31)
        try:
            payload = decoder.decompress(memoryview(data)[offset:], max_output - output_size + 1)
        except zlib.error as error:
            raise ValueError("gzip member checksum or compression invalid") from error
        output_size += len(payload)
        if output_size > max_output:
            raise ValueError("gzip decompressed bytes exceed bound")
        if not decoder.eof or decoder.unconsumed_tail:
            raise ValueError("truncated or oversized gzip member")
        member_end = len(data) - len(decoder.unused_data)
        if member_end < header_end + 8:
            raise ValueError("invalid gzip member bounds")
        retain(data[member_end - 8:member_end])
        payloads.append(payload)
        offset = member_end
    if not members:
        raise ValueError("gzip has no members")
    return b"".join(payloads), b"".join(metadata)



BINARY_MODULE = "github.com/mattn/go-runewidth"
BINARY_VERSION = "v0.0.16"
BINARY_SUM = "h1:E5ScNMtiwvlvB5paMFdw9p4kSQzbXFikJ5SQO6TULQc="
BINARY_MATCH = "7e24f2ade193f05a114944db58399676f153d033dc62a375e7bcc4a9ef57445b"
BINARY_SECRET = "5f033b94b9ad6def05a2585fc320d32cbaeaa41e6966e7196686760fd140055c"
BINARY_CONTEXT = "3b763936a436c90c693a5f2f63a3893c53838b3bbfcd4b4791aa3a09059eff3e"
BINARY_POLICY = "go1.26.2-runewidth-v0.0.16-six-public-string-boundary-v1"


def executable_target(data):
    if len(data) >= 20 and data[:6] == b"\x7fELF\x02\x01":
        arch = {62: "amd64", 183: "arm64"}.get(int.from_bytes(data[18:20], "little"))
        return ("linux", arch) if arch else None
    if len(data) >= 32 and data[:4] == b"\xcf\xfa\xed\xfe":
        arch = {0x1000007: "amd64", 0x100000c: "arm64"}.get(int.from_bytes(data[4:8], "little"))
        return ("darwin", arch) if arch else None
    if len(data) >= 64 and data[:2] == b"MZ":
        offset = int.from_bytes(data[60:64], "little")
        if (offset <= len(data) - 24 and data[offset:offset + 4] == b"PE\0\0" and
                int.from_bytes(data[offset + 4:offset + 6], "little") == 0x8664):
            return ("windows", "amd64")
    return None


def known_binary_finding(data, finding, sums, buildinfo):
    """Only the source-proven six-string compiler boundary, never a credential."""
    target = executable_target(data)
    if not target or finding.get("RuleID") != "generic-api-key":
        return False
    if any(not isinstance(finding.get(key), str) for key in ("Match", "Secret")):
        return False
    match, secret = finding["Match"].encode(), finding["Secret"].encode()
    if (len(match) != 34 or hashlib.sha256(match).hexdigest() != BINARY_MATCH or len(secret) != 17
            or hashlib.sha256(secret).hexdigest() != BINARY_SECRET or secret not in match):
        return False
    identity = [BINARY_MODULE, BINARY_VERSION, BINARY_SUM]
    if identity not in [line.split() for line in sums.splitlines()]:
        return False
    settings = {setting["Key"]: setting["Value"] for setting in buildinfo.get("Settings", [])}
    if (buildinfo.get("GoVersion") != "go1.26.2"
            or buildinfo.get("Path") != "github.com/stellwerk-labs/platform-orchestrator-cli"
            or (settings.get("GOOS"), settings.get("GOARCH")) != target):
        return False
    dependencies = [dep for dep in buildinfo.get("Deps", []) if dep.get("Path") == BINARY_MODULE]
    if (len(dependencies) != 1 or dependencies[0].get("Replace")
            or [dependencies[0].get(key) for key in ("Path", "Version", "Sum")] != identity):
        return False
    occurrences = list(re.finditer(re.escape(match), data))
    for occurrence in occurrences:
        context = data[occurrence.start():occurrence.start() + 37]
        if len(context) != 37 or hashlib.sha256(context).hexdigest() != BINARY_CONTEXT:
            return False
    return bool(occurrences)


def binary_scan(directory, empty_cwd):
    """Retain all findings in memory, and require proof for each binary finding."""
    root, empty_cwd = Path(directory), Path(empty_cwd)
    source = Path(__file__).resolve().parents[1]
    sums = (source / "go.sum").read_text()
    seen, names, other_strings, accepted = set(), [], [], 0

    def inspect(strings):
        result = subprocess.run([
            "gitleaks", "stdin", "--no-banner", "--log-level", "error",
            "--ignore-gitleaks-allow", "--gitleaks-ignore-path", "/dev/null",
            "--config", str(source / "scripts/gitleaks.toml"), "--report-format", "json", "--report-path", "-",
        ], input=strings, stdout=subprocess.PIPE, stderr=subprocess.PIPE, cwd=empty_cwd, timeout=65, check=False)
        if result.returncode not in (0, 1) or result.stderr or not result.stdout:
            raise ValueError("binary scanner failed")
        findings = json.loads(result.stdout)
        if (not isinstance(findings, list) or (result.returncode == 0) != (len(findings) == 0)
                or any(not isinstance(finding, dict) or
                       any(not isinstance(finding.get(key), str) or not finding[key] for key in ("RuleID", "Match", "Secret"))
                       for finding in findings)):
            raise ValueError("binary scanner returned malformed evidence")
        return findings

    for path in root.rglob("*"):
        names.append(path.relative_to(root).as_posix().encode())
        if path.is_symlink():
            raise ValueError("symlinked binary scan content")
        if not path.is_file():
            continue
        data = path.read_bytes()
        if b"\0" not in data:
            continue
        digest = hashlib.sha256(data).digest()
        if digest in seen:
            continue
        seen.add(digest)
        strings = b"\n".join(re.findall(rb"[\x20-\x7e]{4,}", data)) + b"\n"
        if not executable_target(data):
            other_strings.append(strings)
            continue
        findings = inspect(strings)
        if not findings:
            continue
        environment = {**os.environ, "GOTOOLCHAIN": "go1.26.2", "GOWORK": "off", "GOENV": "off"}
        metadata = subprocess.run(["go", "version", "-m", "-json", str(path)], stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, env=environment, timeout=15, check=False)
        if metadata.returncode or metadata.stderr:
            raise ValueError("binary finding lacks verified Go metadata")
        buildinfo = json.loads(metadata.stdout)
        for finding in findings:
            if not known_binary_finding(data, finding, sums, buildinfo):
                raise ValueError("binary secret scan found unapproved content")
            accepted += 1
    if other_strings and inspect(b"\n".join(other_strings)):
        raise ValueError("binary secret scan found prohibited content")
    if inspect(b"\n".join(names)):
        raise ValueError("shipping path secret scan found prohibited content")
    if accepted:
        print(f"accepted {accepted} source-proven binary findings under {BINARY_POLICY}", file=sys.stderr)


def validated_checksum_values(path):
    if path.name != "notices.sha256.json":
        return set()
    try:
        manifest = json.loads(path.read_text())
        if not isinstance(manifest, dict) or not manifest:
            return set()
        actual = {}
        for source in path.parent.rglob("*"):
            if source.is_symlink() or not (source.is_file() or source.is_dir()):
                return set()
            if source.is_file() and source != path:
                actual[source.relative_to(path.parent).as_posix()] = hashlib.sha256(source.read_bytes()).hexdigest()
        if actual != manifest:
            return set()
        return set(actual.values())
    except (ValueError, OSError, TypeError):
        return set()


def public_fixture_record(path, source_root):
    policy = source_root / "scripts/known-public-fixtures.json"
    if not policy.is_file() or policy.is_symlink():
        return None
    for record in json.loads(policy.read_text()):
        suffix = record["path"].split("/")
        if tuple(path.parts[-len(suffix):]) != tuple(suffix):
            continue
        if hashlib.sha256(path.read_bytes()).hexdigest() != record["sha256"]:
            return None
        bundle = path.parents[len(suffix) - 2]
        graphs = json.loads((bundle / "modules.json").read_text())
        identities = [graph[record["module"]] for graph in graphs.values() if record["module"] in graph]
        sum_line = f'{record["module"]} {record["version"]} {record["sum"]}'
        if (not identities or set(identities) != {record["version"]}
                or sum_line not in (source_root / "go.sum").read_text().splitlines()):
            return None
        return record
    return None


def secret_scan(directory, empty_cwd):
    """Capture raw report fields only in memory; emit no match/path/secret values."""
    directory, empty_cwd = Path(directory).resolve(), Path(empty_cwd).resolve()
    source_root = Path(__file__).resolve().parents[1]
    result = subprocess.run([
        "gitleaks", "dir", "--no-banner", "--log-level", "error",
        "--ignore-gitleaks-allow", "--gitleaks-ignore-path", "/dev/null",
        "--config", str(source_root / "scripts/gitleaks.toml"),
        "--report-format", "json", "--report-path", "-", str(directory),
    ], cwd=empty_cwd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120, check=False)
    if result.returncode not in (0, 1) or not result.stdout:
        raise ValueError("secret scanner failed")
    findings = json.loads(result.stdout)
    if not isinstance(findings, list) or (result.returncode == 0 and findings) or (result.returncode == 1 and not findings):
        raise ValueError("secret scanner returned inconsistent evidence")
    grouped = {}
    for finding in findings:
        path = Path(finding["File"])
        path = path if path.is_absolute() else directory / path
        if not path.is_relative_to(directory) or path.is_symlink() or not path.is_file():
            raise ValueError("secret scanner returned an unsafe finding path")
        for key in ("RuleID", "Match", "Secret"):
            if not isinstance(finding[key], str) or not finding[key]:
                raise ValueError("secret scanner returned malformed evidence")
        grouped.setdefault(path, []).append(finding)
    accepted = 0
    for path, values in grouped.items():
        checksums = validated_checksum_values(path)
        remaining = []
        for finding in values:
            if (finding["RuleID"] == "generic-api-key" and re.fullmatch(r"[0-9a-f]{64}", finding["Secret"])
                    and finding["Secret"] in checksums and finding["Match"] in path.read_text()):
                accepted += 1
                continue
            remaining.append(finding)
        if not remaining:
            continue
        record = public_fixture_record(path, source_root)
        actual = Counter((finding["RuleID"], hashlib.sha256(finding["Match"].encode()).hexdigest(),
                          hashlib.sha256(finding["Secret"].encode()).hexdigest()) for finding in remaining)
        expected = Counter()
        if record:
            for finding in record["findings"]:
                if type(finding["count"]) is not int or finding["count"] <= 0:
                    raise ValueError("invalid public fixture finding count")
                expected[(finding["rule"], finding["match"], finding["secret"])] = finding["count"]
        if not record or actual != expected:
            raise ValueError("secret scan found unapproved or changed findings")
        accepted += len(remaining)
    if accepted:
        print(f"accepted {accepted} exact public-fixture/checksum findings under pinned proof policy", file=sys.stderr)


def scan(directory, forbidden=()):
    directory = Path(directory)
    if directory.is_symlink() or not directory.is_dir():
        raise ValueError("scan root must be an ordinary directory")
    common_patterns = [
        rb"/(?:Users|home)/[A-Za-z0-9_.-]+/",
        rb"github\.com/[^\s\"'<>]+-enterprise\b",
        rb"\bhttps?://[a-z0-9.-]+\.(?:internal|localdomain)\b",
    ] + [re.escape(value.encode()) for value in forbidden if value]
    text_patterns = [rb"\bWX[0-9]+\b", rb"\b(?:S1\.[0-9]+|PM[0-9]{2}|MO[0-9]{2}|[QCD][0-9]{2})\b"]
    for path in directory.rglob("*"):
        if path.name == ".gitleaksignore":
            raise ValueError("shipping content cannot supply scanner exemptions")
        if path.is_symlink():
            raise ValueError("refusing symlinked scan content")
        if not (path.is_file() or path.is_dir()):
            raise ValueError("refusing special scan content")
        if any(re.search(pattern, path.relative_to(directory).as_posix().encode())
               for pattern in common_patterns + text_patterns):
            raise ValueError("private reference scan found prohibited content")
        if path.is_file():
            data = path.read_bytes()
            # Three-character planning identifiers collide with ELF machine
            # instructions. Apply them to text; binary content still gets the
            # complete secret scan and all concrete path/owner/URL patterns.
            patterns = common_patterns + ([] if b"\0" in data else text_patterns)
            if any(re.search(pattern, data) for pattern in patterns):
                # Do not print the match or path: either can itself hold a secret.
                raise ValueError("private reference scan found prohibited content")


def binary_strings(directory):
    directory = Path(directory)
    for path in directory.rglob("*"):
        if path.is_symlink():
            raise ValueError("refusing symlinked scan content")
        # Filenames ship too, and can contain credentials independently of bytes.
        sys.stdout.buffer.write(path.relative_to(directory).as_posix().encode() + b"\n")
        if path.is_file():
            data = path.read_bytes()
            if b"\0" in data:
                for value in re.findall(rb"[\x20-\x7e]{4,}", data):
                    sys.stdout.buffer.write(value + b"\n")


def unpack(directory, output, metadata_output=None, max_members=20000, max_bytes=512 * 1024 * 1024, max_depth=3):
    """Expose archive members for scanning without trusting archive paths/types."""
    directory, output = Path(directory), Path(output)
    if not directory.is_dir() or directory.is_symlink() or output.exists():
        raise ValueError("archive scan input must exist and output must be new")
    remaining_members, remaining_bytes = max_members, max_bytes
    archive_limit = min(max_bytes, 256 * 1024 * 1024)
    output.mkdir()
    metadata_output = Path(metadata_output) if metadata_output is not None else None
    if metadata_output is not None:
        metadata_output.mkdir()
    metadata_count = 0

    def archive_path(path):
        return path.name.endswith((".tar.gz", ".tgz", ".tar", ".zip"))

    def extract(source, destination, depth):
        nonlocal remaining_members, remaining_bytes, metadata_count
        if depth > max_depth:
            raise ValueError("archive nesting exceeds scan limit")
        if source.stat().st_size > archive_limit:
            raise ValueError("archive source exceeds scan limit")
        destination.mkdir(parents=True)
        seen = set()

        def write(name, size, is_directory, stream):
            nonlocal remaining_members, remaining_bytes
            path = PurePosixPath(name.rstrip("/") if is_directory else name)
            if (not name or path.is_absolute() or ".." in path.parts or not path.parts
                    or str(path) != (name.rstrip("/") if is_directory else name)
                    or "\\" in name or any(ord(char) < 32 for char in name) or str(path) in seen):
                raise ValueError("unsafe or duplicate archive scan path")
            seen.add(str(path))
            remaining_members -= 1
            remaining_bytes -= size
            if remaining_members < 0 or remaining_bytes < 0 or size < 0:
                raise ValueError("archive content exceeds scan limit")
            target = destination / str(path)
            if is_directory:
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                data = stream.read(size + 1)
                if len(data) != size:
                    raise ValueError("archive scan member size mismatch")
                target.write_bytes(data)

        if source.name.endswith(".zip"):
            with zipfile.ZipFile(source) as archive:
                for entry in archive.infolist():
                    mode = entry.external_attr >> 16
                    if mode & 0o170000 not in (0, 0o100000, 0o040000) or entry.flag_bits & 1:
                        raise ValueError("unsupported archive scan entry")
                    with archive.open(entry) as stream:
                        write(entry.filename, entry.file_size, entry.is_dir(), stream)
        else:
            # Bound decompression before tarfile parses PAX/GNU headers too.
            # Header sizes are attacker-controlled, not just member sizes.
            compressed = source.read_bytes()
            wrapper_metadata = b""
            if source.name.endswith((".tar.gz", ".tgz")):
                raw, wrapper_metadata = decode_gzip_members(compressed, max_output=archive_limit)
            else:
                raw = compressed
            if len(raw) > archive_limit:
                raise ValueError("archive decompression exceeds scan limit")
            metadata_chunks, cursor = [wrapper_metadata], 0
            with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as archive:
                for entry in archive:
                    if not (entry.isfile() or entry.isdir()):
                        raise ValueError("unsupported archive scan entry")
                    if entry.isfile():
                        start, end = entry.offset_data, entry.offset_data + entry.size
                        if start < cursor or end > len(raw):
                            raise ValueError("invalid archive content offsets")
                        metadata_chunks.append(raw[cursor:start])
                        cursor = end
                    write(entry.name, entry.size, entry.isdir(), archive.extractfile(entry) if entry.isfile() else None)
            if metadata_output is not None:
                metadata_chunks.append(raw[cursor:])
                (metadata_output / str(metadata_count)).write_bytes(b"".join(metadata_chunks))
                metadata_count += 1
        for number, nested in enumerate(sorted(destination.rglob("*"))):
            if nested.is_file() and archive_path(nested):
                extract(nested, output / f"nested-{depth}-{remaining_members}-{number}", depth + 1)

    for number, source in enumerate(sorted(directory.rglob("*"))):
        if source.is_symlink():
            raise ValueError("refusing symlinked archive scan input")
        if source.is_file() and archive_path(source):
            extract(source, output / str(number), 1)



def main():
    command, *args = sys.argv[1:]
    if command == "scan":
        scan(args[0], args[1:])
    elif command == "strings":
        binary_strings(*args)
    elif command == "unpack":
        unpack(*args, max_members=100000, max_bytes=2 * 1024 * 1024 * 1024)
    elif command == "secret-scan":
        secret_scan(*args)
    elif command == "binary-scan":
        binary_scan(*args)
    else:
        raise ValueError("unknown release inspection command")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError, tarfile.TarError, zipfile.BadZipFile, EOFError, subprocess.SubprocessError):
        print("release content inspection failed (invalid, prohibited or unsafe content)", file=sys.stderr)
        sys.exit(1)
