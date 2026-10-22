"""安装/升级本包，沿用明确提供的现有私密配置；不上传、不发现凭据。"""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
import tempfile

from configure_device import protect_private_config
from sync import load_config


def install(target, config_path):
    target, config_path = Path(target).expanduser().resolve(), Path(config_path).expanduser().resolve()
    package = Path(__file__).resolve().parents[1]
    if target == package or target == Path.home() or target == Path(target.anchor):
        raise ValueError('archive_install_target_invalid')
    # 必须保留已用设备ID/账号范围/授权路径，不用新空配置覆盖旧设备。
    config = load_config(config_path)
    if not config.get('api_url'):
        raise ValueError('archive_https_config_required')
    backup = None
    if target.exists():
        backup_root = target.parent.parent / 'skill-backups'
        backup_root.mkdir(parents=True, exist_ok=True)
        backup = Path(tempfile.mkdtemp(prefix='chat-archive-sync-'+datetime.now(timezone.utc).strftime('%Y%m%d-'), dir=backup_root))
        for name in ('SKILL.md', 'scripts', 'tests', 'agents', 'config.json'):
            existing = target / name
            if name == 'config.json' and existing.is_file():
                destination = backup/name
                destination.touch()
                protect_private_config(destination)
                shutil.copyfile(existing, destination)
            elif existing.is_dir(): shutil.copytree(existing, backup/name, ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
            elif existing.is_file(): shutil.copy2(existing, backup/name)
    target.mkdir(parents=True, exist_ok=True)
    for name in ('scripts', 'tests', 'agents'):
        if (package/name).is_dir():
            shutil.copytree(package/name, target/name, dirs_exist_ok=True, ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
    shutil.copy2(package/'SKILL.md', target/'SKILL.md')
    # 先限制权限再写内容；原配置保持原样，安装结果统一入口。
    output = target/'config.json'
    output.touch(exist_ok=True)
    protect_private_config(output)
    output.write_text(json.dumps(config,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    return {'installed':True,'target':str(target),'config':str(output),'backup':str(backup) if backup else None}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config',required=True,help='explicit existing private JSON; retains this device identity')
    parser.add_argument('--target',default=str(Path.home()/'.codex'/'skills'/'chat-archive-sync'))
    args=parser.parse_args()
    try:
        print(json.dumps(install(args.target,args.config))); return 0
    except Exception:
        print(json.dumps({'error':'archive_install_failed','action':'check existing private HTTPS config, device identity and target permissions'})); return 1


if __name__=='__main__': raise SystemExit(main())
