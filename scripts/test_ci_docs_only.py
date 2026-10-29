import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import ci_docs_only


class DocsOnlyTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.old = Path.cwd()
        os.chdir(self.root)
        self.addCleanup(os.chdir, self.old)
        self.git('init', '-q')
        self.git('config', 'user.email', 'test@example.invalid')
        self.git('config', 'user.name', 'Test')
        self.git('config', 'commit.gpgsign', 'false')
        self.write('README.md')
        self.write('docs/guides/tools.md')
        self.write('main.go')
        self.base = self.commit()

    def git(self, *args):
        return subprocess.check_output(['git', *args], stderr=subprocess.DEVNULL).decode().strip()

    def write(self, path, text='initial\n'):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text)

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')
        return self.git('rev-parse', 'HEAD')

    def classify(self):
        return ci_docs_only.docs_only(self.base, self.commit())

    def test_known_prose(self):
        self.write('README.md', 'updated\n')
        self.write('docs/README.md')
        self.assertTrue(self.classify())

    def test_delete_prose(self):
        (self.root / 'docs/guides/tools.md').unlink()
        self.assertTrue(self.classify())

    def test_rename_between_prose(self):
        self.git('mv', 'docs/guides/tools.md', 'docs/guides/retry-and-timeout.md')
        self.assertTrue(self.classify())

    def test_rename_source_to_prose(self):
        self.git('mv', 'main.go', 'docs/README.md')
        self.assertFalse(self.classify())

    def test_unknown_paths_and_mixed_changes(self):
        for path in ['main.go', 'mise.toml', '.github/workflows/ci.yml', 'docs/generate.py',
                     'docs/fixtures/example.md', 'docs/guides/new-page.md', 'test/fixture.md',
                     'examples/README.md', 'README.md\nmain.go']:
            with self.subTest(path=path):
                self.git('reset', '--hard', self.base)
                self.write('README.md', 'updated\n')
                self.write(path, 'changed\n')
                self.assertFalse(self.classify())

    def test_executable_and_symlink(self):
        (self.root / 'README.md').chmod(0o755)
        self.assertFalse(self.classify())
        self.git('reset', '--hard', self.base)
        (self.root / 'README.md').unlink()
        (self.root / 'README.md').symlink_to('main.go')
        self.assertFalse(self.classify())

    def test_many_files_cannot_hide_source(self):
        for number in range(3200):
            self.write(f'docs/guides/page-{number}.md')
        self.write('z.go')
        self.assertFalse(self.classify())

    def test_empty_diff_and_invalid_revisions(self):
        self.assertFalse(ci_docs_only.docs_only(self.base, self.base))
        self.assertFalse(ci_docs_only.docs_only('', self.base))
        self.assertFalse(ci_docs_only.docs_only('--help', self.base))

    def test_output_fails_closed_and_push_never_classifies(self):
        output = self.root / 'output'
        for event, error in [('push', None), ('pull_request', OSError()),
                             ('pull_request', subprocess.CalledProcessError(1, 'git')),
                             ('pull_request', UnicodeError())]:
            with self.subTest(event=event, error=error):
                output.write_text('')
                with patch.dict(os.environ, {'GITHUB_EVENT_NAME': event, 'GITHUB_OUTPUT': str(output)}):
                    with patch.object(ci_docs_only, 'docs_only', side_effect=error, return_value=True) as classifier:
                        ci_docs_only.main()
                        if event == 'push':
                            classifier.assert_not_called()
                self.assertEqual(output.read_text(), 'docs_only=false\n')


if __name__ == '__main__':
    unittest.main()
