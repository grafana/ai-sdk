#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


PREFIX = "github.com/grafana/ai-sdk"
CANONICAL = "https://github.com/grafana/ai-sdk.git"
MANIFEST = ".release-please-manifest.json"
RELEASE_APP = "grafana-ai-sdk-release[bot]"
RELEASE_BOT_ID = 332654929
REPOSITORY_ID = 1315458055
VERSION = r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"


def run(*args, cwd=None, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True).strip()


def changed_component(before, after):
    changed = [key for key in before.keys() | after.keys() if before.get(key) != after.get(key)]
    if len(changed) != 1 or changed[0] not in after:
        raise ValueError("release must change exactly one component version without removing it")
    return changed[0]


def release_intent(pr):
    return (pr.get("user", {}).get("id") == RELEASE_BOT_ID
            or pr.get("head", {}).get("ref", "").startswith("release-please--")
            or bool(re.match(r"^chore(?:\([^)]*\))?: release(?: |$)", pr.get("title", ""))))


def release_notes(pr, version):
    parts = re.split(r"(?m)^---$", (pr.get("body") or "").replace("\r\n", "\n"))
    if len(parts) != 3:
        raise ValueError("release body must contain one release notes section")
    notes = parts[1].strip()
    if not re.match(r"^#{2,3} \[?" + re.escape(version) + r"(?:\]| |$)", notes):
        raise ValueError("release body version does not match manifest")
    if "<details>" in notes or "<summary>" in notes:
        raise ValueError("multiple component release bodies are unsupported")
    return notes


def verify_changelog(before, after, notes):
    match = re.search(r"\n###? v?[0-9\[]", before)
    if match:
        expected = (before[:match.start()] + "\n" + notes + "\n" + before[match.start():]).strip() + "\n"
    elif before:
        expected = "# Changelog\n\n" + notes + "\n\n" + re.sub(r"(?m)^#(\s)", r"##\1", before).strip() + "\n"
    else:
        expected = "# Changelog\n\n" + notes + "\n"
    if after != expected:
        raise ValueError("changelog must prepend exactly the release body notes and preserve history")


def verify_release_metadata(pr, before, after, config):
    component = changed_component(before, after)
    if component not in config["packages"]:
        raise ValueError("release component is not registered")
    if (pr["user"].get("login") != RELEASE_APP or pr["user"].get("type") != "Bot"
            or pr["user"].get("id") != RELEASE_BOT_ID):
        raise ValueError("manifest version changes require the dedicated release App")
    if pr["base"]["ref"] != "main" or pr["base"]["repo"]["full_name"] != "grafana/ai-sdk":
        raise ValueError("release must target canonical main")
    if pr["head"]["repo"]["full_name"] != "grafana/ai-sdk":
        raise ValueError("release branch must belong to the canonical repository")
    if any(pr[side]["repo"].get("id") != REPOSITORY_ID for side in ("base", "head")):
        raise ValueError("release must belong to the immutable canonical repository")
    package = config["packages"][component]
    branch = "release-please--branches--main"
    branch_component = package.get("component") or package.get("package-name")
    if branch_component:
        branch += "--components--" + branch_component
    if pr["head"]["ref"] != branch:
        raise ValueError("release branch does not match release-please metadata")
    if not re.fullmatch(VERSION, after[component]):
        raise ValueError("release manifest contains an invalid version")
    name = " " + branch_component if branch_component else ""
    if pr.get("title") != f"chore(main): release{name} {after[component]}":
        raise ValueError("release title does not match component and manifest version")
    release_notes(pr, after[component])
    return component


def verify_release(pr, before, after, config):
    component = verify_release_metadata(pr, before, after, config)
    if pr.get("state") != "open" or pr.get("draft") is not False:
        raise ValueError("release candidate must be an open non-draft PR")
    return component


def owner(path, roots):
    return next((root for root in sorted(roots, key=len, reverse=True)
                 if root != "." and path.startswith(root + "/")), ".")


def embedded_directories(ref, entries, root, roots):
    directories = set()
    for path in entries:
        if path.endswith(".go") and owned_snapshot({path: entries[path]}, root, roots):
            content = run("git", "show", f"{ref}:{path}")
            if re.search(r"(?m)^[ \t]*//go:embed[ \t]", content):
                directories.add(str(Path(path).parent))
    return directories


def owned_snapshot(entries, root, roots, embedded=()):
    snapshot = {}
    for path, blob in entries.items():
        if owner(path, roots) != root:
            continue
        relative = path if root == "." else path[len(root) + 1:]
        if any(directory == "." or path.startswith(directory + "/") for directory in embedded):
            snapshot[relative] = blob
            continue
        if any(piece in {"test", "tests", "testdata", "docs", "examples", "openspec",
                         "release", "scripts", ".github", ".agents", ".claude"}
               for piece in relative.split("/")[:-1]):
            continue
        if relative.endswith((".md", "_test.go")):
            continue
        if root == "." and relative in {MANIFEST, "release-please-config.json", "mise.toml",
                                         "renovate.json", "go.work", "go.work.sum",
                                         "go.gateway.work", "go.gateway.work.sum"}:
            continue
        if relative.startswith("."):
            continue
        snapshot[relative] = blob
    return snapshot


