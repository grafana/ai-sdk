#!/usr/bin/env python3
import argparse
from contextlib import redirect_stdout
import hashlib
import io
import json
from pathlib import Path
import re
import sys
import subprocess
import zipfile

import release_readiness as readiness


SHA = r"[0-9a-f]{40}"
RECORD_KEYS = {
    "schemaVersion", "repositoryId", "repository", "prNumber", "component", "version",
    "headSha", "baseSha", "mergeSha", "treeSha", "titleDigest", "bodyDigest",
    "toolingRevision", "workflowBlob", "runId", "runAttempt",
}


def read_record(archive):
    if len(archive) > 1024 * 1024:
        raise ValueError("readiness artifact exceeds size limit")
    with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
        files = bundle.infolist()
        if len(files) != 1 or files[0].filename != "library-readiness.json" or files[0].file_size > 65536:
            raise ValueError("readiness artifact must contain one bounded library-readiness.json")
        record = json.loads(bundle.read(files[0]))
    if not isinstance(record, dict) or set(record) != RECORD_KEYS or record["schemaVersion"] != 1:
        raise ValueError("unsupported readiness record schema")
    for field in ("headSha", "baseSha", "mergeSha", "treeSha", "toolingRevision", "workflowBlob"):
        if not isinstance(record[field], str) or not re.fullmatch(SHA, record[field]):
            raise ValueError(f"invalid readiness revision: {field}")
    if type(record["prNumber"]) is not int or record["prNumber"] <= 0:
        raise ValueError("invalid readiness PR number")
    for field in ("titleDigest", "bodyDigest"):
        if not isinstance(record[field], str) or not re.fullmatch(r"[0-9a-f]{64}", record[field]):
            raise ValueError(f"invalid readiness digest: {field}")
    for field in ("runId", "runAttempt"):
        if not isinstance(record[field], str) or not re.fullmatch(r"[1-9][0-9]*", record[field]):
            raise ValueError(f"invalid readiness run: {field}")
    return record


def verify_merged(candidate, pr, record):
    readiness.verify_clean_checkout()
    merge = readiness.run("git", "rev-parse", "HEAD")
    if pr.get("state") != "closed" or pr.get("merged") is not True or pr.get("draft") is not False:
        raise ValueError("candidate must be a merged non-draft PR")
    if merge != candidate["mergeSha"] or merge != pr["merge_commit_sha"]:
        raise ValueError("checkout differs from the frozen squash revision")
    parents = readiness.run("git", "show", "-s", "--format=%P", merge).split()
    if parents != [record["baseSha"]]:
        raise ValueError("squash parent must equal the validated candidate base")
    base = parents[0]
    before = json.loads(readiness.run("git", "show", f"{base}:{readiness.MANIFEST}"))
    after = json.loads(Path(readiness.MANIFEST).read_text())
    config = json.loads(readiness.run("git", "show", f"{base}:release-please-config.json"))
    if candidate["config"] != config:
        raise ValueError("release config differs from the merge parent")
    if candidate["parents"] != parents or candidate["treeSha"] != record["treeSha"]:
        raise ValueError("frozen commit identity differs from evidence")
    component = readiness.verify_release_metadata(pr, before, after, config)
    if component == "ai-gateway":
        raise ValueError("Gateway is unsupported by library authorization")
    if candidate["component"] != component or candidate["version"] != after[component]:
        raise ValueError("frozen release differs from merged component/version")
    expected_tag = ("" if component == "." else component + "/") + "v" + after[component]
    if candidate["tag"] != expected_tag:
        raise ValueError("frozen tag differs from the library version")
    expected = {
        "repositoryId": readiness.REPOSITORY_ID, "repository": "grafana/ai-sdk",
        "prNumber": pr["number"], "component": component, "version": after[component],
        "headSha": pr["head"]["sha"], "toolingRevision": base,
        "treeSha": readiness.run("git", "rev-parse", "HEAD^{tree}"),
        "workflowBlob": readiness.run("git", "rev-parse", f"{base}:.github/workflows/release-readiness.yml"),
        "titleDigest": hashlib.sha256(pr["title"].encode()).hexdigest(),
        "bodyDigest": hashlib.sha256(pr["body"].encode()).hexdigest(),
    }
    for key, value in expected.items():
        if record[key] != value:
            raise ValueError(f"validated candidate differs: {key}")
    reconstructed = readiness.run("git", "merge-tree", "--write-tree", base, pr["head"]["sha"])
    if reconstructed != record["treeSha"]:
        raise ValueError("squash tree differs from reconstructed candidate")
    readiness.verify_release_files(component, readiness.run("git", "diff", "--name-only", base, merge).splitlines(), readiness.tree(merge))
    changelog = "CHANGELOG.md" if component == "." else f"{component}/CHANGELOG.md"
    old = subprocess.check_output(["git", "show", f"{base}:{changelog}"], text=True) if changelog in readiness.tree(base) else ""
    notes = readiness.release_notes(pr, after[component])
    if candidate["notes"].strip() != notes:
        raise ValueError("frozen notes differ from validated release body")
    readiness.verify_changelog(old, Path(changelog).read_text(), notes)
    return component, base, set(config["packages"])


def main():
    sys.dont_write_bytecode = True
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=["record", "validate"])
    parser.add_argument("input", type=Path)
    parser.add_argument("--result", type=Path)
    args = parser.parse_args()
    if args.operation == "record":
        print(json.dumps(read_record(args.input.read_bytes())))
        return
    payload = json.loads(args.input.read_text())
    with redirect_stdout(sys.stderr):
        component, base, roots = verify_merged(payload["candidate"], payload["pr"], payload["record"])
        graph = readiness.verify_library(component, base, roots, tooling_root=Path(__file__).resolve().parent.parent)
        readiness.verify_clean_checkout()
    if not args.result:
        raise ValueError("validation requires a result path")
    args.result.write_text(json.dumps({"component": component, "baseSha": base, "prerequisites": graph}))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, zipfile.BadZipFile) as error:
        raise SystemExit(f"release authorization: {error}")
