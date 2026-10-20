import copy
from contextlib import chdir
import subprocess
import tempfile
import json
import re
from pathlib import Path
import unittest
from unittest.mock import patch

import release_readiness as readiness


class ReleaseReadinessTest(unittest.TestCase):
    def setUp(self):
        self.config = {"packages": {".": {}, "providers/openai": {"component": "providers/openai"}, "ai-gateway": {"component": "ai-gateway"}}}
        self.pr = {
            "state": "open", "draft": False,
            "title": "chore(main): release 0.2.0",
            "body": "Header\n---\n\n## 0.2.0 (2026-10-20)\n\n* Fix\n\n---\nFooter",
            "user": {"login": readiness.RELEASE_APP, "type": "Bot", "id": 332654929},
            "base": {"ref": "main", "sha": "base", "repo": {"full_name": "grafana/ai-sdk", "id": 1315458055}},
            "head": {"ref": "release-please--branches--main",
                     "sha": "head", "repo": {"full_name": "grafana/ai-sdk", "id": 1315458055}},
        }

    def test_identifies_one_component_without_labels(self):
        self.assertEqual(readiness.verify_release(self.pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config), ".")
        with self.assertRaises(ValueError):
            readiness.changed_component({".": "0.1.0"}, {".": "0.2.0", "providers/openai": "0.1.0"})
        with self.assertRaises(ValueError):
            readiness.changed_component({".": "0.1.0"}, {})
        self.pr["head"]["ref"] += "--components--providers/openai"
        self.pr["title"] = "chore(main): release providers/openai 0.1.0"
        self.pr["body"] = self.pr["body"].replace("0.2.0", "0.1.0")
        self.assertEqual(readiness.verify_release(self.pr, {}, {"providers/openai": "0.1.0"}, self.config), "providers/openai")

    def test_rejects_spoofed_release_metadata(self):
        for position, field, value in [
            ("user", "id", 123), ("base", "repo", {"full_name": "grafana/ai-sdk", "id": 123}),
            ("user", "login", "contributor"), ("user", "type", "User"),
            ("base", "ref", "development"), ("head", "ref", "ordinary-branch"),
            ("head", "repo", {"full_name": "fork/ai-sdk"}),
        ]:
            with self.subTest(field=field, value=value):
                pr = copy.deepcopy(self.pr)
                pr[position][field] = value
                with self.assertRaises(ValueError):
                    readiness.verify_release(pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config)

    def test_dirty_candidate_cannot_supply_release_evidence(self):
        for status in [" M core.go", "M  core.go", "?? injected.go"]:
            with patch.object(readiness, "run", return_value=status), self.assertRaises(ValueError):
                readiness.verify_clean_checkout()
        with patch.object(readiness, "run", return_value=""):
            readiness.verify_clean_checkout()

    def test_candidate_requires_current_base_and_exact_head(self):
        with patch.object(readiness, "run", return_value="base head"):
            readiness.verify_candidate(self.pr, "base", "HEAD")
            with self.assertRaises(ValueError):
                readiness.verify_candidate(self.pr, "new-base", "HEAD")
        with patch.object(readiness, "run", return_value="base older-head"):
            with self.assertRaises(ValueError):
                readiness.verify_candidate(self.pr, "base", "HEAD")

    def test_release_metadata_must_agree(self):
        for field, value in [("title", "chore(main): release 9.0.0"),
                             ("body", "---\n## 9.0.0\n---"),
                             ("state", "closed"), ("draft", True)]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                readiness.verify_release(dict(self.pr, **{field: value}),
                                         {".": "0.1.0"}, {".": "0.2.0"}, self.config)
        self.assertTrue(readiness.release_intent(self.pr))
        self.assertFalse(readiness.release_intent({"user": {"id": 1}, "head": {"ref": "fix"}, "title": "fix: bug"}))

    def test_live_candidate_changed_while_validation_runs(self):
        self.pr["number"] = 123
        for field, value in [("title", "edited"), ("body", "edited"), ("state", "closed")]:
            with self.subTest(field=field), patch.object(readiness, "run", return_value=json.dumps(dict(self.pr, **{field: value}))):
                with self.assertRaisesRegex(ValueError, "changed during validation"):
                    readiness.verify_live_candidate(self.pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config)
        with patch.object(readiness, "run", side_effect=[json.dumps(self.pr), "", "new-main"]):
            with self.assertRaisesRegex(ValueError, "base is stale"):
                readiness.verify_live_candidate(self.pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config)

    def test_ordinary_pr_and_release_without_manifest_change(self):
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            Path(readiness.MANIFEST).write_text('{".": "0.1.0"}')
            for release in [False, True]:
                pr = self.pr if release else {"user": {"id": 1}, "base": {"sha": "base"}, "head": {"ref": "fix"}, "title": "fix: bug"}
                Path("pr.json").write_text(json.dumps(pr))
                with patch("sys.argv", ["readiness", "--pull-request", "pr.json"]), \
                     patch.object(readiness, "tree", return_value={readiness.MANIFEST: "blob"}), \
                     patch.object(readiness, "run", side_effect=['{".": "0.1.0"}', json.dumps(self.config)]), \
                     patch.object(readiness, "verify_library") as library:
                    if release:
                        with self.assertRaisesRegex(ValueError, "exactly one component"):
                            readiness.main()
                    else:
                        readiness.main()
                    library.assert_not_called()

    def test_generated_changelog_preserves_history(self):
        notes = "## 0.2.0 (2026-10-20)\n\n* Fix"
        old = "# Changelog\n\n## 0.1.0\n\n* Old\n"
        new = "# Changelog\n\n" + notes + "\n\n## 0.1.0\n\n* Old\n"
        readiness.verify_changelog(old, new, notes)
        for invalid in [new.replace("* Old", "* Rewritten"), new.replace("0.2.0", "9.0.0"), ""]:
            with self.assertRaises(ValueError):
                readiness.verify_changelog(old, invalid, notes)

    def test_module_ownership_preserves_root_middleware(self):
        roots = {".", "middleware/logger", "providers/openai", "ai-gateway"}
        files = {"middleware/extract_json.go": "root", "middleware/logger/stream.go": "logger",
                 "providers/openai/model.go": "provider", "docs/guide.md": "docs",
                 "core_test.go": "test", "go.mod": "manifest", "templates/prompt.txt": "asset",
                 ".release-please-manifest.json": "release-state"}
        self.assertEqual(readiness.owned_snapshot(files, ".", roots), {
            "middleware/extract_json.go": "root", "go.mod": "manifest", "templates/prompt.txt": "asset"})
        self.assertEqual(readiness.owned_snapshot(files, "middleware/logger", roots), {"stream.go": "logger"})

    def test_embedded_assets_override_development_exclusions(self):
        entries = {"runtime/embed.go": "go", "runtime/docs/prompt.md": "asset",
                   "runtime/.hidden": "hidden", "docs/guide.md": "doc",
                   "providers/openai/model.go": "nested"}
        expected = {"runtime/embed.go": "go", "runtime/docs/prompt.md": "asset",
                    "runtime/.hidden": "hidden"}
        self.assertEqual(readiness.owned_snapshot(entries, ".", self.config["packages"], {"runtime"}), expected)
        for changed in [dict(entries, **{"runtime/docs/prompt.md": "new"}),
                        {k: v for k, v in entries.items() if k != "runtime/.hidden"},
                        dict(entries, **{"runtime/renamed.md": "asset"})]:
            self.assertNotEqual(readiness.owned_snapshot(changed, ".", self.config["packages"], {"runtime"}), expected)

    def test_tag_target_must_match_origin(self):
        with patch.object(readiness, "run", return_value="b" * 40), self.assertRaisesRegex(ValueError, "tag target"):
            readiness.verify_tag("providers/openai", "v0.1.0", "a" * 40)
        with patch.object(readiness, "run", return_value="a" * 40) as command:
            readiness.verify_tag("providers/openai", "v0.1.0", "a" * 40)
            self.assertIn("refs/tags/providers/openai/v0.1.0", command.call_args_list[0].args)

    def test_release_cannot_smuggle_dependency_or_workflow_edits(self):
        readiness.verify_release_files("providers/openai", [readiness.MANIFEST, "providers/openai/CHANGELOG.md"])
        for path in ["providers/openai/go.mod", ".github/workflows/release-please.yml", "core.go"]:
            with self.subTest(path=path), self.assertRaises(ValueError):
                readiness.verify_release_files("providers/openai", [readiness.MANIFEST, path])

    def test_generated_release_files_cannot_change_file_kind(self):
        files = [readiness.MANIFEST, "CHANGELOG.md"]
        for mode in ["120000", "100755", "160000"]:
            with self.subTest(mode=mode), self.assertRaises(ValueError):
                readiness.verify_release_files(".", files, {
                    readiness.MANIFEST: "100644 blob manifest", "CHANGELOG.md": f"{mode} blob notes"})

    def test_gateway_is_an_application_not_a_standalone_library(self):
        with self.assertRaisesRegex(ValueError, "Gateway application"):
            readiness.verify_library("ai-gateway", "base", self.config["packages"])

    def test_root_readiness_does_not_test_downstream_modules(self):
        graph = json.dumps({"Path": readiness.PREFIX, "Main": True})
        with patch.object(readiness, "run", return_value=graph), \
             patch.object(readiness, "tree", return_value={}), \
             patch("release_consumer.verify_consumer"), \
             patch.object(readiness.subprocess, "run") as execute:
            readiness.verify_library(".", "base", self.config["packages"])
            self.assertEqual(execute.call_count, 1)
            self.assertEqual(execute.call_args.args[0], ["bash", "scripts/module-policy.sh", "standalone", "."])

    def test_library_requires_tagged_fresh_canonical_prerequisite(self):
        dependency = {"Path": readiness.PREFIX, "Version": "v0.1.0"}
        origin = {"Hash": "a" * 40, "VCS": "git", "URL": readiness.CANONICAL, "Subdir": ""}
        metadata = dict(dependency, Origin=origin)
        for name, version, url, stale, accepted in [
            ("current", "v0.1.0", readiness.CANONICAL, False, True),
            ("stale", "v0.1.0", readiness.CANONICAL, True, False),
            ("fork", "v0.1.0", "https://github.com/fork/ai-sdk", False, False),
            ("pseudo", "v0.1.0-20260921194944-e7732b7aa62b", readiness.CANONICAL, False, False),
            ("prerelease pseudo", "v0.1.0-alpha.1.0.20260928202004-dd43c6dff058", readiness.CANONICAL, False, False),
        ]:
            with self.subTest(name=name):
                dependency["Version"] = version
                metadata["Version"] = version
                metadata["Origin"]["URL"] = url
                def command(*args, **kwargs):
                    return json.dumps({} if args[1:3] == ("mod", "edit") else metadata if args[1] == "mod" else dependency)
                def snapshot(ref):
                    return {"core.go": "different" if stale and ref == "base" else "same"}
                with patch.object(readiness, "run", side_effect=command), \
                     patch.object(readiness, "tree", side_effect=snapshot), \
                     patch.object(readiness, "embedded_directories", return_value=set()), \
                     patch.object(readiness, "verify_tag"), \
                     patch("release_consumer.verify_consumer"), \
                     patch.object(readiness.subprocess, "run") as execute:
                    if accepted:
                        readiness.verify_library("providers/openai", "base", self.config["packages"])
                        self.assertEqual(execute.call_count, 2)
                    else:
                        with self.assertRaises(ValueError):
                            readiness.verify_library("providers/openai", "base", self.config["packages"])

    def test_failures_stop_before_standalone(self):
        dependency = {"Path": readiness.PREFIX, "Version": "v0.1.0"}
        metadata = dict(dependency, Origin={"VCS": "git", "URL": readiness.CANONICAL, "Hash": "a" * 40})
        for failure in ["declared replacement", "selected replacement", "proxy", "origin", "unmerged", "missing tag"]:
            with self.subTest(failure=failure):
                def command(*args, **kwargs):
                    if args[1:3] == ("mod", "edit"):
                        return json.dumps({"Replace": [{}]} if failure == "declared replacement" else {})
                    if args[1] == "list":
                        return json.dumps(dict(dependency, Replace={"Path": "../root"}) if failure == "selected replacement" else dependency)
                    if failure == "proxy":
                        raise subprocess.CalledProcessError(1, args)
                    return json.dumps(dependency if failure == "origin" else metadata)
                with patch.object(readiness, "run", side_effect=command), \
                     patch.object(readiness, "verify_tag", side_effect=subprocess.CalledProcessError(1, "fetch") if failure == "missing tag" else None), \
                     patch.object(readiness.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "merge-base")) as execute, \
                     patch("release_consumer.verify_consumer") as consumer:
                    with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                        readiness.verify_library("providers/openai", "base", self.config["packages"])
                    self.assertFalse(any(call.args[0][0] == "bash" for call in execute.call_args_list))
                    consumer.assert_not_called()

    def test_real_git_snapshots_and_current_merge(self):
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            def git(*args):
                return subprocess.check_output(["git", "-c", "commit.gpgsign=false", *args], text=True, stderr=subprocess.PIPE).strip()
            git("init", "-q")
            git("config", "user.email", "test@example.com")
            git("config", "user.name", "Test")
            Path("runtime/docs").mkdir(parents=True)
            Path("runtime/embed.go").write_text('package runtime\nimport _ "embed"\n//go:embed docs/prompt.md\nvar prompt string\n')
            Path("runtime/docs/prompt.md").write_text("original")
            git("add", ".")
            git("commit", "-qm", "base")
            base = git("rev-parse", "HEAD")
            old = readiness.source_snapshot(base, ".", {"."})
            git("checkout", "-qb", "candidate")
            Path("CHANGELOG.md").write_text("release")
            git("add", ".")
            git("commit", "-qm", "candidate")
            head = git("rev-parse", "HEAD")
            git("checkout", "--detach", base)
            git("merge", "--no-ff", "-m", "merge", head)
            pr = copy.deepcopy(self.pr)
            pr["base"]["sha"], pr["head"]["sha"] = base, head
            readiness.verify_candidate(pr, base, "HEAD")
            self.assertEqual(old, readiness.source_snapshot("HEAD", ".", {"."}))
            Path("runtime/docs/prompt.md").write_text("behavior fix")
            git("add", ".")
            git("commit", "-qm", "asset fix")
            self.assertNotEqual(old, readiness.source_snapshot("HEAD", ".", {"."}))
            with self.assertRaises(ValueError):
                readiness.verify_candidate(pr, git("rev-parse", "HEAD"), "HEAD")

    def test_release_entry_point_validates_real_candidate_tree(self):
        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory, "repo")
            repo.mkdir()
            with chdir(repo):
                def git(*args):
                    return subprocess.check_output(["git", "-c", "commit.gpgsign=false", *args], text=True, stderr=subprocess.PIPE).strip()
                git("init", "-q")
                git("config", "user.email", "test@example.com")
                git("config", "user.name", "Test")
                Path(readiness.MANIFEST).write_text('{".": "0.1.0"}')
                Path("release-please-config.json").write_text(json.dumps(self.config))
                git("add", ".")
                git("commit", "-qm", "base")
                base = git("rev-parse", "HEAD")
                git("checkout", "-qb", "candidate")
                Path(readiness.MANIFEST).write_text('{".": "0.2.0"}')
                notes = readiness.release_notes(self.pr, "0.2.0")
                Path("CHANGELOG.md").write_text("# Changelog\n\n" + notes + "\n")
                git("add", ".")
                git("commit", "-qm", "release")
                head = git("rev-parse", "HEAD")
                git("checkout", "--detach", base)
                git("merge", "--no-ff", "-m", "merge", head)
                pr = copy.deepcopy(self.pr)
                pr["number"] = 123
                pr["base"]["sha"], pr["head"]["sha"] = base, head
                metadata = Path(directory, "pr.json")
                metadata.write_text(json.dumps(pr))
                real_run = readiness.run
                def command(*args, **kwargs):
                    if args[:2] == ("git", "fetch"):
                        return ""
                    if args == ("git", "rev-parse", "FETCH_HEAD"):
                        return base
                    if args[0] == "gh":
                        return json.dumps(pr)
                    return real_run(*args, **kwargs)
                with patch("sys.argv", ["readiness", "--pull-request", str(metadata)]), \
                     patch.object(readiness, "run", side_effect=command), \
                     patch.object(readiness, "verify_library") as library:
                    readiness.main()
                    library.assert_called_once_with(".", base, set(self.config["packages"]))

    def test_selected_standalone_ignores_broken_downstream(self):
        policy = Path(__file__).resolve().parent / "module-policy.sh"
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            subprocess.run(["git", "init", "-q"], check=True)
            Path("go.mod").write_text(f"module {readiness.PREFIX}\n\ngo 1.26.3\n")
            Path("core.go").write_text("package aisdk\nfunc Ready() bool { return true }\n")
            Path("core_test.go").write_text('package aisdk\nimport "testing"\nfunc TestReady(t *testing.T) { if !Ready() { t.Fatal("unready") } }\n')
            for root in ["providers/openai", "providers/bedrock"]:
                Path(root).mkdir(parents=True)
                Path(root, "go.mod").write_text(f"module {readiness.PREFIX}/{root}\n\ngo 1.26.3\n")
                Path(root, "broken.go").write_text("package downstream\nfunc Broken(\n")
            subprocess.run(["git", "add", "."], check=True)
            result = subprocess.run(["bash", str(policy), "standalone", "."], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertNotIn("providers/", result.stdout)
            result = subprocess.run(["bash", str(policy), "standalone", "providers/bedrock"], text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("syntax error", result.stderr)

    def test_consumer_scenarios_cover_registered_libraries(self):
        from release_consumer import SCENARIOS
        repo = Path(__file__).resolve().parent.parent
        config = json.loads((repo / "release-please-config.json").read_text())
        self.assertEqual(set(SCENARIOS), set(config["packages"]) - {".", "ai-gateway"})
        for root, scenario in SCENARIOS.items():
            with self.subTest(root=root):
                tests = "\n".join(path.read_text() for path in (repo / root).glob("*_test.go"))
                self.assertIn(f"func {scenario}(t *testing.T)", tests)

    def test_consumer_rejects_substituted_or_changed_dependency_graph(self):
        from release_consumer import VERSION, verify_graph
        path = readiness.PREFIX + "/providers/openai"
        with tempfile.TemporaryDirectory() as directory:
            candidate = {"Path": path, "Version": VERSION, "Dir": directory + "/candidate"}
            root = {"Path": readiness.PREFIX, "Version": "v0.1.0"}
            expected = {readiness.PREFIX: "v0.1.0"}
            verify_graph([candidate, root], path, expected, Path(directory))
            for graph in [[dict(candidate, Main=True), root],
                          [dict(candidate, Dir="/outside"), root],
                          [candidate, dict(root, Version="v0.2.0")],
                          [candidate, dict(root, Replace={"Path": "../.."})]]:
                with self.assertRaises(ValueError):
                    verify_graph(graph, path, expected, Path(directory))

    def test_release_automation_is_disabled_until_activation_review(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/release-please.yml").read_text()
        self.assertTrue(readiness.release_automation_disabled(workflow))
        self.assertFalse(readiness.release_automation_disabled(workflow.replace(
            "    if: ${{ false }}", "    if: ${{ true }}",
        )))
        self.assertTrue(readiness.release_publication_disabled(workflow))
        self.assertFalse(readiness.release_publication_disabled(workflow.replace(
            "          skip-github-release: true",
            "          skip-github-release: false # skip-github-release: true",
        )))
        self.assertFalse(readiness.release_publication_disabled(
            "# skip-github-release: true\n" + workflow.replace(
                "          skip-github-release: true", "",
            )
        ))
        self.assertIn("googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7", workflow)
        self.assertIn("release-please@17.6.0", (root / "mise.toml").read_text())

    def test_token_smoke_test_is_separate_from_release_automation(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/release-please.yml").read_text()
        smoke = re.split(r"(?m)^  [a-z][a-z-]*:\n", workflow.split("  token-smoke-test:\n", 1)[1], maxsplit=1)[0]
        self.assertIn("github.event_name == 'push'", smoke)
        self.assertIn("github.ref == 'refs/heads/main'", smoke)
        self.assertIn("github.repository == 'grafana/ai-sdk'", smoke)
        self.assertIn("gh api --paginate --slurp installation/repositories", smoke)
        self.assertNotIn("release-please-action", smoke)
        self.assertNotIn("actions/checkout", smoke)
        self.assertNotIn("contents: write", smoke)
        self.assertTrue(readiness.release_automation_disabled(workflow))


if __name__ == "__main__":
    unittest.main()
