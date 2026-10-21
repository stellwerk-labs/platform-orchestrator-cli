#!/usr/bin/env python3
"""Generate SPDX/producer evidence from exact GoReleaser archive bytes."""
import ast
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

import release_content as content


ROOT = Path(__file__).resolve().parents[1]
SYFT = "ghcr.io/anchore/syft@sha256:3eb5379ba7b409c3f4069b686110527af0c47df993fa5c10d13e7cf34f49b1aa"
PROVENANCE = "https://slsa.dev/provenance/v1"


QUALIFICATION_ERRORS = (ValueError, OSError, KeyError, TypeError, subprocess.SubprocessError)
RULE_SOURCES = ("qualify_release.py", "client_image.py", "client_oci.py", "release_content.py")
TOOLS = {"git", "go", "docker", "oras", "content-scan", "other"}


class CheckedToolFailure(ValueError):
    def __init__(self, tool, returncode):
        super().__init__("release inspection tool failed")
        self.tool, self.returncode = tool, returncode


class QualificationStageFailure(ValueError):
    def __init__(self, stage, category="unclassified", rule_id=None, tool=None, returncode=None):
        if not re.fullmatch(r"(?:entrypoint|source-identity|module-identity|source-notices|notice-manifest|qualify|recheck|release-notes|receipt-write|shipping-scan|archive-(?:set|inspection|target|spdx|provenance|target-set)|oci-image)(?::[0-9]{1,5}/[0-9]{1,5})?", stage):
            raise ValueError("invalid diagnostic stage")
        if category not in {"unclassified", "checked-tool", "source-literal", "ValueError", "OSError", "KeyError", "TypeError", "SubprocessError"}:
            raise ValueError("invalid diagnostic category")
        if rule_id is not None and not re.fullmatch(r"(?:qualify_release|client_image|client_oci|release_content)\.py:sha256:[0-9a-f]{64}", rule_id):
            raise ValueError("invalid diagnostic rule")
        if tool is not None and tool not in TOOLS:
            raise ValueError("invalid diagnostic tool")
        if returncode is not None and (type(returncode) is not int or not -4294967295 <= returncode <= 4294967295):
            raise ValueError("invalid diagnostic exit")
        super().__init__(stage)
        self.category, self.rule_id, self.tool, self.returncode = category, rule_id, tool, returncode


def literal_rule(error):
    if type(error) is not ValueError or len(error.args) != 1 or type(error.args[0]) is not str:
        return None
    traceback = error.__traceback__
    while traceback and traceback.tb_next:
        traceback = traceback.tb_next
    if traceback is None:
        return None
    source = Path(traceback.tb_frame.f_code.co_filename).resolve()
    if source.parent != ROOT / "scripts" or source.name not in RULE_SOURCES:
        return None
    try:
        tree = ast.parse(source.read_bytes(), filename=str(source))
    except (OSError, SyntaxError):
        return None
    for node in ast.walk(tree):
        if (isinstance(node, ast.Raise) and node.lineno == traceback.tb_lineno
                and isinstance(node.exc, ast.Call) and isinstance(node.exc.func, ast.Name)
                and node.exc.func.id == "ValueError" and len(node.exc.args) == 1
                and not node.exc.keywords and isinstance(node.exc.args[0], ast.Constant)
                and type(node.exc.args[0].value) is str and node.exc.args[0].value == error.args[0]):
            return source.name + ":sha256:" + hashlib.sha256(error.args[0].encode()).hexdigest()
    return None


def closed_failure(error):
    traceback = error.__traceback__
    while traceback and traceback.tb_next:
        traceback = traceback.tb_next
    # Source-bound recognition survives __main__ importing client_image, which
    # loads qualify_release again with a distinct exception class identity.
    if (error.args == ("release inspection tool failed",) and traceback is not None
            and traceback.tb_frame.f_code.co_name == "checked"
            and Path(traceback.tb_frame.f_code.co_filename).resolve() == ROOT / "scripts/qualify_release.py"):
        code, tool = getattr(error, "returncode", None), getattr(error, "tool", None)
        if type(code) is int and -4294967295 <= code <= 4294967295 and tool in TOOLS:
            return "checked-tool", None, tool, code
    rule = literal_rule(error)
    if rule:
        return "source-literal", rule, None, None
    for kind, label in ((ValueError, "ValueError"), (OSError, "OSError"), (KeyError, "KeyError"),
                        (TypeError, "TypeError"), (subprocess.SubprocessError, "SubprocessError")):
        if isinstance(error, kind):
            return label, None, None, None
    return "unclassified", None, None, None


