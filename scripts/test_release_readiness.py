import copy
from contextlib import chdir
import subprocess
import tempfile
import json
import hashlib
import re
from pathlib import Path
import unittest
from unittest.mock import patch

import release_readiness as readiness


class ReleaseReadinessTest(unittest.TestCase):
    def setUp(self):
        self.config = {"separate-pull-requests": True, "packages": {".": {}, "providers/openai": {"component": "providers/openai"}, "ai-gateway": {"component": "ai-gateway"}}}
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
        self.assertEqual(readiness.verify_release(self.pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config), ["."])
        self.assertEqual(readiness.changed_components({".": "0.1.0"}, {".": "0.2.0", "providers/openai": "0.1.0"}),
                         [".", "providers/openai"])
        for after in [{".": "0.1.0"}, {}]:
            with self.subTest(after=after), self.assertRaises(ValueError):
                readiness.changed_components({".": "0.1.0"}, after)
        self.pr["head"]["ref"] += "--components--providers/openai"
        self.pr["title"] = "chore(main): release providers/openai 0.1.0"
        self.pr["body"] = self.pr["body"].replace("0.2.0", "0.1.0")
        self.assertEqual(readiness.verify_release(self.pr, {}, {"providers/openai": "0.1.0"}, self.config), ["providers/openai"])

    def combined_release(self):
        config = {"separate-pull-requests": False, "packages": {
            ".": {}, "providers/openai": {"component": "providers/openai"}, "providers/bedrock": {"component": "providers/bedrock"},
            "ai-gateway": {"component": "ai-gateway", "separate-pull-requests": True}}}
        root = "## [0.2.0](https://example.com) (2026-10-27)\n\n\n### Bug Fixes\n\n* root"
        openai = "## [0.1.1](https://example.com) (2026-10-27)\n\n\n### Bug Fixes\n\n* openai"
        body = ("Header\n---\n\n\n<details><summary>0.2.0</summary>\n\n" + root + "\n</details>\n\n"
                "<details><summary>providers/openai: 0.1.1</summary>\n\n" + openai + "\n</details>\n\n---\nFooter")
        pr = dict(copy.deepcopy(self.pr), title=readiness.COMBINED_TITLE, body=body)
        return config, pr, {".": "0.1.0", "providers/openai": "0.1.0"}, {".": "0.2.0", "providers/openai": "0.1.1"}, {".": root, "providers/openai": openai}

    def test_combined_release_metadata(self):
        config, pr, before, after, notes = self.combined_release()
        self.assertEqual(readiness.verify_release(pr, before, after, config), [".", "providers/openai"])
        self.assertEqual(readiness.component_notes(pr, config, [".", "providers/openai"], after), notes)
        single = dict(pr, body=pr["body"].replace(pr["body"][pr["body"].index("\n\n<details><summary>providers"):pr["body"].index("\n\n---\nFooter")], ""))
        self.assertEqual(readiness.verify_release(single, {".": "0.1.0"}, {".": "0.2.0"}, config), ["."])
        separate = copy.deepcopy(config)
        separate["packages"]["providers/openai"]["separate-pull-requests"] = True
        for name, edit, manifest, settings, message in [
            ("title", {"title": "chore(main): release 0.2.0"}, after, config, "combined release title"),
            ("branch", {"head": dict(pr["head"], ref=pr["head"]["ref"] + "--components--providers/openai")}, after, config, "release branch"),
            ("missing component", {"body": single["body"]}, after, config, "does not match the manifest versions"),
            ("version", {"body": pr["body"].replace("providers/openai: 0.1.1", "providers/openai: 0.1.2")}, after, config, "summary"),
            ("extra text", {"body": pr["body"].replace("Header\n---\n\n", "Header\n---\n\ninjected\n\n")}, after, config, "only component release sections"),
            ("unknown component", {"body": pr["body"].replace("providers/openai: ", "providers/other: ")}, after, config, "unknown or repeated"),
            ("gateway", {}, dict(after, **{"ai-gateway": "0.2.0"}), config, "cannot share a release pull request"),
            ("separate library", {}, after, separate, "cannot share a release pull request"),
        ]:
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, message):
                readiness.verify_release(dict(pr, **edit), before, manifest, settings)

    def test_combined_release_files_are_manifest_and_released_changelogs(self):
        components = [".", "providers/openai"]
        readiness.verify_release_files(components, [readiness.MANIFEST, "CHANGELOG.md", "providers/openai/CHANGELOG.md"])
        readiness.verify_release_files(components, [readiness.MANIFEST, "CHANGELOG.md", "providers/bedrock/go.mod"])
        for path in ["providers/bedrock/CHANGELOG.md", "providers/openai/go.sum", "go.sum", "ai-gateway/package.json"]:
            with self.subTest(path=path), self.assertRaises(ValueError):
                readiness.verify_release_files(components, [readiness.MANIFEST, path])

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
                pr = self.pr if release else {"user": {"id": 1}, "base": {"sha": "base"}, "head": {"ref": "fix", "sha": "head"}, "title": "fix: bug"}
                Path("pr.json").write_text(json.dumps(pr))
                Path("evidence.json").write_text("stale evidence")
                with patch("sys.argv", ["readiness", "--pull-request", "pr.json", "--evidence", "evidence.json"]), \
                     patch.object(readiness, "tree", return_value={readiness.MANIFEST: "blob"}), \
                     patch.object(readiness, "run", side_effect=["base head", '{".": "0.1.0"}', json.dumps(self.config)]), \
                     patch.object(readiness, "verify_requirements") as library:
                    if release:
                        with self.assertRaisesRegex(ValueError, "release must change component versions"):
                            readiness.main()
                    else:
                        readiness.main()
                    library.assert_not_called()
                    self.assertFalse(Path("evidence.json").exists())

    def test_candidate_base_requires_exact_merge_head(self):
        for parents in ["", "base", "base old-head", "base head extra"]:
            with self.subTest(parents=parents), patch.object(readiness, "run", return_value=parents):
                with self.assertRaisesRegex(ValueError, "current PR head"):
                    readiness.candidate_base(self.pr)
        with patch.object(readiness, "run", return_value="new-base head"):
            self.assertEqual(readiness.candidate_base(self.pr), "new-base")

    def test_stale_api_base_classifies_only_changes_from_checkout_base(self):
        for manifest_change in [False, True]:
            with self.subTest(manifest_change=manifest_change), tempfile.TemporaryDirectory() as directory:
                repo = Path(directory, "repo")
                repo.mkdir()
                git = self.git_repository(repo)
                with chdir(repo):
                    Path(readiness.MANIFEST).write_text('{".": "0.1.0"}')
                    Path("release-please-config.json").write_text(json.dumps(self.config))
                    git("add", ".")
                    git("commit", "-qm", "old base")
                    old_base = git("rev-parse", "HEAD")
                    git("checkout", "-qb", "feature")
                    Path("core.go").write_text("package core\n")
                    if manifest_change:
                        Path(readiness.MANIFEST).write_text('{".": "0.1.0", "providers/openai": "0.1.0"}')
                    git("add", ".")
                    git("commit", "-qm", "feature")
                    head = git("rev-parse", "HEAD")
                    git("checkout", "--detach", old_base)
                    Path(readiness.MANIFEST).write_text('{".": "0.2.0"}')
                    git("add", ".")
                    git("commit", "-qm", "intervening release")
                    if manifest_change:
                        git("merge", "--no-ff", "--no-commit", "-s", "ours", head)
                        Path(readiness.MANIFEST).write_text('{".": "0.2.0", "providers/openai": "0.1.0"}')
                        git("add", ".")
                        git("commit", "-qm", "merge")
                    else:
                        git("merge", "--no-ff", "-m", "merge", head)
                    pr = copy.deepcopy(self.pr)
                    pr["base"]["sha"], pr["head"]["sha"] = old_base, head
                    pr["user"] = {"login": "contributor", "type": "User", "id": 1}
                    pr["head"]["ref"], pr["title"] = "fix", "fix: bug"
                    metadata = Path(directory, "pr.json")
                    metadata.write_text(json.dumps(pr))
                    evidence = Path(directory, "evidence.json")
                    evidence.write_text("stale evidence")
                    with patch("sys.argv", ["readiness", "--pull-request", str(metadata), "--evidence", str(evidence)]):
                        if manifest_change:
                            with self.assertRaisesRegex(ValueError, "dedicated release App"):
                                readiness.main()
                        else:
                            readiness.main()
                            pr["title"] = "chore: release main"
                            metadata.write_text(json.dumps(pr))
                            with self.assertRaisesRegex(ValueError, "release must change component versions"):
                                readiness.main()
                    self.assertFalse(evidence.exists())

    def test_generated_changelog_preserves_history(self):
        notes = "## 0.2.0 (2026-10-20)\n\n* Fix"
        old = "# Changelog\n\n## 0.1.0\n\n* Old\n"
        new = "# Changelog\n\n" + notes + "\n\n## 0.1.0\n\n* Old\n"
        readiness.verify_changelog(old, new, notes)
        for invalid in [new.replace("* Old", "* Rewritten"), new.replace("0.2.0", "9.0.0"), ""]:
            with self.assertRaises(ValueError):
                readiness.verify_changelog(old, invalid, notes)

    def test_release_cannot_smuggle_dependency_or_workflow_edits(self):
        readiness.verify_release_files(["providers/openai"], [readiness.MANIFEST, "providers/openai/CHANGELOG.md"])
        readiness.verify_release_files(["providers/openai"], [readiness.MANIFEST, "providers/openai/go.mod"])
        for path in ["providers/openai/go.sum", ".github/workflows/release-please.yml", "core.go"]:
            with self.subTest(path=path), self.assertRaises(ValueError):
                readiness.verify_release_files(["providers/openai"], [readiness.MANIFEST, path])

    def test_gateway_release_may_update_the_plugin_version(self):
        readiness.verify_release_files(["ai-gateway"], [readiness.MANIFEST, "ai-gateway/CHANGELOG.md", "ai-gateway/package.json"])
        with self.assertRaises(ValueError):
            readiness.verify_release_files(["providers/openai"], [readiness.MANIFEST, "ai-gateway/package.json"])
        with self.assertRaises(ValueError):
            readiness.verify_release_files(["ai-gateway"], [readiness.MANIFEST, "ai-gateway/go.sum"])

    def test_generated_release_files_cannot_change_file_kind(self):
        files = [readiness.MANIFEST, "CHANGELOG.md"]
        for mode in ["120000", "100755", "160000"]:
            with self.subTest(mode=mode), self.assertRaises(ValueError):
                readiness.verify_release_files(["."], files, {
                    readiness.MANIFEST: "100644 blob manifest", "CHANGELOG.md": f"{mode} blob notes"})

    def test_gateway_is_an_application_not_a_standalone_library(self):
        with self.assertRaisesRegex(ValueError, "Gateway application"):
            readiness.verify_requirements("ai-gateway", ["ai-gateway"], {}, "base")

    def git_repository(self, directory):
        def git(*args):
            return subprocess.check_output(["git", "-c", "commit.gpgsign=false", *args], cwd=directory, text=True, stderr=subprocess.PIPE).strip()
        git("init", "-q")
        git("config", "user.email", "test@example.com")
        git("config", "user.name", "Test")
        return git

    def test_requirement_updates_follow_the_manifest(self):
        module = ("module {prefix}/providers/openai\n\ngo 1.27.3\n\nrequire (\n\t{prefix} {version}\n\texample.com/x v1.0.0\n)\n\n"
                  "replace {prefix} => ../..\n")
        versions = {readiness.PREFIX: "v0.2.0"}
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            git = self.git_repository(directory)
            Path("providers/openai").mkdir(parents=True)
            path = Path("providers/openai/go.mod")
            def commit(content):
                path.write_text(content)
                git("add", ".")
                git("commit", "-qm", "change", "--allow-empty")
                return git("rev-parse", "HEAD")
            base = commit(module.format(prefix=readiness.PREFIX, version="v0.1.0"))
            updated = commit(module.format(prefix=readiness.PREFIX, version="v0.2.0"))
            readiness.verify_requirement_updates(base, updated, [str(path), "CHANGELOG.md"], versions)
            for name, content in [
                ("other version", module.format(prefix=readiness.PREFIX, version="v0.3.0")),
                ("third-party requirement", module.format(prefix=readiness.PREFIX, version="v0.2.0").replace("v1.0.0", "v1.1.0")),
                ("go directive", module.format(prefix=readiness.PREFIX, version="v0.2.0").replace("go 1.27.3", "go 1.27.4")),
                ("replacement", module.format(prefix=readiness.PREFIX, version="v0.2.0").replace("=> ../..", "=> ../other")),
            ]:
                with self.subTest(name=name), self.assertRaisesRegex(ValueError, "only update first-party requirements"):
                    readiness.verify_requirement_updates(base, commit(content), [str(path)], versions)

    def test_released_library_requirements_are_tagged_or_co_released(self):
        versions = {readiness.PREFIX: "v0.2.0", readiness.PREFIX + "/providers/openai": "v0.1.1"}
        module = ("module {prefix}/providers/bedrock\n\ngo 1.27.3\n\nrequire (\n\t{prefix} v0.2.0\n"
                  "\t{prefix}/providers/openai {openai}\n\texample.com/x v1.0.0\n)\n")
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            Path("providers/bedrock").mkdir(parents=True)
            path = Path("providers/bedrock/go.mod")
            path.write_text(module.format(prefix=readiness.PREFIX, openai="v0.1.1"))
            components = ["providers/bedrock", "providers/openai"]
            with patch.object(readiness, "verify_tagged") as tagged:
                self.assertEqual(readiness.verify_requirements("providers/bedrock", components, versions, "base"), [
                    {"path": readiness.PREFIX, "version": "v0.2.0", "coReleased": False},
                    {"path": readiness.PREFIX + "/providers/openai", "version": "v0.1.1", "coReleased": True}])
                tagged.assert_called_once_with(".", "v0.2.0", "base")
            with patch.object(readiness, "verify_tagged", side_effect=subprocess.CalledProcessError(1, "fetch")), \
                 self.assertRaisesRegex(ValueError, "neither a canonical tag nor released"):
                readiness.verify_requirements("providers/bedrock", components, versions, "base")
            path.write_text(module.format(prefix=readiness.PREFIX, openai="v0.1.0"))
            with patch.object(readiness, "verify_tagged"), self.assertRaisesRegex(ValueError, "expected v0.1.1"):
                readiness.verify_requirements("providers/bedrock", components, versions, "base")

    def test_expected_versions_use_manifest_then_initial_version(self):
        config = {"initial-version": "0.0.1", "packages": {".": {}, "providers/bedrock": {"initial-version": "0.1.0-alpha.1"}, "middleware/new": {}}}
        self.assertEqual(readiness.expected_versions({".": "0.2.0"}, config), {
            readiness.PREFIX: "v0.2.0", readiness.PREFIX + "/providers/bedrock": "v0.1.0-alpha.1",
            readiness.PREFIX + "/middleware/new": "v0.0.1"})

    def test_real_git_current_merge(self):
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            def git(*args):
                return subprocess.check_output(["git", "-c", "commit.gpgsign=false", *args], text=True, stderr=subprocess.PIPE).strip()
            git("init", "-q")
            git("config", "user.email", "test@example.com")
            git("config", "user.name", "Test")
            Path("core.go").write_text("package core\n")
            git("add", ".")
            git("commit", "-qm", "base")
            base = git("rev-parse", "HEAD")
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
            Path("core.go").write_text("package core\n\nconst fix = true\n")
            git("add", ".")
            git("commit", "-qm", "fix")
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
                Path(".github/workflows").mkdir(parents=True)
                Path(".github/workflows/release-readiness.yml").write_text("name: readiness\n")
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
                evidence = Path(directory, "library-readiness.json")
                with patch("sys.argv", ["readiness", "--pull-request", str(metadata), "--evidence", str(evidence)]), \
                     patch.object(readiness, "run", side_effect=command), \
                     patch.object(readiness, "verify_requirements") as library:
                    readiness.main()
                    library.assert_called_once_with(".", ["."], readiness.expected_versions({".": "0.2.0"}, self.config), base)
                    record = json.loads(evidence.read_text())
                    self.assertEqual(record["schemaVersion"], 2)
                    self.assertEqual(record["headSha"], head)
                    self.assertEqual(record["baseSha"], base)
                    self.assertEqual(record["mergeSha"], git("rev-parse", "HEAD"))
                    self.assertEqual(record["treeSha"], git("rev-parse", "HEAD^{tree}"))
                    self.assertEqual(record["toolingRevision"], base)
                    self.assertEqual(record["bodyDigest"], hashlib.sha256(pr["body"].encode()).hexdigest())
                    self.assertEqual(record["components"], {".": "0.2.0"})
                    pr["base"]["sha"] = head
                    metadata.write_text(json.dumps(pr))
                    with self.assertRaisesRegex(ValueError, "release PR base is stale"):
                        readiness.main()
                    self.assertFalse(evidence.exists())
                    pr["base"]["sha"] = base
                    metadata.write_text(json.dumps(pr))
                    for failure in ["library", "live"]:
                        with self.subTest(failure=failure):
                            target = "verify_requirements" if failure == "library" else "verify_live_candidate"
                            with patch.object(readiness, target, side_effect=ValueError("failed")):
                                with self.assertRaisesRegex(ValueError, "failed"):
                                    readiness.main()
                            self.assertFalse(evidence.exists())

    def test_release_entry_point_validates_combined_candidate(self):
        config, pr, before, after, notes = self.combined_release()
        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory, "repo")
            repo.mkdir()
            with chdir(repo):
                def git(*args):
                    return subprocess.check_output(["git", "-c", "commit.gpgsign=false", *args], text=True, stderr=subprocess.PIPE).strip()
                git("init", "-q")
                git("config", "user.email", "test@example.com")
                git("config", "user.name", "Test")
                Path(readiness.MANIFEST).write_text(json.dumps(before))
                Path("release-please-config.json").write_text(json.dumps(config))
                Path(".github/workflows").mkdir(parents=True)
                Path(".github/workflows/release-readiness.yml").write_text("name: readiness\n")
                git("add", ".")
                git("commit", "-qm", "base")
                base = git("rev-parse", "HEAD")
                git("checkout", "-qb", "candidate")
                Path(readiness.MANIFEST).write_text(json.dumps(after))
                Path("CHANGELOG.md").write_text("# Changelog\n\n" + notes["."] + "\n")
                Path("providers/openai").mkdir(parents=True)
                Path("providers/openai/CHANGELOG.md").write_text("# Changelog\n\n" + notes["providers/openai"] + "\n")
                git("add", ".")
                git("commit", "-qm", "release")
                head = git("rev-parse", "HEAD")
                git("checkout", "--detach", base)
                git("merge", "--no-ff", "-m", "merge", head)
                pr["number"] = 124
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
                evidence = Path(directory, "library-readiness.json")
                with patch("sys.argv", ["readiness", "--pull-request", str(metadata), "--evidence", str(evidence)]), \
                     patch.object(readiness, "run", side_effect=command), \
                     patch.object(readiness, "verify_requirements") as library:
                    readiness.main()
                    versions = readiness.expected_versions(after, config)
                    self.assertEqual([call.args for call in library.call_args_list],
                                     [(".", [".", "providers/openai"], versions, base), ("providers/openai", [".", "providers/openai"], versions, base)])
                    self.assertEqual(json.loads(evidence.read_text())["components"], after)
                    with patch.object(readiness, "verify_requirements", side_effect=[None, ValueError("provider failed")]):
                        with self.assertRaisesRegex(ValueError, "provider failed"):
                            readiness.main()
                    self.assertFalse(evidence.exists())

    def test_consumer_scenarios_cover_registered_libraries(self):
        from release_consumer import SCENARIOS
        repo = Path(__file__).resolve().parent.parent
        config = json.loads((repo / "release-please-config.json").read_text())
        self.assertEqual(set(SCENARIOS), set(config["packages"]) - {".", "ai-gateway"})
        for root, scenario in SCENARIOS.items():
            with self.subTest(root=root):
                tests = "\n".join(path.read_text() for path in (repo / root).glob("*_test.go"))
                self.assertIn(f"func {scenario}(t *testing.T)", tests)

    def test_consumer_requires_the_published_version_without_replacements(self):
        from release_consumer import verify_graph
        path = readiness.PREFIX + "/providers/openai"
        with tempfile.TemporaryDirectory() as directory:
            candidate = {"Path": path, "Version": "v0.1.1", "Dir": directory + "/candidate"}
            root = {"Path": readiness.PREFIX, "Version": "v0.2.0"}
            verify_graph([candidate, root], path, "v0.1.1", Path(directory))
            for graph in [[dict(candidate, Main=True), root],
                          [dict(candidate, Dir="/outside"), root],
                          [dict(candidate, Version="v0.1.0"), root],
                          [candidate, dict(root, Replace={"Path": "../.."})]]:
                with self.assertRaises(ValueError):
                    verify_graph(graph, path, "v0.1.1", Path(directory))

    def test_consumer_parses_only_library_release_tags(self):
        from release_consumer import parse_tag
        self.assertEqual(parse_tag("v0.1.0-alpha.4"), (".", "v0.1.0-alpha.4"))
        self.assertEqual(parse_tag("providers/openai/v0.1.0-alpha.2"), ("providers/openai", "v0.1.0-alpha.2"))
        self.assertEqual(parse_tag("middleware/logger/v1.2.3"), ("middleware/logger", "v1.2.3"))
        for tag in ["ai-gateway/v0.1.0", "providers/openai/0.1.0", "providers/../v1.0.0", "v1.0.0; rm -rf /", ""]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                parse_tag(tag)

    def test_consumer_waits_for_the_version_and_its_first_party_requirements(self):
        import release_consumer
        openai = readiness.PREFIX + "/providers/openai"
        with tempfile.TemporaryDirectory() as directory:
            mod = Path(directory, "bedrock.mod")
            mod.write_text(f"module {readiness.PREFIX}/providers/bedrock\n\ngo 1.27.3\n\nrequire (\n"
                           f"\t{readiness.PREFIX} v0.1.0-alpha.4\n\t{openai} v0.1.0-alpha.2\n\texample.com/x v1.0.0\n)\n")
            available = set()
            requested = []
            real_run = subprocess.run
            def download(args, **kwargs):
                if list(args[:3]) != ["go", "mod", "download"]:
                    return real_run(args, **kwargs)
                module = args[-1]
                requested.append(module)
                if module.startswith(openai) and openai not in available:
                    return subprocess.CompletedProcess(args, 1, json.dumps({"Error": "unknown revision providers/openai/v0.1.0-alpha.2"}), "")
                return subprocess.CompletedProcess(args, 0, json.dumps({"GoMod": str(mod)}), "")
            now = [0.0]
            def sleep(seconds):
                now[0] += seconds
                if now[0] >= 60:
                    available.add(openai)
            with patch.object(release_consumer.subprocess, "run", side_effect=download):
                release_consumer.wait_until_available(readiness.PREFIX + "/providers/bedrock", "v0.1.0-alpha.1", 5,
                                                      sleep=sleep, clock=lambda: now[0])
                self.assertIn(f"{openai}@v0.1.0-alpha.2", requested)
                self.assertNotIn("example.com/x@v1.0.0", requested)
                self.assertEqual(now[0], 60)
                available.clear()
                now[0] = 0.0
                with self.assertRaisesRegex(ValueError, "after 1 minutes: .*providers/openai@v0.1.0-alpha.2: unknown revision"):
                    release_consumer.wait_until_available(readiness.PREFIX + "/providers/bedrock", "v0.1.0-alpha.1", 1,
                                                          sleep=lambda seconds: now.__setitem__(0, now[0] + seconds),
                                                          clock=lambda: now[0])

    def test_initial_adoption_requires_disabled_automation(self):
        root = Path(__file__).resolve().parent.parent
        workflow = """  release-please:
    if: ${{ false }}
        uses: googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7
        with:
          skip-github-release: true
"""
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

    def test_readiness_evidence_upload_is_success_only_and_attempt_scoped(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/release-readiness.yml").read_text()
        upload = workflow.split("      - name: Retain validated library candidate evidence", 1)[1]
        self.assertNotIn("always()", upload)
        self.assertNotIn("continue-on-error", workflow)
        self.assertIn("library-readiness-${{ github.run_id }}-${{ github.run_attempt }}", upload)
        self.assertIn("retention-days: 90", upload)
        self.assertIn("path: ${{ runner.temp }}/library-readiness.json", upload)
        self.assertNotIn("contents: write", workflow)
        with patch.dict("os.environ", {"GH_TOKEN": "do-not-retain", "GITHUB_RUN_ID": "123", "GITHUB_RUN_ATTEMPT": "2"}), \
             patch.object(readiness, "run", return_value="a" * 40):
            record = readiness.readiness_record(dict(self.pr, number=123), {".": "0.2.0"})
        self.assertEqual(record["runId"], "123")
        self.assertEqual(record["runAttempt"], "2")
        self.assertNotIn("do-not-retain", json.dumps(record))

    def test_library_activation_keeps_workflow_permissions_and_mode_explicit(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/release-please.yml").read_text()
        for expected in ["github.event_name == 'push'", "github.ref == 'refs/heads/main'",
                         "github.repository == 'grafana/ai-sdk'", "vars.RELEASE_LIBRARY_MODE == 'root'",
                         "vars.RELEASE_LIBRARY_MODE == 'libraries'", "cancel-in-progress: false",
                         "GH_READ_TOKEN: ${{ github.token }}", "persist-credentials: false",
                         "checks: read", "actions: read", "node release/tooling/publish.mjs"]:
            self.assertIn(expected, workflow)
        self.assertNotIn("contents: write", workflow)
        self.assertNotIn("token-smoke-test:", workflow)
        self.assertNotIn("app-pr-test:", workflow)


    def test_release_verification_workflow_is_read_only_and_tag_scoped(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/release-verification.yml").read_text()
        for expected in ["types: [published]", "workflow_dispatch:", "permissions: {}", "contents: read",
                         "persist-credentials: false", "cancel-in-progress: false",
                         "!startsWith(github.event.release.tag_name || inputs.tag, 'ai-gateway/')",
                         'python3 scripts/release_consumer.py "$RELEASE_TAG" --wait-minutes 40']:
            self.assertIn(expected, workflow)
        for forbidden in ["write", "secrets.", "GH_TOKEN", "run: python3 scripts/release_consumer.py ${{"]:
            self.assertNotIn(forbidden, workflow)
        self.assertNotIn("release_consumer", (root / ".github/workflows/release-please.yml").read_text())

if __name__ == "__main__":
    unittest.main()
