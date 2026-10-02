import copy
import json
from pathlib import Path
import unittest
from unittest.mock import patch

import release_readiness as readiness


class ReleaseReadinessTest(unittest.TestCase):
    def setUp(self):
        self.config = {"packages": {".": {}, "providers/openai": {"component": "providers/openai"}, "ai-gateway": {"component": "ai-gateway"}}}
        self.pr = {
            "user": {"login": readiness.RELEASE_APP, "type": "Bot"},
            "base": {"ref": "main", "sha": "base", "repo": {"full_name": "grafana/ai-sdk"}},
            "head": {"ref": "release-please--branches--main",
                     "sha": "head", "repo": {"full_name": "grafana/ai-sdk"}},
        }

    def test_identifies_one_component_without_labels(self):
        self.assertEqual(readiness.verify_release(self.pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config), ".")
        with self.assertRaises(ValueError):
            readiness.changed_component({".": "0.1.0"}, {".": "0.2.0", "providers/openai": "0.1.0"})
        with self.assertRaises(ValueError):
            readiness.changed_component({".": "0.1.0"}, {})
        self.pr["head"]["ref"] += "--components--providers/openai"
        self.assertEqual(readiness.verify_release(self.pr, {}, {"providers/openai": "0.1.0"}, self.config), "providers/openai")

    def test_rejects_spoofed_release_metadata(self):
        for position, field, value in [
            ("user", "login", "contributor"), ("user", "type", "User"),
            ("base", "ref", "development"), ("head", "ref", "ordinary-branch"),
            ("head", "repo", {"full_name": "fork/ai-sdk"}),
        ]:
            with self.subTest(field=field, value=value):
                pr = copy.deepcopy(self.pr)
                pr[position][field] = value
                with self.assertRaises(ValueError):
                    readiness.verify_release(pr, {".": "0.1.0"}, {".": "0.2.0"}, self.config)

    def test_candidate_requires_current_base_and_exact_head(self):
        with patch.object(readiness, "run", return_value="base head"):
            readiness.verify_candidate(self.pr, "base", "HEAD")
            with self.assertRaises(ValueError):
                readiness.verify_candidate(self.pr, "new-base", "HEAD")
        with patch.object(readiness, "run", return_value="base older-head"):
            with self.assertRaises(ValueError):
                readiness.verify_candidate(self.pr, "base", "HEAD")

    def test_module_ownership_preserves_root_middleware(self):
        roots = {".", "middleware/logger", "providers/openai", "ai-gateway"}
        files = {"middleware/extract_json.go": "root", "middleware/logger/stream.go": "logger",
                 "providers/openai/model.go": "provider", "docs/guide.md": "docs",
                 "core_test.go": "test", "go.mod": "manifest", "templates/prompt.txt": "asset",
                 ".release-please-manifest.json": "release-state"}
        self.assertEqual(readiness.owned_snapshot(files, ".", roots), {
            "middleware/extract_json.go": "root", "go.mod": "manifest", "templates/prompt.txt": "asset"})
        self.assertEqual(readiness.owned_snapshot(files, "middleware/logger", roots), {"stream.go": "logger"})

    def test_release_cannot_smuggle_dependency_or_workflow_edits(self):
        readiness.verify_release_files("providers/openai", [readiness.MANIFEST, "providers/openai/CHANGELOG.md"])
        for path in ["providers/openai/go.mod", ".github/workflows/release-please.yml", "core.go"]:
            with self.subTest(path=path), self.assertRaises(ValueError):
                readiness.verify_release_files("providers/openai", [readiness.MANIFEST, path])

    def test_gateway_is_an_application_not_a_standalone_library(self):
        with self.assertRaisesRegex(ValueError, "Gateway application"):
            readiness.verify_library("ai-gateway", "base", self.config["packages"])

    def test_root_readiness_does_not_test_downstream_modules(self):
        graph = json.dumps({"Path": readiness.PREFIX, "Main": True})
        with patch.object(readiness, "run", return_value=graph), \
             patch.object(readiness, "tree", return_value={}), \
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
        ]:
            with self.subTest(name=name):
                dependency["Version"] = version
                metadata["Version"] = version
                metadata["Origin"]["URL"] = url
                def command(*args, **kwargs):
                    return json.dumps(metadata if args[1] == "mod" else dependency)
                def snapshot(ref):
                    return {"core.go": "different" if stale and ref == "base" else "same"}
                with patch.object(readiness, "run", side_effect=command), \
                     patch.object(readiness, "tree", side_effect=snapshot), \
                     patch.object(readiness.subprocess, "run") as execute:
                    if accepted:
                        readiness.verify_library("providers/openai", "base", self.config["packages"])
                        self.assertEqual(execute.call_count, 2)
                    else:
                        with self.assertRaises(ValueError):
                            readiness.verify_library("providers/openai", "base", self.config["packages"])

    def test_release_automation_is_disabled_until_activation_review(self):
        root = Path(__file__).resolve().parent.parent
        workflow = (root / ".github/workflows/release-please.yml").read_text()
        self.assertTrue(readiness.release_automation_disabled(workflow))
        self.assertFalse(readiness.release_automation_disabled(workflow.replace(
            "  workflow_dispatch:", "  push:\n    branches: [main]",
        )))
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


if __name__ == "__main__":
    unittest.main()