def at_stage(stage, action, *args):
    try:
        return action(*args)
    except QualificationStageFailure:
        raise
    except QUALIFICATION_ERRORS as error:
        raise QualificationStageFailure(stage, *closed_failure(error)) from None


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def write_json(path, value):
    with Path(path).open("x") as output:
        output.write(json.dumps(value, sort_keys=True, indent=2) + "\n")


def checked(args, **options):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False, **options)
    if result.returncode:
        tool = {"git": "git", "go": "go", "docker": "docker", "oras": "oras",
                "scan-release-content.sh": "content-scan"}.get(Path(args[0]).name, "other")
        raise CheckedToolFailure(tool, result.returncode)
    return result.stdout


def module_path():
    match = re.search(r"(?m)^module ([^\s]+)$", (ROOT / "go.mod").read_text())
    if not match:
        raise ValueError("application module identity missing")
    return match[1]


def notices(directory, own):
    directory = Path(directory)
    manifest = json.loads((directory / "notices.sha256.json").read_text())
    actual = {}
    for path in directory.rglob("*"):
        if path.is_symlink() or not (path.is_file() or path.is_dir()):
            raise ValueError("unsafe notice content")
        if path.is_file() and path != directory / "notices.sha256.json":
            actual[path.relative_to(directory).as_posix()] = sha(path)
    if not manifest or actual != manifest:
        raise ValueError("notice checksum manifest does not cover exact shipped content")
    for name in (own + "/LICENSE", "go/LICENSE", "go/PATENTS", "modules.json"):
        path = directory / name
        if not path.is_file() or not path.read_bytes().strip():
            raise ValueError("missing required legal/graph notice")
    graphs = json.loads((directory / "modules.json").read_text())
    if not isinstance(graphs, dict) or not graphs:
        raise ValueError("missing target dependency graphs")
    for graph in graphs.values():
        if not isinstance(graph, dict):
            raise ValueError("invalid target dependency graph")
        for module in graph:
            if not any(name.startswith(module + "/") and re.match(r"(?i)(LICENSE|LICENCE|COPYING)", Path(name).name)
                       for name in actual):
                raise ValueError("compiled dependency is missing its shipped license")
    return graphs


def inspect_archive(archive, output, own, expected_notices=None):
    archive, output = Path(archive), Path(output)
    if archive.is_symlink() or not archive.is_file() or not re.fullmatch(r"[A-Za-z0-9_.+-]+", archive.name):
        raise ValueError("unsafe release archive")
    with tempfile.TemporaryDirectory(prefix="client-one-archive-") as temporary:
        source = Path(temporary) / "source"
        source.mkdir()
        shutil.copyfile(archive, source / archive.name)
        content.unpack(source, output, max_members=20000, max_bytes=256 * 1024 * 1024)
    roots = sorted(output.iterdir())
    if len(roots) != 1 or not roots[0].is_dir():
        raise ValueError("unexpected nested release archive")
    filesystem = roots[0]
    graphs = notices(filesystem / "licenses", own)
    notice_manifest = json.loads((filesystem / "licenses/notices.sha256.json").read_text())
    if expected_notices is not None and notice_manifest != expected_notices:
        raise ValueError("archive legal bytes differ from the source-collected notice union")
    if (filesystem / "LICENSE").read_bytes() != (filesystem / "licenses" / own / "LICENSE").read_bytes():
        raise ValueError("archive own license differs from collected own license")
    binaries = [path for path in filesystem.iterdir() if path.is_file() and path.name not in ("LICENSE", "README.md")]
    if len(binaries) != 1:
        raise ValueError("archive must contain exactly one executable")
    info = json.loads(checked(["go", "version", "-m", "-json", str(binaries[0])]))
    settings = {item["Key"]: item["Value"] for item in info.get("Settings", [])}
    target = settings.get("GOOS", "") + "/" + settings.get("GOARCH", "")
    if (info.get("GoVersion") != "go1.26.2" or info.get("Path") != own or target not in graphs
            or settings.get("-trimpath") != "true" or settings.get("CGO_ENABLED") != "0"):
        raise ValueError("archive binary has wrong source/toolchain/target/flags")
    graph = graphs[target]
    dependencies = {}
    for dependency in info.get("Deps", []):
        if dependency.get("Replace"):
            raise ValueError("release binary has a replaced dependency")
        dependencies[dependency["Path"]] = dependency["Version"]
    if dependencies != graph:
        raise ValueError("compiled binary dependencies differ from collected target graph")
    return {"target": target, "filesystem": filesystem, "binary": binaries[0],
            "binarySHA256": sha(binaries[0]), "dependencies": dependencies, "graphs": graphs}