def tree(ref):
    entries = {}
    for line in run("git", "ls-tree", "-rz", ref).split("\0"):
        if not line:
            continue
        metadata, path = line.split("\t", 1)
        entries[path] = metadata
    return entries


def json_stream(raw):
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        value, end = decoder.raw_decode(raw)
        yield value
        raw = raw[end:]


def verify_candidate(pr, base, head):
    if pr["base"]["sha"] != base:
        raise ValueError("release PR base is stale; update it against current canonical main")
    expected = [base, pr["head"]["sha"]]
    parents = run("git", "show", "-s", "--format=%P", head).split()
    if parents != expected:
        raise ValueError("checkout must be the merge of the current canonical base and release head")


def verify_clean_checkout():
    if run("git", "status", "--porcelain", "--untracked-files=normal"):
        raise ValueError("release validation requires a clean candidate checkout")


def verify_release_files(component, files, entries=None):
    changelog = "CHANGELOG.md" if component == "." else f"{component}/CHANGELOG.md"
    allowed = {MANIFEST, changelog}
    if component == "ai-gateway":
        allowed.add("ai-gateway/package.json")
    if not set(files).issubset(allowed) or MANIFEST not in files:
        raise ValueError("release candidate may only update its generated changelog and version manifest")
    if entries is not None and any(not entries[path].startswith("100644 blob ") for path in files):
        raise ValueError("generated release files must be ordinary non-executable files")


def verify_tag(root, version, commit):
    tag = version if root == "." else f"{root}/{version}"
    run("git", "fetch", "--no-tags", CANONICAL, f"refs/tags/{tag}")
    if run("git", "rev-parse", "FETCH_HEAD^{commit}") != commit:
        raise ValueError(f"canonical tag target differs from prerequisite origin: {tag}")


def source_snapshot(ref, root, roots):
    entries = tree(ref)
    return owned_snapshot(entries, root, roots, embedded_directories(ref, entries, root, roots))


def verify_library(root, base, roots, tooling_root=None):
    if root == "ai-gateway":
        raise ValueError("Gateway application readiness awaits a plugin publication workflow and workspace release attribution; publication remains disabled")
    with tempfile.TemporaryDirectory(prefix="release-readiness-") as cache:
        env = dict(os.environ, GOWORK="off", GOFLAGS="-mod=readonly -modcacherw",
                   GOPROXY="https://proxy.golang.org", GOPRIVATE="", GONOPROXY="",
                   GONOSUMDB="", GOSUMDB="sum.golang.org", GOMODCACHE=cache)
        manifest = json.loads(run("go", "mod", "edit", "-json", cwd=root, env=env))
        if manifest.get("Replace"):
            raise ValueError("selected library contains replace directives")
        graph = list(json_stream(run("go", "list", "-m", "-json", "all", cwd=root, env=env)))
        for module in graph:
            path = module["Path"]
            if module.get("Replace"):
                raise ValueError(f"selected replacement is not releasable: {path}")
            if module.get("Main") or not (path == PREFIX or path.startswith(PREFIX + "/")):
                continue
            dependency = "." if path == PREFIX else path[len(PREFIX) + 1:]
            if dependency not in roots or dependency == "ai-gateway":
                raise ValueError(f"unsupported library prerequisite: {path}")
            version = module["Version"]
            if not re.fullmatch("v" + VERSION, version) or re.search(r"[.-]\d{14}-[0-9a-f]{12}$", version):
                raise ValueError(f"library publication requires a tagged prerequisite: {path}@{version}")
            metadata = json.loads(run("go", "mod", "download", "-json", f"{path}@{version}", env=env))
            origin = metadata.get("Origin", {})
            commit = origin.get("Hash", "")
            subdir = "" if dependency == "." else dependency
            if (metadata.get("Error") or metadata.get("Path") != path or metadata.get("Version") != version
                    or origin.get("VCS") != "git" or origin.get("URL", "").removesuffix(".git") != CANONICAL.removesuffix(".git")
                    or origin.get("Subdir", "") != subdir or not re.fullmatch(r"[0-9a-f]{40}", commit)):
                raise ValueError(f"unverified canonical prerequisite: {path}@{version}")
            module["Origin"] = origin
            verify_tag(dependency, version, commit)
            subprocess.run(["git", "merge-base", "--is-ancestor", commit, base], check=True)
            if source_snapshot(commit, dependency, roots) != source_snapshot(base, dependency, roots):
                raise ValueError(f"prerequisite snapshot is stale: {path}@{version}; release its current owned source first")
        policy = str(Path(tooling_root) / "scripts/module-policy.sh") if tooling_root else "scripts/module-policy.sh"
        subprocess.run(["bash", policy, "standalone", root], check=True, env=env)
        from release_consumer import verify_consumer
        verify_consumer(root, env, graph, tooling_root=tooling_root)
        return [{"path": m["Path"], "version": m.get("Version"), "origin": m.get("Origin")}
                for m in graph if not m.get("Main")]


