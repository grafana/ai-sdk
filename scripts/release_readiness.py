#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


PREFIX = "github.com/grafana/ai-sdk"
CANONICAL = "https://github.com/grafana/ai-sdk.git"
MANIFEST = ".release-please-manifest.json"
RELEASE_APP = "grafana-ai-sdk-release[bot]"


def run(*args, cwd=None, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True).strip()


def changed_component(before, after):
    changed = [key for key in before.keys() | after.keys() if before.get(key) != after.get(key)]
    if len(changed) != 1 or changed[0] not in after:
        raise ValueError("release must change exactly one component version without removing it")
    return changed[0]


def verify_release(pr, before, after, config):
    component = changed_component(before, after)
    if component not in config["packages"]:
        raise ValueError("release component is not registered")
    if pr["user"].get("login") != RELEASE_APP or pr["user"].get("type") != "Bot":
        raise ValueError("manifest version changes require the dedicated release App")
    if pr["base"]["ref"] != "main" or pr["base"]["repo"]["full_name"] != "grafana/ai-sdk":
        raise ValueError("release must target canonical main")
    if pr["head"]["repo"]["full_name"] != "grafana/ai-sdk":
        raise ValueError("release branch must belong to the canonical repository")
    package = config["packages"][component]
    branch = "release-please--branches--main"
    branch_component = package.get("component") or package.get("package-name")
    if branch_component:
        branch += "--components--" + branch_component
    if pr["head"]["ref"] != branch:
        raise ValueError("release branch does not match release-please metadata")
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", after[component]):
        raise ValueError("release manifest contains an invalid version")
    return component


def owner(path, roots):
    return next((root for root in sorted(roots, key=len, reverse=True)
                 if root != "." and path.startswith(root + "/")), ".")


def owned_snapshot(entries, root, roots):
    snapshot = {}
    for path, blob in entries.items():
        if owner(path, roots) != root:
            continue
        relative = path if root == "." else path[len(root) + 1:]
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
    for line in run("git", "ls-tree", "-r", ref).splitlines():
        metadata, path = line.split("\t", 1)
        entries[path] = metadata.split()[2]
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


def verify_release_files(component, files):
    changelog = "CHANGELOG.md" if component == "." else f"{component}/CHANGELOG.md"
    if not set(files).issubset({MANIFEST, changelog}) or MANIFEST not in files:
        raise ValueError("release candidate may only update its generated changelog and version manifest")


def verify_library(root, base, roots):
    if root == "ai-gateway":
        raise ValueError("Gateway application readiness awaits #263 and workspace release attribution; publication remains disabled")
    with tempfile.TemporaryDirectory(prefix="release-readiness-") as cache:
        env = dict(os.environ, GOWORK="off", GOFLAGS="-mod=readonly -modcacherw",
                   GOPROXY="https://proxy.golang.org", GOPRIVATE="", GONOPROXY="",
                   GONOSUMDB="", GOSUMDB="sum.golang.org", GOMODCACHE=cache)
        graph = list(json_stream(run("go", "list", "-m", "-json", "all", cwd=root, env=env)))
        baseline = tree(base)
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
            if re.search(r"-\d{14}-[0-9a-f]{12}$", version):
                raise ValueError(f"library publication requires a tagged prerequisite: {path}@{version}")
            metadata = json.loads(run("go", "mod", "download", "-json", f"{path}@{version}", env=env))
            origin = metadata.get("Origin", {})
            commit = origin.get("Hash", "")
            subdir = "" if dependency == "." else dependency
            if (metadata.get("Error") or metadata.get("Path") != path or metadata.get("Version") != version
                    or origin.get("VCS") != "git" or origin.get("URL", "").removesuffix(".git") != CANONICAL.removesuffix(".git")
                    or origin.get("Subdir", "") != subdir or not re.fullmatch(r"[0-9a-f]{40}", commit)):
                raise ValueError(f"unverified canonical prerequisite: {path}@{version}")
            subprocess.run(["git", "merge-base", "--is-ancestor", commit, base], check=True)
            if owned_snapshot(tree(commit), dependency, roots) != owned_snapshot(baseline, dependency, roots):
                raise ValueError(f"prerequisite snapshot is stale: {path}@{version}; release its current owned source first")
        subprocess.run(["bash", "scripts/module-policy.sh", "standalone", root], check=True, env=env)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--pull-request", required=True)
    args = parser.parse_args()
    pr = json.loads(Path(args.pull_request).read_text())
    base = pr["base"]["sha"]
    after = json.loads(Path(MANIFEST).read_text())
    base_files = tree(base)
    if MANIFEST not in base_files:
        workflow = Path(".github/workflows/release-please.yml").read_text()
        if "skip-github-release: true" not in workflow:
            raise ValueError("initial adoption must keep automatic publication disabled")
        print("Release readiness: adoption change; publication disabled")
        return
    before = json.loads(run("git", "show", f"{base}:{MANIFEST}"))
    if before == after:
        print("Release readiness: ordinary source PR; publication validation not applicable")
        return
    config = json.loads(Path("release-please-config.json").read_text())
    component = verify_release(pr, before, after, config)
    subprocess.run(["git", "fetch", "--no-tags", CANONICAL, "refs/heads/main"], check=True)
    canonical = run("git", "rev-parse", "FETCH_HEAD")
    verify_candidate(pr, canonical, "HEAD")
    verify_release_files(component, run("git", "diff", "--name-only", canonical, "HEAD").splitlines())
    roots = set(config["packages"])
    verify_library(component, canonical, roots)
    print(f"Release readiness passed: {component} at {run('git', 'rev-parse', 'HEAD')}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"release readiness: {error}")