def identity():
    source = checked(["git", "-C", str(ROOT), "rev-parse", "HEAD"]).decode().strip()
    if source != os.environ.get("QUALIFY_SOURCE_SHA") or not re.fullmatch(r"[0-9a-f]{40}", source):
        raise ValueError("qualification source does not match exact GoReleaser commit")
    checked(["git", "-C", str(ROOT), "diff", "--quiet", "HEAD"])
    # Only the known generated output trees may be absent from the source
    # commit. Ignored Go files are still build inputs and must not slip through.
    for options in (["--exclude-standard"], ["--ignored", "--exclude-standard"]):
        paths = checked(["git", "-C", str(ROOT), "ls-files", "--others", "-z", *options]).decode().split("\0")
        if any(path and not path.startswith(("dist/", "licenses/", "scripts/__pycache__/")) for path in paths):
            raise ValueError("uncommitted or ignored files could alter release build inputs")
    raw_date = checked(["git", "-C", str(ROOT), "show", "-s", "--format=%cI", source]).decode().strip()
    created = datetime.datetime.fromisoformat(raw_date).astimezone(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    repository = os.environ.get("QUALIFY_REPOSITORY", os.environ.get("GITHUB_REPOSITORY", ""))
    workflow = os.environ.get("QUALIFY_WORKFLOW_REF", os.environ.get("GITHUB_WORKFLOW_REF", ""))
    workflow_sha = os.environ.get("QUALIFY_WORKFLOW_SHA", os.environ.get("GITHUB_WORKFLOW_SHA", ""))
    run = os.environ.get("QUALIFY_RUN_ID", os.environ.get("GITHUB_RUN_ID", ""))
    attempt = os.environ.get("QUALIFY_RUN_ATTEMPT", os.environ.get("GITHUB_RUN_ATTEMPT", ""))
    if (not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository)
            or not re.fullmatch(re.escape(repository) + r"/\.github/workflows/[A-Za-z0-9_.-]+@[A-Za-z0-9_./-]+", workflow)
            or not re.fullmatch(r"[0-9a-f]{40}", workflow_sha) or not run.isdecimal() or not attempt.isdecimal()):
        raise ValueError("exact GitHub producer/workflow/run identity is required")
    if (not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?(?:\+[A-Za-z0-9.-]+)?", os.environ.get("QUALIFY_VERSION", ""))
            or os.environ.get("QUALIFY_TAG") != "v" + os.environ["QUALIFY_VERSION"]):
        raise ValueError("exact release tag/version identity required")
    tagged_source = checked(["git", "-C", str(ROOT), "rev-parse", "--verify",
                             "refs/tags/" + os.environ["QUALIFY_TAG"] + "^{commit}"]).decode().strip()
    if tagged_source != source:
        raise ValueError("pre-existing release tag does not bind the qualified source commit")
    return {"sourceSHA": source, "created": created, "repository": repository, "workflowRef": workflow,
            "workflowSHA": workflow_sha, "runID": run, "runAttempt": attempt,
            "version": os.environ["QUALIFY_VERSION"], "tag": os.environ["QUALIFY_TAG"]}


def generate_spdx(filesystem, output, archive_hash, producer, source_kind="dir", dependencies=None):
    output = Path(output)
    if source_kind not in ("dir", "oci-dir"):
        raise ValueError("unsupported SPDX inspection source")
    repository_digests = json.loads(checked(["docker", "image", "inspect", SYFT, "--format", "{{json .RepoDigests}}"]))
    if SYFT not in repository_digests:
        raise ValueError("pinned Syft image must be explicitly pulled before qualification")
    checked(["docker", "run", "--rm", "--pull=never", "--network", "none", "--read-only",
             "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", f"{os.getuid()}:{os.getgid()}",
             "--tmpfs", "/tmp:rw,noexec,nosuid,size=128m,mode=1777", "--env", "HOME=/tmp",
             "--env", "XDG_CACHE_HOME=/tmp/.cache", "--env", "SYFT_CHECK_FOR_APP_UPDATE=false",
             "--mount", f"type=bind,src={Path(filesystem).resolve()},dst=/input,readonly",
             "--mount", f"type=bind,src={output.parent.resolve()},dst=/output",
             SYFT, "scan", source_kind + ":/input", "--select-catalogers", "+go-module-binary-cataloger",
             "--source-name", output.name.removesuffix(".spdx.json"),
             "-o", "spdx-json=/output/" + output.name])
    document = json.loads(output.read_text())
    if (document.get("spdxVersion") != "SPDX-2.3" or document.get("SPDXID") != "SPDXRef-DOCUMENT"
            or not any(package.get("name") == "stdlib" and package.get("versionInfo") == "go1.26.2"
                       for package in document.get("packages", []))):
        raise ValueError("SPDX document does not describe the inspected Go binary")
    packages = {(package.get("name"), package.get("versionInfo")) for package in document.get("packages", [])}
    if dependencies is not None and not set(dependencies.items()).issubset(packages):
        raise ValueError("SPDX catalog is missing compiled dependency identities")
    document["creationInfo"]["created"] = producer["created"]
    document["documentNamespace"] = "https://stellwerk.dev/sbom/client/" + archive_hash
    output.write_text(json.dumps(document, sort_keys=True, indent=2) + "\n")
    return document


