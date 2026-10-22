import copy
from contextlib import chdir
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import release_authorization as authorization
import release_readiness as readiness


class MergedAuthorizationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.previous = Path.cwd()
        os.chdir(self.directory)
        self.addCleanup(os.chdir, self.previous)
        self.git("init", "-q")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "user.name", "Test")
        self.config = {"packages": {".": {}, "providers/openai": {"component": "providers/openai"}}}
        Path(readiness.MANIFEST).write_text('{".": "0.1.0-alpha.1"}')
        Path("release-please-config.json").write_text(json.dumps(self.config))
        Path(".github/workflows").mkdir(parents=True)
        Path(".github/workflows/release-readiness.yml").write_text("name: readiness\n")
        Path("core.go").write_text("package aisdk\n")
        self.git("add", ".")
        self.git("commit", "-qm", "base")
        self.base = self.git("rev-parse", "HEAD")
        self.git("checkout", "-qb", "release")
        Path(readiness.MANIFEST).write_text('{".": "0.1.0-alpha.2"}')
        self.notes = "## 0.1.0-alpha.2 (2026-10-22)\n\n* Fix"
        Path("CHANGELOG.md").write_text("# Changelog\n\n" + self.notes + "\n")
        self.git("add", ".")
        self.git("commit", "-qm", "candidate")
        self.head = self.git("rev-parse", "HEAD")
        self.git("checkout", "--detach", self.base)
        self.git("merge", "--no-ff", "-m", "test merge", self.head)
        repo = {"id": readiness.REPOSITORY_ID, "full_name": "grafana/ai-sdk"}
        self.pr = {"number": 10, "state": "open", "draft": False,
                   "title": "chore(main): release 0.1.0-alpha.2",
                   "body": f"Header\n---\n\n{self.notes}\n\n---\nFooter",
                   "user": {"id": readiness.RELEASE_BOT_ID, "login": readiness.RELEASE_APP, "type": "Bot"},
                   "head": {"sha": self.head, "ref": "release-please--branches--main", "repo": repo},
                   "base": {"sha": self.base, "ref": "main", "repo": repo}}
        with patch.dict(os.environ, {"GITHUB_RUN_ID": "200", "GITHUB_RUN_ATTEMPT": "1"}):
            self.record = readiness.readiness_record(self.pr, ".", "0.1.0-alpha.2")
        self.git("checkout", "--detach", self.base)
        self.git("merge", "--squash", self.head)
        self.git("commit", "-qm", "squash")
        self.merge = self.git("rev-parse", "HEAD")
        self.pr.update(state="closed", merged=True, merge_commit_sha=self.merge)
        self.candidate = {"mergeSha": self.merge, "parents": [self.base], "treeSha": self.record["treeSha"],
                          "component": ".", "version": "0.1.0-alpha.2", "tag": "v0.1.0-alpha.2",
                          "notes": self.notes, "config": self.config}

    def git(self, *args):
        return subprocess.check_output(["git", "-c", "commit.gpgsign=false", *args], text=True, stderr=subprocess.PIPE).strip()

    def archive(self, record=None, name="library-readiness.json"):
        data = io.BytesIO()
        with zipfile.ZipFile(data, "w") as archive:
            archive.writestr(name, json.dumps(self.record if record is None else record))
        return data.getvalue()

    def test_record_accepts_only_bounded_named_schema(self):
        self.assertEqual(authorization.read_record(self.archive()), self.record)
        for name in ["../library-readiness.json", "/library-readiness.json", "other.json"]:
            with self.subTest(name=name), self.assertRaises(ValueError):
                authorization.read_record(self.archive(name=name))
        for record in [dict(self.record, schemaVersion=2), dict(self.record, unknown=True),
                       dict(self.record, baseSha="main"), dict(self.record, runAttempt=None),
                       dict(self.record, bodyDigest="x" * 65536)]:
            with self.subTest(record=list(record)), self.assertRaises(ValueError):
                authorization.read_record(self.archive(record))
        with self.assertRaises(ValueError):
            authorization.read_record(bytes(1024 * 1024 + 1))

    def test_actual_squash_tree_matches_successful_pr_test_merge(self):
        self.assertNotEqual(self.record["mergeSha"], self.merge)
        component, parent, roots = authorization.verify_merged(self.candidate, self.pr, self.record)
        self.assertEqual((component, parent, roots), (".", self.base, set(self.config["packages"])))
        self.git("checkout", "-qb", "later-main")
        Path("later.md").write_text("later unrelated merge")
        self.git("add", ".")
        self.git("commit", "-qm", "later")
        self.git("checkout", "--detach", self.merge)
        authorization.verify_merged(self.candidate, self.pr, self.record)

    def test_mutable_metadata_and_unbound_evidence_are_rejected(self):
        for target, field, value in [
            ("pr", "title", "edited"), ("pr", "body", self.pr["body"] + "edited"),
            ("pr", "merged", False), ("pr", "state", "open"), ("pr", "draft", True),
            ("record", "baseSha", self.head), ("record", "treeSha", self.base),
            ("record", "headSha", self.base), ("record", "prNumber", 99),
            ("record", "toolingRevision", self.head), ("record", "workflowBlob", self.head),
            ("candidate", "tag", "v9.0.0"), ("candidate", "notes", "edited"),
            ("candidate", "version", "9.0.0"), ("candidate", "component", "providers/openai"),
        ]:
            data = {"pr": copy.deepcopy(self.pr), "record": copy.deepcopy(self.record), "candidate": copy.deepcopy(self.candidate)}
            data[target][field] = value
            with self.subTest(target=target, field=field), self.assertRaises(ValueError):
                authorization.verify_merged(data["candidate"], data["pr"], data["record"])
        pr = copy.deepcopy(self.pr)
        pr["user"]["id"] = 1
        with self.assertRaises(ValueError):
            authorization.verify_merged(self.candidate, pr, self.record)

    def test_dirty_source_and_extra_squash_edits_fail(self):
        Path("core.go").write_text("package aisdk\nfunc Injected() {}\n")
        with self.assertRaisesRegex(ValueError, "clean"):
            authorization.verify_merged(self.candidate, self.pr, self.record)
        self.git("add", ".")
        self.git("commit", "--amend", "-qm", "altered squash")
        self.candidate["mergeSha"] = self.pr["merge_commit_sha"] = self.git("rev-parse", "HEAD")
        self.candidate["treeSha"] = self.git("rev-parse", "HEAD^{tree}")
        with self.assertRaisesRegex(ValueError, "treeSha|commit identity"):
            authorization.verify_merged(self.candidate, self.pr, self.record)
        self.record["treeSha"] = self.candidate["treeSha"]
        with self.assertRaisesRegex(ValueError, "reconstructed"):
            authorization.verify_merged(self.candidate, self.pr, self.record)

    def test_merge_commit_topology_is_not_accepted_as_a_squash(self):
        self.git("checkout", "--detach", self.record["mergeSha"])
        self.candidate["mergeSha"] = self.pr["merge_commit_sha"] = self.record["mergeSha"]
        with self.assertRaisesRegex(ValueError, "squash parent"):
            authorization.verify_merged(self.candidate, self.pr, self.record)

    def test_trusted_policy_path_cannot_be_replaced_by_candidate(self):
        trusted = Path(__file__).resolve().parent.parent
        graph = json.dumps({"Path": readiness.PREFIX, "Main": True})
        with patch.object(readiness, "run", return_value=graph), \
             patch("release_consumer.verify_consumer") as consumer, \
             patch.object(readiness.subprocess, "run") as execute:
            readiness.verify_library(".", self.base, set(self.config["packages"]), tooling_root=trusted)
        self.assertEqual(execute.call_args.args[0], ["bash", str(trusted / "scripts/module-policy.sh"), "standalone", "."])
        self.assertEqual(consumer.call_args.kwargs["tooling_root"], trusted)


