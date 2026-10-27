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
COMBINED_DETAILS = re.compile(
    r"<details><summary>(?:(?P<component>[^:<]+): )?(?P<version>[^<]+)</summary>\n\n(?P<notes>.*?)\n</details>", re.S)


def run(*args, cwd=None, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True).strip()


def changed_components(before, after):
    changed = sorted(key for key in before.keys() | after.keys() if before.get(key) != after.get(key))
    if not changed or any(key not in after for key in changed):
        raise ValueError("release must change component versions without removing any")
    return changed


def separate(config, component):
    default = config.get("separate-pull-requests", len(config["packages"]) == 1)
    return config["packages"][component].get("separate-pull-requests", default)


def combined(config, components):
    return not (len(components) == 1 and separate(config, components[0]))


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


def verify_release_files(components, files, entries=None):
    allowed = {MANIFEST} | {changelog_path(component) for component in components}
    if GATEWAY in components:
        allowed.add("ai-gateway/package.json")
    if not all(path in allowed or path == "go.mod" or path.endswith("/go.mod") for path in files) or MANIFEST not in files:
        raise ValueError("release candidate may only update its generated changelogs, module requirements and version manifest")
    if entries is not None and any(not entries[path].startswith("100644 blob ") for path in files):
        raise ValueError("generated release files must be ordinary non-executable files")


def module_root(path):
    return "." if path == PREFIX else path[len(PREFIX) + 1:]


def first_party(path):
    return path == PREFIX or path.startswith(PREFIX + "/")


def expected_versions(after, config):
    versions = {}
    for root, package in config["packages"].items():
        version = after.get(root) or package.get("initial-version") or config.get("initial-version")
        if version:
            versions[PREFIX if root == "." else f"{PREFIX}/{root}"] = "v" + version
    return versions


def module_file(ref, path):
    with tempfile.NamedTemporaryFile("w", suffix=".mod") as handle:
        handle.write(run("git", "show", f"{ref}:{path}") + "\n")
        handle.flush()
        return json.loads(run("go", "mod", "edit", "-json", handle.name, env=dict(os.environ, GOWORK="off")))


def verify_requirement_updates(base, head, files, versions):
    for path in files:
        if path != "go.mod" and not path.endswith("/go.mod"):
            continue
        expected = module_file(base, path)
        for requirement in expected.get("Require") or []:
            if first_party(requirement["Path"]):
                requirement["Version"] = versions.get(requirement["Path"], requirement["Version"])
        if module_file(head, path) != expected:
            raise ValueError(f"release candidate may only update first-party requirements to the manifest versions: {path}")


def verify_tagged(root, version, base):
    tag = version if root == "." else f"{root}/{version}"
    run("git", "fetch", "--no-tags", CANONICAL, f"refs/tags/{tag}")
    subprocess.run(["git", "merge-base", "--is-ancestor", run("git", "rev-parse", "FETCH_HEAD^{commit}"), base], check=True)


def verify_requirements(component, components, versions, base):
    if component == GATEWAY:
        raise ValueError("Gateway application readiness awaits a plugin publication workflow and Gateway release attribution; publication remains disabled")
    manifest = json.loads(run("go", "mod", "edit", "-json", str(Path(component, "go.mod")), env=dict(os.environ, GOWORK="off")))
    requirements = []
    for requirement in manifest.get("Require") or []:
        path, version = requirement["Path"], requirement["Version"]
        if not first_party(path):
            continue
        dependency = module_root(path)
        if version != versions.get(path):
            raise ValueError(f"{component} requires {path}@{version}, expected {versions.get(path)}")
        co_released = dependency in components
        if not co_released:
            try:
                verify_tagged(dependency, version, base)
            except subprocess.CalledProcessError as error:
                raise ValueError(f"{component} requires {path}@{version}, which is neither a canonical tag nor released by this pull request") from error
        requirements.append({"path": path, "version": version, "coReleased": co_released})
    return requirements


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
    verify_clean_checkout()
    run("git", "fetch", "--no-tags", CANONICAL, "refs/heads/main")
    canonical = run("git", "rev-parse", "FETCH_HEAD")
    verify_candidate(pr, canonical, "HEAD")
    files = run("git", "diff", "--name-only", canonical, "HEAD").splitlines()
    verify_release_files(components, files, tree("HEAD"))
    versions = expected_versions(after, config)
    verify_requirement_updates(canonical, "HEAD", files, versions)
    notes = component_notes(pr, config, components, after)
    canonical_files = tree(canonical)
    for component in components:
        changelog = changelog_path(component)
        old_notes = subprocess.check_output(["git", "show", f"{canonical}:{changelog}"], text=True) if changelog in canonical_files else ""
        verify_changelog(old_notes, Path(changelog).read_text(), notes[component])
    for component in components:
        verify_requirements(component, components, versions, canonical)
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