def producer_statement(archive, spdx, inspection, producer):
    return {
        "_type": "https://in-toto.io/Statement/v1",
        "subject": [{"name": archive.name, "digest": {"sha256": sha(archive)}},
                    {"name": spdx.name, "digest": {"sha256": sha(spdx)}}],
        "predicateType": PROVENANCE,
        "predicate": {
            "buildDefinition": {
                "buildType": "https://stellwerk.dev/build/goreleaser-client/v1",
                "externalParameters": {"version": producer["version"], "tag": producer["tag"],
                                       "repository": producer["repository"], "workflowRef": producer["workflowRef"]},
                "resolvedDependencies": [
                    {"uri": "git+https://github.com/" + producer["repository"],
                     "digest": {"gitCommit": producer["sourceSHA"]}},
                    {"uri": "git+https://github.com/" + producer["repository"],
                     "name": producer["workflowRef"].split("@")[0].removeprefix(producer["repository"] + "/"),
                     "digest": {"gitCommit": producer["workflowSHA"]}},
                    {"uri": "oci://" + SYFT, "digest": {"sha256": SYFT.rsplit(":", 1)[1]}},
                ],
                "internalParameters": {"target": inspection["target"], "goVersion": "go1.26.2",
                                       "goreleaserVersion": "2.18.1", "goLicensesVersion": "v2.0.1",
                                       "gitleaksVersion": "8.30.1", "binarySHA256": inspection["binarySHA256"],
                                       "compiledDependencies": inspection["dependencies"]},
            },
            "runDetails": {
                "builder": {"id": "https://github.com/" + producer["workflowRef"]},
                "metadata": {"invocationId": "https://github.com/" + producer["repository"] + "/actions/runs/" +
                             producer["runID"] + "/attempts/" + producer["runAttempt"]},
            },
        },
    }


def archive_paths(dist):
    return sorted(path for path in Path(dist).iterdir() if path.name.endswith((".tar.gz", ".zip")))


def qualify(dist):
    dist = Path(dist).resolve()
    producer = at_stage("source-identity", identity)
    stage = dist / "qualified-content"
    stage.mkdir()
    shipping = stage / "shipping"
    shipping.mkdir()
    extracts = stage / "extracted"
    extracts.mkdir()
    own = at_stage("module-identity", module_path)
    expected = at_stage("source-notices", notices, ROOT / "licenses", own)
    expected_notices = at_stage("notice-manifest", lambda: json.loads((ROOT / "licenses/notices.sha256.json").read_text()))
    archives = archive_paths(dist)
    if len(archives) != len(expected):
        raise ValueError("archive set does not cover every configured release target")
    payloads, targets, inspections = {}, set(), {}
    for number, archive in enumerate(archives):
        progress = f"{number + 1}/{len(archives)}"
        inspection = at_stage("archive-inspection:" + progress, inspect_archive, archive, extracts / str(number), own, expected_notices)
        if inspection["target"] in targets or inspection["graphs"] != expected:
            raise ValueError("duplicate target or archive contains a different notice graph")
        targets.add(inspection["target"])
        inspections[inspection["target"]] = inspection
        spdx = dist / (archive.name + ".spdx.json")
        at_stage("archive-spdx:" + progress, lambda: generate_spdx(inspection["filesystem"], spdx, sha(archive), producer, dependencies=inspection["dependencies"]))
        provenance = dist / (archive.name + ".provenance.json")
        at_stage("archive-provenance:" + progress, lambda: write_json(provenance, producer_statement(archive, spdx, inspection, producer)))
        for path in (archive, spdx, provenance):
            payloads[path.name] = sha(path)
            shutil.copyfile(path, shipping / path.name)
    if targets != set(expected):
        raise ValueError("missing configured target archive")
    import client_image
    image_files, image_receipt = at_stage("oci-image", client_image.build, dist, stage, inspections, producer)
    for path in image_files:
        payloads[path.name] = sha(path)
        shutil.copyfile(path, shipping / path.name)
    notes = ROOT / "docs/releases" / (producer["tag"] + ".md")
    if not notes.is_file() or notes.is_symlink() or not notes.read_bytes().strip():
        raise ValueError("reviewed release notes are missing")
    shutil.copyfile(notes, shipping / "release-notes.md")
    registry = ROOT / "terraform-registry-manifest.json"
    if registry.exists():
        payloads["terraform-registry-manifest.json"] = sha(registry)
        shutil.copyfile(registry, shipping / registry.name)
    receipt = {"source": producer, "files": payloads, "releaseNotesSHA256": sha(notes), "image": image_receipt}
    at_stage("receipt-write", write_json, dist / "release-content.json", receipt)
    (stage / "receipt.sha256").write_text(sha(dist / "release-content.json") + "\n")
    shutil.copyfile(dist / "release-content.json", shipping / "release-content.json")
    # Generated metadata and original compressed archives are all in scope.
    at_stage("shipping-scan", checked, [str(ROOT / "scripts/scan-release-content.sh"), str(shipping), "jayonthenet"])
    return receipt