class SelectedRootIntegrationTest(unittest.TestCase):
    @unittest.skipUnless(os.environ.get("RELEASE_AUTHORIZATION_INTEGRATION") == "1", "explicit clean-cache root consumer integration")
    def test_root_consumer_passes_with_unready_downstream_source(self):
        trusted = Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory() as directory:
            checkout = Path(directory, "candidate")
            subprocess.run(["git", "worktree", "add", "--detach", str(checkout), "HEAD"], cwd=trusted, check=True, capture_output=True)
            try:
                with chdir(checkout):
                    for root in ["providers/openai", "providers/bedrock"]:
                        Path(root, "broken_authorization_test.go").write_text("package broken\nfunc Broken(\n")
                    subprocess.run(["git", "add", "."], check=True)
                    subprocess.run(["git", "-c", "commit.gpgsign=false", "-c", "user.name=Test", "-c", "user.email=test@example.com",
                                    "commit", "-qm", "broken downstream fixture"], check=True)
                    parent = readiness.run("git", "rev-parse", "HEAD")
                    config = json.loads(Path("release-please-config.json").read_text())
                    with patch.object(readiness.subprocess, "run", wraps=subprocess.run) as execute:
                        readiness.verify_library(".", parent, set(config["packages"]), tooling_root=trusted)
                    selections = [call.args[0][-1] for call in execute.call_args_list if "standalone" in call.args[0]]
                    self.assertEqual(selections, ["."])
                    readiness.verify_clean_checkout()
            finally:
                subprocess.run(["git", "worktree", "remove", "--force", str(checkout)], cwd=trusted, check=True, capture_output=True)


if __name__ == "__main__":
    unittest.main()
