import json
from pathlib import Path
import sys
import tempfile
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))


class InstallTests(unittest.TestCase):
    def test_failed_backup_protection_never_leaves_secret_contents(self):
        from install import install
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);target=root/'skills'/'chat-archive-sync';target.mkdir(parents=True)
            secret={'api_url':'https://example.com','api_key':'k'*40,'device_id':'d','account_scope':'a',
                    'sources':[{'source':'codex','path':'/s.jsonl'}]}
            cfg=target/'config.json';cfg.write_text(json.dumps(secret));cfg.chmod(0o600)
            with patch('install.protect_private_config',side_effect=OSError('ACL failed')):
                with self.assertRaises(OSError): install(target,cfg)
            for copied in (root/'skill-backups').glob('*/config.json'):
                self.assertEqual(copied.read_bytes(),b'')

    def test_upgrade_preserves_existing_device_config_and_backs_up_code(self):
        from install import install
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root/'skills'/'chat-archive-sync'
            target.mkdir(parents=True)
            (target/'SKILL.md').write_text('old skill')
            cfg = root/'existing.json'
            original = {'api_url':'https://example.com','api_key':'k'*40,'account_scope':'company',
                        'device_id':'original-device','sources':[{'source':'codex','path':'/authorized/*.jsonl'}]}
            cfg.write_text(json.dumps(original)); cfg.chmod(0o600)
            result = install(target, cfg)
            self.assertEqual(json.loads((target/'config.json').read_text()),original)
            self.assertEqual(json.loads(cfg.read_text()),original)
            self.assertEqual((Path(result['backup'])/'SKILL.md').read_text(),'old skill')
            self.assertTrue((target/'scripts'/'reconcile.py').is_file())
            self.assertEqual((target/'config.json').stat().st_mode & 0o777,0o600)

    def test_install_refuses_missing_existing_identity_instead_of_silent_duplicate(self):
        from install import install
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp); cfg=root/'config.json'
            cfg.write_text(json.dumps({'api_url':'https://example.com','api_key':'k'*40,'device_id':'','sources':[]}))
            cfg.chmod(0o600)
            with self.assertRaises(ValueError): install(root/'skill',cfg)


if __name__ == '__main__': unittest.main()