def recheck(dist, artifact):
    dist, artifact = Path(dist).resolve(), Path(artifact).resolve()
    if artifact.parent != dist or artifact.is_symlink() or not artifact.is_file():
        raise ValueError("signed artifact must be an exact regular dist file")
    receipt_path = dist / "release-content.json"
    anchor = dist / "qualified-content/receipt.sha256"
    if receipt_path.is_symlink() or anchor.is_symlink() or anchor.read_text().strip() != sha(receipt_path):
        raise ValueError("qualified receipt changed before signing")
    receipt = json.loads(receipt_path.read_text())
    if receipt["source"] != identity():
        raise ValueError("qualified producer identity changed")
    notes = ROOT / "docs/releases" / (receipt["source"]["tag"] + ".md")
    if sha(notes) != receipt["releaseNotesSHA256"]:
        raise ValueError("reviewed release notes changed after qualification")
    for name, value in receipt["files"].items():
        path = ROOT / name if name == "terraform-registry-manifest.json" else dist / name
        if path.is_symlink() or not path.is_file() or sha(path) != value:
            raise ValueError("qualified shipping bytes changed before signing")
    if {path.name for path in archive_paths(dist)} != {name for name in receipt["files"] if name.endswith((".tar.gz", ".zip"))}:
        raise ValueError("archive set changed after qualification")
    checksum_name = ("checksums.txt" if module_path().endswith("/platform-orchestrator-cli") else
                     module_path().rsplit("/", 1)[1] + "_" + receipt["source"]["version"] + "_SHA256SUMS")
    if artifact.name not in receipt["files"] and artifact.name not in ("release-content.json", checksum_name):
        raise ValueError("signer received an unqualified artifact")
    if artifact.name == checksum_name:
        verify_checksums(artifact, receipt, dist)
    with tempfile.TemporaryDirectory(prefix="client-signing-check-") as temporary:
        shutil.copyfile(artifact, Path(temporary) / artifact.name)
        checked([str(ROOT / "scripts/scan-release-content.sh"), temporary, "jayonthenet"])


def verify_checksums(path, receipt, dist):
    expected = dict(receipt["files"])
    if "terraform-registry-manifest.json" in expected:
        value = expected.pop("terraform-registry-manifest.json")
        expected[module_path().rsplit("/", 1)[1] + "_" + receipt["source"]["version"] + "_manifest.json"] = value
    expected["release-content.json"] = sha(Path(dist) / "release-content.json")
    actual = {}
    for line in Path(path).read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.+-]+)", line)
        if not match or match[2] in actual:
            raise ValueError("malformed or duplicate release checksum entry")
        actual[match[2]] = match[1]
    if actual != expected:
        raise ValueError("signed checksums must bind every exact qualified shipping asset")


def main():
    command, *args = sys.argv[1:]
    if command == "qualify":
        at_stage("qualify", qualify, *args)
    elif command == "recheck":
        at_stage("recheck", recheck, *args)
    else:
        raise ValueError("unknown client qualification command")


def run():
    try:
        at_stage("entrypoint", main)
    except QualificationStageFailure as error:
        detail = " class=" + error.category
        if error.rule_id:
            detail += " rule=" + error.rule_id
        if error.tool:
            detail += " tool=" + error.tool + " exit=" + str(error.returncode)
        print("client release qualification failed at " + str(error) + detail, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(run())