def release_publication_disabled(workflow):
    action = re.search(
        r"(?m)^        uses: googleapis/release-please-action@[^\n]+\n"
        r"        with:\n(?P<options>(?:          [^\n]*\n)*)",
        workflow,
    )
    return bool(action and re.search(
        r"(?m)^          skip-github-release: true(?:\s+#.*)?$",
        action.group("options"),
    ))


def release_automation_disabled(workflow):
    return bool(
        re.search(
            r"(?m)^  release-please:\n    if: \$\{\{ false \}\}(?:[ \t]+#.*)?$",
            workflow,
        )
    )


def verify_live_candidate(pr, before, after, config):
    live = json.loads(run("gh", "api", f"repos/grafana/ai-sdk/pulls/{pr['number']}"))
    if any(live.get(key) != pr.get(key) for key in ("title", "body", "user", "state", "draft")):
        raise ValueError("release metadata changed during validation; rerun readiness")
    run("git", "fetch", "--no-tags", CANONICAL, "refs/heads/main")
    verify_release(live, before, after, config)
    verify_candidate(live, run("git", "rev-parse", "FETCH_HEAD"), "HEAD")


def readiness_record(pr, component, version):
    return {
        "schemaVersion": 1,
        "repositoryId": REPOSITORY_ID,
        "repository": "grafana/ai-sdk",
        "prNumber": pr["number"],
        "component": component,
        "version": version,
        "headSha": pr["head"]["sha"],
        "baseSha": pr["base"]["sha"],
        "mergeSha": run("git", "rev-parse", "HEAD"),
        "treeSha": run("git", "rev-parse", "HEAD^{tree}"),
        "titleDigest": hashlib.sha256(pr["title"].encode()).hexdigest(),
        "bodyDigest": hashlib.sha256(pr["body"].encode()).hexdigest(),
        "toolingRevision": pr["base"]["sha"],
        "workflowBlob": run("git", "rev-parse", "HEAD:.github/workflows/release-readiness.yml"),
        "runId": os.environ.get("GITHUB_RUN_ID"),
        "runAttempt": os.environ.get("GITHUB_RUN_ATTEMPT"),
    }


def main():
    sys.dont_write_bytecode = True
    parser = argparse.ArgumentParser()
    parser.add_argument("--pull-request", required=True)
    parser.add_argument("--evidence", type=Path)
    args = parser.parse_args()
    if args.evidence:
        args.evidence.unlink(missing_ok=True)
    pr = json.loads(Path(args.pull_request).read_text())
    base = pr["base"]["sha"]
    after = json.loads(Path(MANIFEST).read_text())
    base_files = tree(base)
    if MANIFEST not in base_files:
        workflow = Path(".github/workflows/release-please.yml").read_text()
        if not release_publication_disabled(workflow) or not release_automation_disabled(workflow):
            raise ValueError("initial adoption must keep release automation and publication disabled")
        print("Release readiness: adoption change; release automation disabled pending #245")
        return
    before = json.loads(run("git", "show", f"{base}:{MANIFEST}"))
    if before == after and not release_intent(pr):
        print("Release readiness: ordinary source PR; publication validation not applicable")
        return
    config = json.loads(run("git", "show", f"{base}:release-please-config.json"))
    component = verify_release(pr, before, after, config)
    verify_clean_checkout()
    run("git", "fetch", "--no-tags", CANONICAL, "refs/heads/main")
    canonical = run("git", "rev-parse", "FETCH_HEAD")
    verify_candidate(pr, canonical, "HEAD")
    verify_release_files(component, run("git", "diff", "--name-only", canonical, "HEAD").splitlines(), tree("HEAD"))
    changelog = "CHANGELOG.md" if component == "." else f"{component}/CHANGELOG.md"
    old_notes = subprocess.check_output(["git", "show", f"{canonical}:{changelog}"], text=True) if changelog in tree(canonical) else ""
    verify_changelog(old_notes, Path(changelog).read_text(), release_notes(pr, after[component]))
    roots = set(config["packages"])
    verify_library(component, canonical, roots)
    verify_clean_checkout()
    verify_live_candidate(pr, before, after, config)
    if args.evidence:
        args.evidence.write_text(json.dumps(readiness_record(pr, component, after[component]), sort_keys=True) + "\n")
    print(f"Release readiness passed: {component} at {run('git', 'rev-parse', 'HEAD')}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"release readiness: {error}")
