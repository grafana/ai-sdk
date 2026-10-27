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
LIBRARY_BRANCH = "release-please--branches--main"
COMBINED_TITLE = "chore: release main"
GATEWAY = "ai-gateway"
GO_LOCATIONS = {"GOROOT", "GOTOOLCHAIN", "GOCACHE", "GOMODCACHE", "GOPATH"}
COMBINED_DETAILS = re.compile(
    r"<details><summary>(?:(?P<component>[^:<]+): )?(?P<version>[^<]+)</summary>\n\n(?P<notes>.*?)\n</details>", re.S)


def run(*args, cwd=None, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True).strip()


def changed_components(before, after):
    changed = sorted(key for key in before.keys() | after.keys() if before.get(key) != after.get(key))
    if not changed or any(key not in after for key in changed):
        raise ValueError("release must change component versions without removing any")
    return changed


def changed_component(before, after):
    changed = changed_components(before, after)
    if len(changed) != 1:
        raise ValueError("release must change exactly one component version without removing it")
    return changed[0]


def separate(config, component):
    default = config.get("separate-pull-requests", len(config["packages"]) == 1)
    return config["packages"][component].get("separate-pull-requests", default)


def combined(config, components):
    return not (len(components) == 1 and separate(config, components[0]))


def module_path(component):
    return PREFIX if component == "." else f"{PREFIX}/{component}"


def changelog_path(component):
    return "CHANGELOG.md" if component == "." else f"{component}/CHANGELOG.md"


def combined_release_notes(pr, config):
    parts = re.split(r"(?m)^---$", (pr.get("body") or "").replace("\r\n", "\n"))
    if len(parts) != 3:
        raise ValueError("release body must contain one release notes section")
    content = parts[1].strip()
    blocks = list(COMBINED_DETAILS.finditer(content))
    if "\n\n".join(block.group(0) for block in blocks) != content:
        raise ValueError("combined release body must contain only component release sections")
    names = {package.get("component") or package.get("package-name"): path for path, package in config["packages"].items()}
    releases = {}
    for block in blocks:
        name = block.group("component")
        component = "." if name is None else names.get(name)
        if component is None or component in releases:
            raise ValueError(f"combined release body names an unknown or repeated component: {name}")
        version, notes = block.group("version"), block.group("notes").strip()
        if not re.match(r"^#{2,3} \[?" + re.escape(version) + r"(?:\]| |$)", notes):
            raise ValueError(f"release notes version does not match the summary: {component}")
        releases[component] = (version, notes)
    return releases


def component_notes(pr, config, components, after):
    if not combined(config, components):
        return {components[0]: release_notes(pr, after[components[0]])}
    releases = combined_release_notes(pr, config)
    if set(releases) != set(components) or any(releases[c][0] != after[c] for c in components):
        raise ValueError("combined release body does not match the manifest versions")
    return {component: notes for component, (_, notes) in releases.items()}


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
    components = changed_components(before, after)
    if any(component not in config["packages"] for component in components):
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
    if any(not re.fullmatch(VERSION, after[component]) for component in components):
        raise ValueError("release manifest contains an invalid version")
    if combined(config, components):
        if any(separate(config, component) for component in components):
            raise ValueError("separately released components cannot share a release pull request")
        if pr["head"]["ref"] != LIBRARY_BRANCH:
            raise ValueError("release branch does not match release-please metadata")
        if pr.get("title") != COMBINED_TITLE:
            raise ValueError("combined release title does not match release-please metadata")
        component_notes(pr, config, components, after)
        return components
    component = components[0]
    package = config["packages"][component]
    branch = LIBRARY_BRANCH
    branch_component = package.get("component") or package.get("package-name")
    if branch_component:
        branch += "--components--" + branch_component
    if pr["head"]["ref"] != branch:
        raise ValueError("release branch does not match release-please metadata")
    name = " " + branch_component if branch_component else ""
    if pr.get("title") != f"chore(main): release{name} {after[component]}":
        raise ValueError("release title does not match component and manifest version")
    release_notes(pr, after[component])
    return components


def verify_release(pr, before, after, config):
    components = verify_release_metadata(pr, before, after, config)
    if pr.get("state") != "open" or pr.get("draft") is not False:
        raise ValueError("release candidate must be an open non-draft PR")
    return components


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


