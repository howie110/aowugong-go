import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, readlinkSync, realpathSync, symlinkSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
const scripts = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// 系统命令替身仅访问测试临时目录，任何意外系统路径写入立即失败。
const mock = `#!/usr/bin/env python3
import os, sys, shutil, subprocess
name=os.path.basename(sys.argv[0]); args=sys.argv[1:]; root=os.environ['TEST_ROOT']
with open(root+'/commands','a') as f: f.write(name+' '+' '.join(args)+'\\n')
if name=='id':
 if args==['-u']: print('0')
 elif args and args[0]=='-gn': print('test')
 sys.exit(0)
if name in ('chown','sleep'): sys.exit(0)
if name=='swapon': print('existing swap'); sys.exit(0)
if name in ('useradd','swapoff','mkswap','sysctl'): sys.exit(99)
if name=='systemctl':
 if args[0]=='is-active': print('active')
 if args[0]=='is-enabled': print('enabled')
 sys.exit(0)
if name=='curl': sys.exit(1 if os.environ.get('FAIL_HEALTH')=='1' else 0)
if name=='readlink': print(os.path.realpath(args[-1])); sys.exit(0)
if name=='mv' and args[0]=='-Tf':
 os.replace(args[1],args[2]); sys.exit(0)
if name=='sha256sum': sys.exit(subprocess.call(['shasum','-a','256']+args))
if name=='install':
 cleaned=[]; i=0
 while i<len(args):
  if args[i] in ('-o','-g'): i+=2; continue
  cleaned.append(args[i]); i+=1
 if not os.path.abspath(cleaned[-1]).startswith(root+'/'): raise Exception('unsafe install target')
 sys.exit(subprocess.call(['/usr/bin/install']+cleaned))
if name=='mv': sys.exit(subprocess.call(['/bin/mv']+args))
raise Exception('unexpected command '+name)
`;
function fixture(t, { initialized = true, old = true } = {}) {
  const root = realpathSync(mkdtempSync(path.join(tmpdir(), 'deploy-test-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const app = path.join(root, 'app'); const bin = path.join(root, 'bin'); const units = path.join(root, 'units');
  for (const dir of [bin, units, path.join(app, 'releases')]) mkdirSync(dir, { recursive: true });
  const helper = path.join(bin, 'mock'); writeFileSync(helper, mock, { mode: 0o755 });
  for (const name of ['id','chown','sleep','systemctl','curl','readlink','mv','sha256sum','install','swapon','useradd','swapoff','mkswap','sysctl']) symlinkSync(helper, path.join(bin, name));
  const envText = 'AOWUGONG_JWT_SECRET=test-secret\nAOWUGONG_ENCRYPTION_KEY=test-key\nAOWUGONG_DATABASE_URL=postgres://test\n';
  const supplied = path.join(root, 'provided.env'); writeFileSync(supplied, envText);
  if (initialized) { mkdirSync(path.join(app, 'shared/storage'), { recursive: true }); writeFileSync(path.join(app, 'shared/.env'), envText); }
  if (old) {
    mkdirSync(path.join(app, 'releases/v-old/migrations/postgres'), { recursive: true });
    mkdirSync(path.join(app, 'releases/v-old/init/systemd'), { recursive: true });
    writeFileSync(path.join(app, 'releases/v-old/init/systemd/aowugong-go.service'), 'old template');
    symlinkSync(path.join(app, 'releases/v-old'), path.join(app, 'current'));
    symlinkSync(path.join(app, 'releases/v-previous'), path.join(app, 'previous'));
    writeFileSync(path.join(units, 'aowugong-go.service'), 'original unit');
  }
  const packageDir = path.join(root, 'aowugong-go-v-new-linux-amd64');
  for (const file of ['aowugong','aowugong-migrate','web/dist/index.html','migrations/postgres/001.sql','scripts/tool.sh','init/systemd/aowugong-go.service','init/systemd/aowugong-go-canary.service']) {
    const target = path.join(packageDir, file); mkdirSync(path.dirname(target), { recursive: true }); writeFileSync(target, 'fixture');
  }
  const archive = path.join(root, 'aowugong-go-v-new-linux-amd64.tar.gz');
  assert.equal(spawnSync('tar', ['-czf', archive, '-C', root, path.basename(packageDir)]).status, 0);
  writeFileSync(archive+'.sha256', createHash('sha256').update(readFileSync(archive)).digest('hex')+'  '+path.basename(archive)+'\n');
  const env = { ...process.env, PATH: bin+path.delimiter+process.env.PATH, TEST_ROOT: root, TMPDIR: root,
    APP_ROOT: app, SYSTEMD_UNIT_DIR: units, RELEASE_ARCHIVE: archive, HEALTH_ATTEMPTS: '1', RUN_USER: 'test', DEPLOY_MODE: 'main', ENV_FILE: '' };
  const execute = (script='deploy-release.sh', overrides={}) => spawnSync('bash', [path.join(scripts, script), 'v-new'], { env: {...env,...overrides}, encoding: 'utf8' });
  return { root, app, units, envText, supplied, execute, commands: () => readFileSync(path.join(root,'commands'),'utf8') };
}
test('ordinary deploy switches release without machine initialization or shared chmod', t => {
  const f=fixture(t); const result=f.execute(); assert.equal(result.status,0,result.stderr);
  assert.equal(readlinkSync(path.join(f.app,'current')),path.join(f.app,'releases/v-new'));
  assert.equal(readlinkSync(path.join(f.app,'previous')),path.join(f.app,'releases/v-old'));
  assert.doesNotMatch(f.commands(), /swapon|useradd|sysctl|vaultwarden/);
  assert.ok(!f.commands().includes(`chown -R test:test ${f.app}/shared`));
  assert.match(readFileSync(path.join(f.app,'shared/.env'),'utf8'), /AOWUGONG_DATABASE_SKIP_MIGRATIONS=false/);
});
test('failed health restores exact config, unit, current and previous', t => {
  const f=fixture(t); writeFileSync(f.supplied, f.envText+'NEW_SETTING=1\n');
  const result=f.execute('deploy-release.sh',{FAIL_HEALTH:'1',ENV_FILE:f.supplied}); assert.notEqual(result.status,0);
  assert.equal(readlinkSync(path.join(f.app,'current')),path.join(f.app,'releases/v-old'));
  assert.equal(readlinkSync(path.join(f.app,'previous')),path.join(f.app,'releases/v-previous'));
  assert.equal(readFileSync(path.join(f.app,'shared/.env'),'utf8'),f.envText);
  assert.equal(readFileSync(path.join(f.units,'aowugong-go.service'),'utf8'),'original unit');
  assert.match(f.commands(), /systemctl start aowugong-go/);
});
test('uninitialized server is rejected before upload or service changes',t=>{
  const f=fixture(t,{initialized:false,old:false}); const r=f.execute(); assert.notEqual(r.status,0); assert.match(r.stderr,/未初始化/);
  assert.doesNotMatch(f.commands(),/systemctl|swapon|useradd|curl/);
});
test('explicit initialization followed by first deployment works; initialization preserves env',t=>{
  const f=fixture(t,{initialized:false,old:false}); let r=f.execute('initialize-server.sh',{ENV_FILE:f.supplied}); assert.equal(r.status,0,r.stderr);
  assert.equal(readFileSync(path.join(f.app,'shared/.env'),'utf8'),f.envText);
  writeFileSync(f.supplied,'replacement'); r=f.execute('initialize-server.sh',{ENV_FILE:f.supplied}); assert.equal(r.status,0,r.stderr);
  assert.equal(readFileSync(path.join(f.app,'shared/.env'),'utf8'),f.envText);
  r=f.execute(); assert.equal(r.status,0,r.stderr);
});
test('failed first canary removes new env and unit and leaves main untouched',t=>{
  const f=fixture(t); const r=f.execute('deploy-release.sh',{DEPLOY_MODE:'canary',FAIL_HEALTH:'1'}); assert.notEqual(r.status,0);
  assert.equal(existsSync(path.join(f.app,'canary')),false);
  assert.equal(existsSync(path.join(f.app,'shared/.env.canary')),false);
  assert.equal(existsSync(path.join(f.units,'aowugong-go-canary.service')),false);
  assert.equal(readFileSync(path.join(f.app,'shared/.env'),'utf8'),f.envText);
  assert.equal(readlinkSync(path.join(f.app,'current')),path.join(f.app,'releases/v-old'));
});

test('explicit rollback after upgrade restores previous application without changing database config',t=>{
  const f=fixture(t); let r=f.execute(); assert.equal(r.status,0,r.stderr);
  const config=readFileSync(path.join(f.app,'shared/.env'),'utf8');
  r=f.execute('rollback.sh'); assert.equal(r.status,0,r.stderr);
  assert.equal(readlinkSync(path.join(f.app,'current')),path.join(f.app,'releases/v-old'));
  assert.equal(readlinkSync(path.join(f.app,'previous')),path.join(f.app,'releases/v-new'));
  assert.equal(readFileSync(path.join(f.app,'shared/.env'),'utf8'),config);
});
test('reusing a release version is rejected without restarting',t=>{
  const f=fixture(t); let r=f.execute(); assert.equal(r.status,0,r.stderr);
  writeFileSync(path.join(f.root,'commands'),''); r=f.execute(); assert.notEqual(r.status,0); assert.match(r.stderr,/版本目录已存在/);
  assert.doesNotMatch(f.commands(),/systemctl/);
});
