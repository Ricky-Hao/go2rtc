#!/usr/bin/env python3
"""Apply the manifest's ordered Git diffs and run checks in an application checkout."""

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ACTIVE = {'keep', 'feature', 'local', 'awaiting-upstream'}
STATUSES = ACTIVE | {'upstream', 'replaced'}
TIERS = {'focused', 'project'}


class Failure(Exception):
    pass


def require(condition, message):
    if not condition:
        raise Failure(message)


def inside(root, relative):
    path = Path(relative)
    require(not path.is_absolute() and '..' not in path.parts, f'Unsafe path: {relative}')
    resolved = (root / path).resolve()
    require(resolved.is_relative_to(root.resolve()), f'Path escapes checkout: {relative}')
    return resolved


def git(target, *args, env=None, data=None):
    result = subprocess.run(['git', '-C', str(target), *args], input=data,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    if result.returncode:
        detail = result.stderr.decode(errors='replace').strip()
        raise Failure(f'git {" ".join(args)} failed in {target} (exit {result.returncode})\n{detail}')
    return result.stdout


def strings(value):
    return isinstance(value, list) and bool(value) and all(isinstance(x, str) and x for x in value)


def validate_check(check):
    require(bool(check['name']) and check['tier'] in TIERS, 'Invalid check name or tier')
    name = check['name']
    require(strings(check['requires']), f'No check entry paths: {name}')
    if check['runner'] == 'go':
        package = check['package']
        require(package == '.' or re.fullmatch(r'\./[\w-]+(?:/[\w-]+)*', package),
                f'Expected a single local Go package: {name}')
        tests = check['tests']
        require(strings(tests) and len(set(tests)) == len(tests)
                and all(re.fullmatch(r'Test[A-Z0-9_][A-Za-z0-9_]*', test) for test in tests),
                f'Expected a nonempty list of distinct Go test names: {name}')
    else:
        require(check['runner'] == 'command', f'Unknown test runner: {name}')
        require(strings(check['command']), f'Expected a nonempty argument array: {name}')


def load_manifest(root=ROOT):
    manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8'))
    require(manifest['format'] == 1, 'Unsupported manifest format')
    require(re.fullmatch(r'[0-9a-f]{40}', manifest['base']), 'Base must be a full commit SHA')
    require(isinstance(manifest['patches'], list) and manifest['patches'], 'Empty patch manifest')
    seen = set()
    for entry in manifest['patches']:
        name = entry['id']
        require(re.fullmatch(r'[a-z0-9]+(?:-[a-z0-9]+)*', name), f'Invalid id: {name}')
        require(name not in seen, f'Duplicate id: {name}')
        require(entry['status'] in STATUSES, f'Unknown status: {name}')
        dependencies = entry['depends_on']
        require(isinstance(dependencies, list) and set(dependencies) <= seen,
                f'Missing or out-of-order dependency: {name}')
        seen.add(name)
        if entry['status'] in ACTIVE:
            require(entry['patch'] and inside(root, entry['patch']).is_file(), f'Payload missing: {name}')
            require(strings(entry['paths']) and len(set(entry['paths'])) == len(entry['paths']),
                    f'Empty or duplicate payload paths: {name}')
        else:
            require(entry['patch'] is None and entry['paths'] == [], f'Replacement has a payload: {name}')
            require(re.fullmatch(r'[0-9a-f]{40}', entry['replacement']['commit']),
                    f'Replacement commit missing: {name}')
            evidence = entry['replacement']['evidence']
            require(isinstance(evidence, str) and evidence.strip(), f'Replacement evidence missing: {name}')
        require(inside(root, f'patches/{name}/README.md').is_file(), f'Guide missing: {name}')
        read_checks(root, entry)
    return manifest


def read_checks(root, entry):
    contract = inside(root, entry['tests'])
    require(contract.is_file(), f'Test contract missing: {entry["id"]}')
    checks = json.loads(contract.read_text(encoding='utf-8'))['checks']
    require(isinstance(checks, list) and checks, f'Empty test contract: {entry["id"]}')
    for check in checks:
        validate_check(check)
    return checks


def select(manifest, only=None, statuses=None):
    entries = manifest['patches']
    known = {p['id']: p for p in entries}
    wanted = set(only) if only else {p['id'] for p in entries
                                    if not statuses or p['status'] in statuses}
    require(wanted <= known.keys(), f'Unknown patch ids: {sorted(wanted - known.keys())}')
    pending = list(wanted)
    while pending:
        for dependency in known[pending.pop()]['depends_on']:
            if dependency not in wanted:
                wanted.add(dependency)
                pending.append(dependency)
    require(bool(wanted), 'Selection is empty')
    return [p for p in entries if p['id'] in wanted]


def checkout(target):
    target = target.resolve()
    top = Path(git(target, 'rev-parse', '--show-toplevel').decode().strip()).resolve()
    require(top == target, '--target must name the application Git checkout root')
    return target


def clean_base(target, base):
    require(not git(target, 'status', '--porcelain', '--untracked-files=all',
                    env={**os.environ, 'GIT_OPTIONAL_LOCKS': '0'}),
            f'Target has changes or untracked files: {target}. '
            'Repeated application is refused. Use a fresh checkout for a new selection.')
    head = git(target, 'rev-parse', 'HEAD').decode().strip()
    require(head == base, f'Target HEAD {head} differs from fixed base {base}')


def applicable(target, entries):
    result = []
    for entry in entries:
        if entry['status'] in ACTIVE:
            result.append(entry)
            continue
        commit = entry['replacement']['commit']
        git(target, 'merge-base', '--is-ancestor', commit, 'HEAD')
        print(f'SKIP {entry["id"]}: {entry["status"]}, included commit {commit}')
    return result


def prepare(root, target, manifest, entries):
    base = manifest['base']
    clean_base(target, base)
    entries = applicable(target, entries)
    # Use a private index for the entire sequence. A later conflict leaves the
    # application's index and working files intact.
    git_dir = git(target, 'rev-parse', '--absolute-git-dir').decode().strip()
    with tempfile.TemporaryDirectory(prefix='patchset-', dir=git_dir) as temporary:
        env = {**os.environ, 'GIT_INDEX_FILE': str(Path(temporary) / 'index')}
        git(target, 'read-tree', base, env=env)
        tree = git(target, 'write-tree', env=env).decode().strip()
        for entry in entries:
            try:
                payload = inside(root, entry['patch']).read_bytes()
                git(target, 'apply', '--cached', '--whitespace=nowarn', '-', env=env, data=payload)
                names = git(target, 'diff', '--cached', '--no-renames', '--name-only', '-z', tree, '--', env=env)
                paths = {os.fsdecode(path) for path in names.split(b'\0') if path}
                require(paths == set(entry['paths']), f'Payload path inventory differs: {entry["id"]}')
                for path in paths:
                    inside(target, path)
                tree = git(target, 'write-tree', env=env).decode().strip()
            except Failure as error:
                raise Failure(f'Preflight stopped at {entry["id"]}. Target unchanged.\n{error}') from error
        combined = git(target, 'diff', '--binary', '--full-index', '--no-ext-diff', '--no-textconv',
                       '--no-renames', '--no-color', '--src-prefix=a/', '--dst-prefix=b/',
                       base, tree, '--')
    return entries, tree, combined


def apply(root, target, manifest, entries, write=False):
    entries, tree, combined = prepare(root, target, manifest, entries)
    if combined:
        clean_base(target, manifest['base'])
        options = ('--index',) if write else ('--index', '--check')
        git(target, 'apply', *options, '--whitespace=nowarn', '-', data=combined)
    action = 'APPLIED (staged)' if write else 'CHECKED (no application files changed)'
    print(f'{action}: {len(entries)} patches; tree {tree}')
    return tree


def contracts(root, target, entries, tier):
    checks = []
    seen = set()
    for entry in entries:
        for check in read_checks(root, entry):
            if tier != 'all' and check['tier'] != tier:
                continue
            key = json.dumps(check, sort_keys=True)
            if key in seen:
                continue
            seen.add(key)
            cwd = inside(target, check['cwd'])
            require(cwd.is_dir(), f'Check directory missing: {cwd}')
            if check['runner'] == 'go':
                package = inside(target, cwd.relative_to(target) / check['package'])
                require(package.is_dir(), f'Go test directory missing: {package}')
            for path in check['requires']:
                require(inside(target, path).is_file(), f'Test/check entry missing: {path}')
            checks.append(check)
    require(bool(checks), f'No checks in tier {tier}; nothing was run')
    return checks


def go_test(cwd, check):
    expected = set(check['tests'])
    pattern = '^(' + '|'.join(sorted(expected)) + ')$'
    package = check['package']
    command = ['go', 'test', '-mod=readonly']
    listing = subprocess.check_output([*command, '-list', pattern, package], cwd=cwd, text=True)
    found = {line for line in listing.splitlines() if line.startswith('Test')}
    require(found == expected, f'Go test discovery differs in {package}: '
            f'missing {sorted(expected - found)}, unexpected {sorted(found - expected)}')
    print(f'DISCOVERED {len(found)} Go tests in {package}', flush=True)

    result = subprocess.run([*command, '-json', '-count=1', '-run', pattern, package],
                            cwd=cwd, text=True, stdout=subprocess.PIPE)
    events = [json.loads(line) for line in result.stdout.splitlines()]
    if result.returncode:
        for event in events:
            if 'Output' in event:
                print(event['Output'], end='', flush=True)
        result.check_returncode()
    skipped = {event.get('Test', package) for event in events if event['Action'] == 'skip'}
    require(not skipped, f'Go tests skipped in {package}: {sorted(skipped)}')
    passed = {event['Test'] for event in events
              if event['Action'] == 'pass' and 'Test' in event and '/' not in event['Test']}
    require(passed == expected, f'Go test results differ in {package}: '
            f'missing {sorted(expected - passed)}, unexpected {sorted(passed - expected)}')
    print(f'PASSED {len(passed)} Go tests in {package}', flush=True)


def test(root, target, entries, tier='focused', plan=False):
    checks = contracts(root, target, entries, tier)
    for check in checks:
        print(f'{"NOT RUN" if plan else "RUN"}: {check["name"]} ({check["cwd"]})', flush=True)
        if plan:
            continue
        cwd = inside(target, check['cwd'])
        if check['runner'] == 'go':
            go_test(cwd, check)
        else:
            subprocess.run(check['command'], cwd=cwd, check=True)
    if not plan:
        print(f'Completed {len(checks)} {tier} checks.')


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=ROOT, help=argparse.SUPPRESS)
    parser.add_argument('action', choices=['list', 'check', 'apply', 'test'])
    parser.add_argument('--target', type=Path)
    selection = parser.add_mutually_exclusive_group()
    selection.add_argument('--only', nargs='+', metavar='ID', help='include ids and their dependencies')
    selection.add_argument('--status', nargs='+', choices=sorted(STATUSES))
    parser.add_argument('--tier', choices=sorted(TIERS | {'all'}), default='focused')
    parser.add_argument('--plan', action='store_true', help='validate test paths without running commands')
    args = parser.parse_args(argv)
    if args.plan and args.action != 'test':
        parser.error('--plan is only for test')
    root = args.root.resolve()
    manifest = load_manifest(root)
    entries = select(manifest, args.only, args.status)
    if args.action == 'list':
        for entry in entries:
            print(f'{entry["id"]}\t{entry["status"]}\t'
                  f'deps={",".join(entry["depends_on"]) or "-"}\t{entry["patch"] or "no payload"}')
        return
    require(args.target is not None, '--target is required')
    target = checkout(args.target)
    require(target != root, 'Use a separate application checkout')
    if args.action == 'test':
        test(root, target, entries, args.tier, args.plan)
    else:
        apply(root, target, manifest, entries, args.action == 'apply')


if __name__ == '__main__':
    try:
        main()
    except (Failure, OSError, TypeError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)
