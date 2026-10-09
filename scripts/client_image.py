#!/usr/bin/env python3
"""Build locally, inspect every original layer, and retain the exact OCI bytes."""
import datetime
import json
from pathlib import Path
import re
import shutil

import client_oci
import qualify_release as release
import release_content


IMAGE = "ghcr.io/stellwerk-labs/octl"


def image_tools():
    tools = json.loads((release.ROOT / "scripts/release-image-tools.json").read_text())
    if set(tools) != {"buildkit", "sbomGenerator"}:
        raise ValueError("unexpected release image build tool")
    for name, repository in (("buildkit", "moby/buildkit"), ("sbomGenerator", "docker/buildkit-syft-scanner")):
        if (set(tools[name]["platforms"]) != {"linux/amd64", "linux/arm64"}
                or not re.fullmatch(re.escape(repository) + r"@sha256:[0-9a-f]{64}", tools[name]["image"])
                or any(not re.fullmatch(r"sha256:[0-9a-f]{64}", digest) for digest in tools[name]["platforms"].values())):
            raise ValueError("release image build tool must have a qualified multiarch digest pin")
    return tools


def build(dist, stage, inspections, producer):
    tools = image_tools()
    context = stage / "image-context"
    context.mkdir()
    for arch in ("amd64", "arm64"):
        target = "linux/" + arch
        if target not in inspections:
            raise ValueError("CLI image requires both qualified Linux binaries")
        destination = context / "linux" / arch
        destination.mkdir(parents=True)
        shutil.copyfile(inspections[target]["binary"], destination / "octl")
        (destination / "octl").chmod(0o755)
    shutil.copytree(release.ROOT / "licenses", context / "licenses")
    shutil.copyfile(release.ROOT / "goreleaser.Dockerfile", context / "goreleaser.Dockerfile")
    epoch = str(int(datetime.datetime.fromisoformat(producer["created"].replace("Z", "+00:00")).timestamp()))
    archive = stage / "client-image.tar"
    release.checked([
        "docker", "buildx", "build", "--pull", "--platform", "linux/amd64,linux/arm64",
        "--tag", IMAGE + ":" + producer["version"],
        "--file", "goreleaser.Dockerfile", "--build-arg", "SOURCE_DATE_EPOCH=" + epoch,
        "--label", "org.opencontainers.image.version=" + producer["version"],
        "--label", "org.opencontainers.image.created=" + producer["created"],
        "--label", "org.opencontainers.image.revision=" + producer["sourceSHA"],
        "--provenance=mode=max", "--attest", "type=sbom,generator=" + tools["sbomGenerator"]["image"],
        "--output", "type=oci,dest=" + str(archive.resolve()) + ",rewrite-timestamp=true", ".",
    ], cwd=context)
    source = stage / "image-archive"
    source.mkdir()
    shutil.move(archive, source / archive.name)
    expanded = stage / "image-unpacked"
    release_content.unpack(source, expanded, max_members=100000, max_bytes=512 * 1024 * 1024)
    roots = list(expanded.iterdir())
    if len(roots) != 1:
        raise ValueError("unexpected OCI archive layout")
    layout = stage / "image-layout"
    shutil.move(roots[0], layout)
    inspection = stage / "image-inspection"
    report = client_oci.export(layout, inspection, ("amd64", "arm64"), release.module_path(), ("usr/local/bin/octl",))
    for arch, payload in report["payloads"].items():
        if payload["binaries"]["usr/local/bin/octl"] != "sha256:" + inspections["linux/" + arch]["binarySHA256"]:
            raise ValueError("image bytes differ from qualified archive binary")
    release.checked([str(release.ROOT / "scripts/scan-release-content.sh"), str(inspection), "jayonthenet"])
    files = []
    for arch, payload in report["payloads"].items():
        selected = stage / ("sbom-image-" + arch)
        release.checked(["oras", "cp", "--no-tty", "--from-oci-layout", "--to-oci-layout",
                         str(layout) + "@" + payload["manifest"], str(selected) + ":qualified"])
        client_oci.validate_sbom_layout(selected, payload["manifest"])
        spdx = dist / ("octl-image-linux-" + arch + ".spdx.json")
        release.generate_spdx(selected, spdx, payload["manifest"].removeprefix("sha256:"), producer, "oci-dir",
                              inspections["linux/" + arch]["dependencies"])
        files.append(spdx)
    evidence = dist / "octl-image-content.json"
    release.write_json(evidence, {"sourceSHA": producer["sourceSHA"], "image": IMAGE, "buildTools": tools, **report})
    provenance = dist / "octl-image.provenance.json"
    predicate = {
        "buildDefinition": {
            "buildType": "https://stellwerk.dev/build/goreleaser-client-image/v1",
            "externalParameters": {"repository": producer["repository"], "version": producer["version"],
                                   "tag": producer["tag"], "workflowRef": producer["workflowRef"]},
            "resolvedDependencies": [
                {"uri": "git+https://github.com/" + producer["repository"],
                 "digest": {"gitCommit": producer["sourceSHA"]}},
                {"uri": "git+https://github.com/" + producer["repository"],
                 "name": producer["workflowRef"].split("@")[0].removeprefix(producer["repository"] + "/"),
                 "digest": {"gitCommit": producer["workflowSHA"]}},
                *[{"uri": "oci://" + tool["image"], "digest": {"sha256": tool["image"].rsplit(":", 1)[1]}}
                  for tool in tools.values()],
                {"uri": "oci://" + release.SYFT, "digest": {"sha256": release.SYFT.rsplit(":", 1)[1]}},
            ],
            "internalParameters": {"imageIndex": report["index"], "payloads": report["payloads"],
                                   "builtinBuildKitAttestationsPreserved": True},
        },
        "runDetails": {"builder": {"id": "https://github.com/" + producer["workflowRef"]},
                       "metadata": {"invocationId": "https://github.com/" + producer["repository"] +
                                    "/actions/runs/" + producer["runID"] + "/attempts/" + producer["runAttempt"]}},
    }
    release.write_json(provenance, {"_type": "https://in-toto.io/Statement/v1",
                                   "subject": [{"name": IMAGE, "digest": {"sha256": report["index"][7:]}}],
                                   "predicateType": release.PROVENANCE, "predicate": predicate})
    files.extend((evidence, provenance))
    return files, {"digest": report["index"], "layout": "qualified-content/image-layout",
                   "report": evidence.name, "provenance": provenance.name}


def reinspect(dist, receipt, output):
    image = receipt["image"]
    if image.get("layout") != "qualified-content/image-layout" or image.get("report") != "octl-image-content.json":
        raise ValueError("unexpected qualified image location")
    expected = json.loads((dist / image["report"]).read_text())
    report = client_oci.export(dist / image["layout"], output, ("amd64", "arm64"),
                               release.module_path(), ("usr/local/bin/octl",))
    if report["index"] != image["digest"] or expected != {"sourceSHA": receipt["source"]["sourceSHA"], "image": IMAGE,
                                                       "buildTools": image_tools(), **report}:
        raise ValueError("OCI content changed after qualification")
    release.checked([str(release.ROOT / "scripts/scan-release-content.sh"), str(output), "jayonthenet"])
    return report
