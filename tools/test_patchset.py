"""Exercise replay in real Git checkouts and verify the Go runner's result handling."""

import copy
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import patchset

CLI = Path(__file__).with_name('patchset.py').resolve()


class PatchsetTests(unittest.TestCase):
    def setUp(self):
        work = CLI.parents[1] / '.work'
        work.mkdir(exist_ok=True)
        temporary = tempfile.TemporaryDirectory(prefix='tool-test-', dir=work)
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name) / 'catalog'
        self.target = Path(temporary.name) / 'application'
        self.root.mkdir()
        self.target.mkdir()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Patchset test')
        self.git('config', 'user.email', 'test@example.invalid')
        self.git('config', 'commit.gpgsign', 'false')
        self.write('file.txt', 'before\nbase\nafter\n')
        self.write('.gitignore', 'ignored.txt\n')
        self.write('example/fixture_test.go', 'package example\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'base')
        self.base = self.git('rev-parse', 'HEAD').decode().strip()
        self.manifest = {'format': 1, 'base': self.base, 'patches': []}
        self.first = self.add_patch('first', 'base', 'first')
        self.second = self.add_patch('second', 'first', 'second', ['first'])
        self.save()

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.target), *args], stderr=subprocess.PIPE)

    def write(self, path, text):
        destination = self.target / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(text, encoding='utf-8')

    def add_patch(self, name, before, after, dependencies=None):
        directory = self.root / 'patches' / name
        directory.mkdir(parents=True)
        (directory / 'README.md').write_text(f'# {name}\n')
        (directory / 'changes.patch').write_text(
            'diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n'
            f'@@ -1,3 +1,3 @@\n before\n-{before}\n+{after}\n after\n')
        entry = {'id': name, 'status': 'keep', 'depends_on': dependencies or [],
                 'patch': f'patches/{name}/changes.patch', 'paths': ['file.txt'],
                 'tests': f'patches/{name}/tests.json'}
        self.manifest['patches'].append(entry)
        self.set_checks(entry, [{'name': 'real command', 'tier': 'focused', 'cwd': '.',
                                'requires': ['file.txt'], 'runner': 'command',
                                'command': [sys.executable, '-c',
                                            "from pathlib import Path; assert Path('file.txt').is_file()"]}])
        return entry

    def set_checks(self, entry, checks):
        (self.root / entry['tests']).write_text(json.dumps({'checks': checks}))

    def save(self):
        (self.root / 'manifest.json').write_text(json.dumps(self.manifest))

    def cli(self, action, *args, target=None, env=None):
        return subprocess.run([sys.executable, str(CLI), '--root', str(self.root), action,
                               '--target', str(target or self.target), *args],
                              capture_output=True, text=True, env=env)

    def assert_clean(self):
        self.assertEqual(self.git('status', '--porcelain'), b'')
        self.assertEqual((self.target / 'file.txt').read_text(), 'before\nbase\nafter\n')

    def test_selection_includes_transitive_dependencies_once_in_manifest_order(self):
        third = self.add_patch('third', 'second', 'third', ['second'])
        chosen = patchset.select(self.manifest, ['third', 'first', 'third'])
        self.assertEqual([entry['id'] for entry in chosen], ['first', 'second', 'third'])
        third['status'] = 'local'
        self.assertEqual(patchset.select(self.manifest, statuses=['local']), chosen)
        with self.assertRaisesRegex(patchset.Failure, 'Unknown patch ids'):
            patchset.select(self.manifest, ['typo'])
        with self.assertRaisesRegex(patchset.Failure, 'Selection is empty'):
            patchset.select(self.manifest, statuses=['replaced'])

    def test_check_preserves_target_and_apply_stages_complete_selection(self):
        index = self.target / '.git/index'
        before = index.read_bytes()
        checked = self.cli('check', '--only', 'second')
        self.assertEqual(checked.returncode, 0, checked.stderr)
        self.assertEqual(index.read_bytes(), before)
        self.assert_clean()
        applied = self.cli('apply', '--only', 'second')
        self.assertEqual(applied.returncode, 0, applied.stderr)
        self.assertEqual(self.git('show', ':file.txt'), b'before\nsecond\nafter\n')
        self.assertEqual(self.git('diff'), b'')
        tree = self.git('write-tree').decode().strip()
        self.assertIn(tree, checked.stdout)
        self.assertIn(tree, applied.stdout)
        self.assertEqual(self.git('rev-parse', 'HEAD').decode().strip(), self.base)
        duplicate = self.cli('apply')
        self.assertNotEqual(duplicate.returncode, 0)
        self.assertIn('Repeated application is refused', duplicate.stderr)
        self.assertEqual(self.git('write-tree').decode().strip(), tree)

    def test_later_conflict_does_not_leave_earlier_changes(self):
        path = self.root / self.second['patch']
        path.write_text(path.read_text().replace('-first\n', '-not-the-base\n'))
        for action in ('check', 'apply'):
            with self.subTest(action=action):
                result = self.cli(action)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('Preflight stopped at second', result.stderr)
                self.assert_clean()

    def test_dirty_worktree_and_index_are_preserved(self):
        for path, stage in [('untracked.txt', False), ('file.txt', False), ('file.txt', True)]:
            with self.subTest(path=path, stage=stage):
                self.write(path, 'user work\n')
                if stage:
                    self.git('add', '--', path)
                index = self.git('diff', '--cached')
                result = self.cli('apply')
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('Target has changes', result.stderr)
                self.assertEqual((self.target / path).read_text(), 'user work\n')
                self.assertEqual(self.git('diff', '--cached'), index)
                if path == 'untracked.txt':
                    (self.target / path).unlink()

    def test_ignored_file_conflict_is_checked_before_any_real_write(self):
        self.write('ignored.txt', 'local content\n')
        (self.root / self.second['patch']).write_text(
            'diff --git a/ignored.txt b/ignored.txt\nnew file mode 100644\n'
            '--- /dev/null\n+++ b/ignored.txt\n@@ -0,0 +1 @@\n+generated content\n')
        self.second['paths'] = ['ignored.txt']
        self.save()
        for action in ('check', 'apply'):
            with self.subTest(action=action):
                result = self.cli(action)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('ignored.txt', result.stderr)
                self.assertEqual((self.target / 'ignored.txt').read_text(), 'local content\n')
                self.assert_clean()

    def test_wrong_base_and_nested_directory_are_refused(self):
        nested = self.cli('apply', target=self.target / 'example')
        self.assertNotEqual(nested.returncode, 0)
        self.assertIn('Git checkout root', nested.stderr)
        self.git('commit', '--allow-empty', '-qm', 'different base')
        head = self.git('rev-parse', 'HEAD')
        result = self.cli('apply')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('differs from fixed base', result.stderr)
        self.assertEqual(self.git('rev-parse', 'HEAD'), head)
        self.assert_clean()

    def test_inventory_mismatch_and_duplicate_ids_are_rejected(self):
        self.first['paths'] = ['other.txt']
        self.save()
        result = self.cli('apply')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Payload path inventory differs', result.stderr)
        self.assert_clean()
        self.manifest['patches'].append(copy.deepcopy(self.first))
        self.save()
        with self.assertRaisesRegex(patchset.Failure, 'Duplicate id'):
            patchset.load_manifest(self.root)

    def test_dependency_order_and_empty_contract_are_rejected(self):
        self.manifest['patches'].reverse()
        self.save()
        with self.assertRaisesRegex(patchset.Failure, 'out-of-order dependency'):
            patchset.load_manifest(self.root)
        self.manifest['patches'].reverse()
        self.save()
        self.set_checks(self.first, [])
        with self.assertRaisesRegex(patchset.Failure, 'Empty test contract'):
            patchset.load_manifest(self.root)

    def test_replacement_requires_evidence_and_an_ancestor_commit(self):
        self.first.update(status='upstream', patch=None, paths=[],
                          replacement={'commit': self.base, 'evidence': 'reviewed base helper'})
        self.save()
        result = self.cli('check', '--only', 'first')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('SKIP first', result.stdout)
        self.first['replacement']['evidence'] = ''
        self.save()
        with self.assertRaisesRegex(patchset.Failure, 'Replacement evidence missing'):
            patchset.load_manifest(self.root)
        self.first['replacement'].update(commit='0' * 40, evidence='absent commit')
        self.save()
        self.assertNotEqual(self.cli('check', '--only', 'first').returncode, 0)
        self.assert_clean()

    def test_plan_checks_paths_without_running_commands(self):
        check = {'name': 'command plan', 'tier': 'focused', 'cwd': '.',
                 'requires': ['file.txt'], 'runner': 'command',
                 'command': [sys.executable, '-c', 'raise SystemExit(7)']}
        self.set_checks(self.first, [check])
        result = self.cli('test', '--only', 'first', '--plan')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('NOT RUN', result.stdout)
        self.assertNotIn('Completed', result.stdout)
        check['requires'] = ['missing.txt']
        self.set_checks(self.first, [check])
        result = self.cli('test', '--only', 'first', '--plan')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('entry missing', result.stderr)

    def test_commands_run_once_in_application_cwd_and_propagate_failure(self):
        check = {'name': 'cwd contract', 'tier': 'focused', 'cwd': 'example',
                 'requires': ['example/fixture_test.go'], 'runner': 'command',
                 'command': [sys.executable, '-c', "from pathlib import Path; "
                             "assert Path('fixture_test.go').is_file(); Path('ran').open('x').close()"]}
        self.set_checks(self.first, [check])
        self.set_checks(self.second, [check])
        result = self.cli('test')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Completed 1 focused checks', result.stdout)
        check['command'] = [sys.executable, '-c', 'raise SystemExit(9)']
        self.set_checks(self.first, [check])
        result = self.cli('test')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('exit status 9', result.stderr)
        self.assertNotIn('Completed', result.stdout)
        result = self.cli('test', '--tier', 'project')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('No checks in tier', result.stderr)

    def test_go_discovery_nonempty_execution_and_failure_handling(self):
        # Use an executable fixture so maintenance tests can run without Go.
        runner = self.target / '.git/go'
        runner.write_text(f'#!{sys.executable}\n' + '''import json, os, sys
from pathlib import Path
args = sys.argv[1:]
mode = os.environ['GO_FIXTURE_MODE']
assert args[:2] == ['test', '-mod=readonly']
assert args[-1] == '.' and Path.cwd().name == 'example'
names = ['TestA', 'TestB']
if '-list' in args:
    assert args[-2] == '^(TestA|TestB)$'
    if mode not in ('no-discovery', 'partial-discovery'):
        print('TestA')
    if mode != 'no-discovery':
        print('TestB')
else:
    assert '-json' in args and '-count=1' in args and '-run' in args
    assert args[-2] == '^(TestA|TestB)$'
    if mode == 'partial-result':
        names.pop()
    if mode != 'empty-result':
        for name in names:
            print(json.dumps({'Action': 'pass', 'Test': name}))
    if mode == 'skipped':
        print(json.dumps({'Action': 'skip', 'Test': 'TestB/optional'}))
    if mode == 'failed':
        print(json.dumps({'Action': 'output', 'Output': 'regression failed\\n'}))
        sys.exit(4)
''')
        runner.chmod(0o755)
        self.set_checks(self.first, [
            {'name': 'Go fixture', 'tier': 'focused', 'cwd': 'example', 'runner': 'go',
             'requires': ['example/fixture_test.go'], 'package': '.', 'tests': ['TestA', 'TestB']}])
        env = {**os.environ, 'PATH': str(runner.parent) + os.pathsep + os.environ['PATH']}
        for mode, diagnostic in [('pass', 'PASSED 2'), ('no-discovery', 'discovery differs'),
                                 ('partial-discovery', 'discovery differs'), ('empty-result', 'results differ'),
                                 ('partial-result', 'results differ'), ('skipped', 'tests skipped'),
                                 ('failed', 'exit status 4')]:
            with self.subTest(mode=mode):
                result = self.cli('test', '--only', 'first', env={**env, 'GO_FIXTURE_MODE': mode})
                if mode == 'pass':
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertIn(diagnostic, result.stdout)
                else:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(diagnostic, result.stderr)
                    self.assertNotIn('Completed', result.stdout)
                if mode == 'failed':
                    self.assertIn('regression failed', result.stdout)

    def test_paths_cannot_escape_checkout(self):
        for path in ('../outside', '/absolute'):
            with self.assertRaises(patchset.Failure):
                patchset.inside(self.target, path)
        (self.target / 'escape').symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(patchset.Failure):
            patchset.inside(self.target, 'escape/manifest.json')


if __name__ == '__main__':
    unittest.main()
