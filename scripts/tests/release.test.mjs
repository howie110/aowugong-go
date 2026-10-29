import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { buildRelease, run } from '../build-release.mjs';

function fixture(t) {
  const root = mkdtempSync(path.join(tmpdir(), 'release-test-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const file of ['README.md', 'scripts/check.sh', 'init/systemd/app.service', 'skills/tool/SKILL.md', 'migrations/postgres/001.sql']) {
    mkdirSync(path.dirname(path.join(root, file)), { recursive: true });
    writeFileSync(path.join(root, file), file.endsWith('.sh') ? '#!/bin/bash\ntrue\n' : file);
  }
  mkdirSync(path.join(root, 'web'));
  writeFileSync(path.join(root, '.gitignore'), 'dist/\nweb/dist/\n.env\n');
  const git = (args) => run('git', args, { cwd: root, stdio: 'pipe' });
  git(['init', '-q']); git(['add', '.']);
  git(['-c', 'user.name=Test', '-c', 'user.email=test@example.test', 'commit', '-qm', 'fixture']);
  writeFileSync(path.join(root, '.env'), 'SECRET=must-not-ship');
  const calls = [];
  function execute(command, args, options) {
    calls.push([command, ...args]);
    if (command.startsWith('npm')) {
      if (args[0] === 'run') {
        mkdirSync(path.join(root, 'web/dist'), { recursive: true });
        writeFileSync(path.join(root, 'web/dist/index.html'), '<html>built</html>');
      }
      return '';
    }
    if (command === 'go') {
      if (args[0] === 'build') writeFileSync(args[args.indexOf('-o') + 1], 'binary', { mode: 0o755 });
      return '';
    }
    return run(command, args, { ...options, stdio: 'pipe' });
  }
  return { root, execute, calls, git };
}

test('release contains required artifacts and revision, excludes private configuration', (t) => {
  const f = fixture(t);
  const archive = buildRelease(f.root, 'v-test', f.execute);
  const list = run('tar', ['-tzf', archive], { stdio: 'pipe' });
  for (const file of ['aowugong', 'web/dist/index.html', 'skills/tool/SKILL.md', 'migrations/postgres/001.sql', 'REVISION']) assert.ok(list.includes(file));
  assert.ok(!list.includes('.env'));
  assert.equal(run('tar', ['-xOf', archive, 'aowugong-go-v-test-linux-amd64/REVISION'], { stdio: 'pipe' }), f.git(['rev-parse', 'HEAD']));
  assert.ok(readFileSync(`${archive}.sha256`, 'utf8').startsWith(createHash('sha256').update(readFileSync(archive)).digest('hex')));
  assert.ok(f.calls.some(c => c.includes('-race')));
});

for (const failure of ['npm-test', 'go-test', 'go-build', 'tar']) {
  test(`failure at ${failure} stops without publishing an archive`, (t) => {
    const f = fixture(t);
    assert.throws(() => buildRelease(f.root, 'v-fail', (command, args, options) => {
      if ((failure === 'npm-test' && command.startsWith('npm') && args[0] === 'test') ||
          (failure === 'go-test' && command === 'go' && args[0] === 'test') ||
          (failure === 'go-build' && command === 'go' && args[0] === 'build') ||
          (failure === 'tar' && command === 'tar' && args[0] === '-czf')) throw new Error('injected failure');
      return f.execute(command, args, options);
    }), /injected failure/);
    assert.equal(existsSync(path.join(f.root, 'dist/aowugong-go-v-fail-linux-amd64.tar.gz')), false);
  });
}
test('dirty source and invalid version are rejected before any build', (t) => {
  const f = fixture(t);
  assert.throws(() => buildRelease(f.root, '../escape', f.execute), /Invalid/);
  writeFileSync(path.join(f.root, 'README.md'), 'modified');
  assert.throws(() => buildRelease(f.root, 'v-dirty', f.execute), /clean committed/);
  assert.ok(!f.calls.some(c => c[0] === 'go' || c[0].startsWith('npm')));
});
test('command runner propagates failed subprocess status', () => {
  assert.throws(() => run(process.execPath, ['-e', 'process.exit(7)'], { stdio: 'pipe' }), /failed \(7\)/);
});