def verify_release_files(components, files, entries=None, requirements=False):
    allowed = {MANIFEST} | {changelog_path(component) for component in components}
    if GATEWAY in components:
        allowed.add("ai-gateway/package.json")
    if requirements:
        for root in [*components, GATEWAY]:
            prefix = "" if root == "." else root + "/"
            allowed |= {prefix + "go.mod", prefix + "go.sum"}
    if not set(files).issubset(allowed) or MANIFEST not in files:
        raise ValueError("release candidate may only update its generated changelogs, requirement files and version manifest")
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


def candidate_check(base, candidate, proxy, tooling_root=None):
    tool = Path(tooling_root or ".") / "release/candidate"
    env = {key: value for key, value in os.environ.items() if not key.startswith("GO") or key in GO_LOCATIONS}
    output = run("go", "-C", str(tool), "run", "./cmd/candidate", "check", "-repo", str(Path.cwd()),
                 "-base", base, "-candidate", candidate, "-proxy", str(proxy),
                 env=dict(env, GOWORK="off", GOFLAGS="-mod=readonly"))
    return json.loads(output)



def verify_library(root, base, roots, tooling_root=None, co_released=None, proxy=None):
    if root == "ai-gateway":
        raise ValueError("Gateway application readiness awaits a plugin publication workflow and workspace release attribution; publication remains disabled")
    co_released = co_released or {}
    if co_released and proxy is None:
        raise ValueError("co-released prerequisites require the candidate proxy")
    with tempfile.TemporaryDirectory(prefix="release-readiness-") as cache:
        env = dict(os.environ, GOWORK="off", GOFLAGS="-mod=readonly -modcacherw",
                   GOPROXY="https://proxy.golang.org", GOPRIVATE="", GONOPROXY="",
                   GONOSUMDB="", GOSUMDB="sum.golang.org", GOMODCACHE=cache)
        if proxy is not None:
            env.update(GOPROXY=f"{Path(proxy).resolve().as_uri()},https://proxy.golang.org",
                       GONOSUMDB=PREFIX, CANDIDATE_PROXY=str(Path(proxy).resolve()))
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
            if path in co_released:
                if version != co_released[path]:
                    raise ValueError(f"co-released prerequisite must select its released version: {path}@{version}")
                continue
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


def readiness_record(pr, components):
    return {
        "schemaVersion": 2,
        "repositoryId": REPOSITORY_ID,
        "repository": "grafana/ai-sdk",
        "prNumber": pr["number"],
        "components": components,
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
    components = verify_release(pr, before, after, config)
    together = combined(config, components)
    verify_clean_checkout()
    run("git", "fetch", "--no-tags", CANONICAL, "refs/heads/main")
    canonical = run("git", "rev-parse", "FETCH_HEAD")
    verify_candidate(pr, canonical, "HEAD")
    verify_release_files(components, run("git", "diff", "--name-only", canonical, "HEAD").splitlines(), tree("HEAD"),
                         requirements=together)
    notes = component_notes(pr, config, components, after)
    for component in components:
        changelog = changelog_path(component)
        old_notes = subprocess.check_output(["git", "show", f"{canonical}:{changelog}"], text=True) if changelog in tree(canonical) else ""
        verify_changelog(old_notes, Path(changelog).read_text(), notes[component])
    roots = set(config["packages"])
    with tempfile.TemporaryDirectory(prefix="release-candidate-proxy-") as proxy:
        co_released = {}
        if together:
            candidate_check(canonical, run("git", "rev-parse", "HEAD"), proxy)
            co_released = {module_path(component): "v" + after[component] for component in components}
        for component in components:
            verify_library(component, canonical, roots, co_released=co_released, proxy=proxy if together else None)
    verify_clean_checkout()
    verify_live_candidate(pr, before, after, config)
    if args.evidence:
        record = readiness_record(pr, {component: after[component] for component in components})
        args.evidence.write_text(json.dumps(record, sort_keys=True) + "\n")
    print(f"Release readiness passed: {', '.join(components)} at {run('git', 'rev-parse', 'HEAD')}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"release readiness: {error}")
