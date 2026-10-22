import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))
from configure_device import configure


class ConfigureDeviceTests(unittest.TestCase):
    def test_sets_real_paths_keeps_device_and_removes_old_connection(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            (root / 'sessions').mkdir()
            (root / 'state_5.sqlite').touch()
            path = root / 'config.json'
            path.write_text(json.dumps({'api_url': 'https://example.com', 'api_key': 'k' * 40, 'account_scope': 'a', 'dsn': 'old', 'device_id': ''}))
            result = configure(path, root)
            self.assertTrue(result['configured'])
            config = json.loads(path.read_text())
            self.assertNotIn('dsn', config)
            self.assertEqual(config['sources'][0]['path'], str(root / 'sessions' / '**' / '*.jsonl'))
            self.assertEqual(config['codex_index'], str(root / 'state_5.sqlite'))
            self.assertTrue(config['device_id'])
            configure(path, root)
            self.assertEqual(config['device_id'], json.loads(path.read_text())['device_id'])
