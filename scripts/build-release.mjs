import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync, rmSync, renameSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export function run(command, args, options = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} ${args.join(' ')} failed (${result.status})`);
  return result.stdout?.toString().trim();
}

// 本地与 CI 共用完整验证、编译和打包流程；私有配置不进入 Git 归档。
export function buildRelease(root, version, execute = run) {
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(version)) throw new Error('Invalid release version');
  const invoke = (command, args, options = {}) => execute(command, args, { cwd: root, ...options });
  const capture = (args) => invoke('git', args, { stdio: ['ignore', 'pipe', 'inherit'] });
  if (capture(['status', '--porcelain', '--untracked-files=normal'])) {
    throw new Error('Release requires a clean committed worktree');
  }
  const revision = capture(['rev-parse', 'HEAD']);
  const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';
  for (const args of [['ci'], ['test'], ['run', 'build']]) {
    invoke(npm, args, { cwd: path.join(root, 'web'), shell: process.platform === 'win32' });
  }
  invoke('go', ['test', '-race', '-p', '1', './...']);
  invoke('go', ['vet', './...']);
  const dist = path.join(root, 'dist');
  mkdirSync(dist, { recursive: true });
  const staging = mkdtempSync(path.join(dist, '.release-'));
  const name = `aowugong-go-${version}-linux-amd64`;
  const packageDir = path.join(staging, name);
  mkdirSync(packageDir);
  try {
    // 从同一提交获取全部运行支持文件，避免工作区缓存或凭证混入。
    const sources = path.join(staging, 'sources.tar');
    invoke('git', ['archive', '--format=tar', `--output=${sources}`, revision,
      'migrations/postgres', 'init', 'scripts', 'skills', 'README.md']);
    invoke('tar', ['-xf', sources, '-C', packageDir]);
    invoke('bash', ['-c', 'for script in scripts/*.sh; do bash -n "$script" || exit; done']);
    const env = { ...process.env, CGO_ENABLED: '0', GOOS: 'linux', GOARCH: 'amd64' };
    for (const [binary, entry] of [['aowugong', './cmd/aowugong']]) {
      invoke('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', path.join(packageDir, binary), entry], { env });
    }
    // tar 保留静态资源结构，并使用三平台已有的归档工具。
    const frontend = path.join(staging, 'frontend.tar');
    invoke('tar', ['-cf', frontend, '-C', root, 'web/dist']);
    invoke('tar', ['-xf', frontend, '-C', packageDir]);
    writeFileSync(path.join(packageDir, 'VERSION'), `${version}\n`);
    writeFileSync(path.join(packageDir, 'REVISION'), `${revision}\n`);
    const filename = `${name}.tar.gz`;
    const archive = path.join(staging, filename);
    invoke('tar', ['-czf', archive, '-C', staging, name]);
    const digest = createHash('sha256').update(readFileSync(archive)).digest('hex');
    writeFileSync(`${archive}.sha256`, `${digest}  ${filename}\n`);
    renameSync(archive, path.join(dist, filename));
    renameSync(`${archive}.sha256`, path.join(dist, `${filename}.sha256`));
    return path.join(dist, filename);
  } finally {
    rmSync(staging, { recursive: true, force: true });
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
    console.log(`Release artifact: ${buildRelease(root, process.argv[2] || `dev-${Date.now()}`)}`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
