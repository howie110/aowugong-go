"""将用户提供的 HTTPS 私密配置适配到当前电脑；不读取聊天正文或其他凭据。"""
import argparse
import json
import os
from pathlib import Path
import uuid
import subprocess


def protect_private_config(path):
    if os.name != 'nt':
        os.chmod(path, 0o600)
        return
    # 仅当前用户 ACL，路径通过环境变量传递，不拼接进 PowerShell 表达式。
    script = """$ErrorActionPreference='Stop';
$sid=[System.Security.Principal.WindowsIdentity]::GetCurrent().User;
$acl=New-Object System.Security.AccessControl.FileSecurity;
$acl.SetOwner($sid);$acl.SetAccessRuleProtection($true,$false);
$rule=New-Object System.Security.AccessControl.FileSystemAccessRule($sid,'FullControl','Allow');
$acl.AddAccessRule($rule);
Set-Acl -LiteralPath $env:ARCHIVE_CONFIG_PATH -AclObject $acl
"""
    subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', script],
                   env={**os.environ, 'ARCHIVE_CONFIG_PATH': str(Path(path).resolve())},
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def configure(path, codex_home):
    from http_store import HTTPArchiveStore
    config_path = Path(path)
    protect_private_config(config_path)
    config = json.loads(config_path.read_text(encoding='utf-8'))
    HTTPArchiveStore(config.get('api_url', ''), config.get('api_key'))
    root = Path(codex_home).expanduser().resolve()
    if not root.is_dir() or not any((root / name).is_dir() for name in ('sessions', 'archived_sessions')):
        raise ValueError('codex_log_directory_missing')
    # 首次初始化设备身份；重跑不会产生另一个设备。
    if not config.get('device_id'):
        config['device_id'] = 'device-' + str(uuid.uuid4())
    config['sources'] = [{'source': 'codex', 'path': str(root / name / '**' / '*.jsonl')} for name in ('sessions', 'archived_sessions') if (root / name).is_dir()]
    indices = sorted(root.glob('state_*.sqlite'))
    if len(indices) == 1:
        config['codex_index'] = str(indices[0])
    else:
        config.pop('codex_index', None)
    config.pop('dsn', None)
    with config_path.open('w', encoding='utf-8') as stream:
        if os.name != 'nt':
            os.fchmod(stream.fileno(), 0o600)
        json.dump(config, stream, ensure_ascii=False, indent=2)
        stream.write('\n')
    return {'configured': True, 'source_patterns': len(config['sources']), 'index_configured': 'codex_index' in config}


def main():
    parser = argparse.ArgumentParser(description='Initialize private HTTPS archive config for this device; no upload')
    parser.add_argument('--config', required=True)
    parser.add_argument('--codex-home', required=True, help='explicit authorized local Codex data directory')
    args = parser.parse_args()
    try:
        print(json.dumps(configure(args.config, args.codex_home)))
        return 0
    except Exception:
        print(json.dumps({'error': 'device_configuration_failed', 'action': 'check provided HTTPS config and authorized Codex data directory'}))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
