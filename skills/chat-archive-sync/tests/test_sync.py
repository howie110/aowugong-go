import copy
import hashlib
import json
import sys
import tempfile
import unittest
import subprocess
import shutil
import sqlite3
from contextlib import contextmanager
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from sync import SyncConfig, sync_source, load_config


def sample(text="hello", mid="m"):
    return (json.dumps({"type": "response_item", "payload": {"id": mid, "type": "message", "role": "user", "internal_chat_message_metadata_passthrough": {"content_item_kinds": ["user.text"]}, "content": [{"type": "input_text", "text": text}]}}) + "\n").encode()


HEADER = b'{"type":"session_meta","payload":{"id":"session"}}\n'


class MemoryStore:
    def __init__(self):
        self.state = None
        self.batches = []
        self.fail = False

    @contextmanager
    def sync_transaction(self, key):
        before = copy.deepcopy((self.state, self.batches))
        try:
            yield self
        except Exception:
            self.state, self.batches = before
            raise

    def get_state(self, key):
        return self.state

    def save_batch(self, config, key, state, raw, parsed, offset):
        self.batches.append((raw, parsed))
        if self.fail:
            raise RuntimeError("synthetic database failure")
        self.state = state


class SyncTests(unittest.TestCase):
    def test_large_codex_header_uses_same_event_limit(self):
        self.path.write_bytes((json.dumps({'type':'session_meta','payload':{'id':'session','instructions':'x' * (1024 * 1024 + 1)}}) + '\n').encode() + sample())
        self.assertEqual(sync_source(self.cfg, self.store)['messages'], 1)

    @unittest.skipUnless(shutil.which('zstd'), 'zstd not installed')
    def test_finalized_compressed_log_without_newline_is_not_reported_complete(self):
        self.path.write_bytes(HEADER + sample()[:-1])
        compressed = self.path.with_suffix('.jsonl.zst')
        subprocess.run(['zstd','-q',str(self.path),'-o',str(compressed)], check=True)
        with self.assertRaises(ValueError):
            sync_source(SyncConfig('device','account','codex',str(compressed)), self.store)
        self.assertEqual(self.store.batches, [])

    def test_discovery_includes_compressed_and_reports_index_gaps(self):
        from sync import discover_paths, check_codex_index
        compressed = self.path.with_suffix('.jsonl.zst')
        compressed.touch()
        paths = discover_paths(str(self.path.parent / '*.jsonl'))
        self.assertEqual(paths, [str(self.path)])
        self.path.unlink()
        self.assertEqual(discover_paths(str(self.path.parent / '*.jsonl')), [str(compressed)])
        index = self.path.parent / 'index.sqlite'
        with sqlite3.connect(index) as db:
            db.execute('CREATE TABLE threads(id TEXT, rollout_path TEXT, source TEXT)')
            db.executemany('INSERT INTO threads VALUES (?,?,?)', [('a',str(self.path),'vscode'), ('b',str(self.path.parent / 'missing.jsonl'),'vscode'), ('c',str(self.path.parent / 'sub.jsonl'),'{"subagent":{}}')])
        result = check_codex_index(str(index), [{'source':'codex','path':str(self.path.parent / '*.jsonl')}])
        self.assertEqual(result, {'expected_sessions':2, 'located_sessions':1, 'missing_session_ids':['b']})
        result = check_codex_index(str(index), [{'source':'codex','path':str(self.path.parent / '**/*.jsonl')}])
        self.assertEqual(result['expected_sessions'], 2)

    def test_large_log_does_not_block_small_dialogue(self):
        with self.path.open('wb') as stream:
            stream.write(HEADER)
            event = (json.dumps({'type': 'event_msg', 'payload': {'type': 'token_count', 'padding': 'x' * 1024}}) + '\n').encode()
            for _ in range(66000):
                stream.write(event)
            stream.write(sample('small dialogue'))
        result = sync_source(self.cfg, self.store)
        self.assertEqual(result['messages'], 1)
        self.assertEqual(self.store.batches[-1][1].messages[0]['body'], 'small dialogue')
        self.assertEqual(sync_source(self.cfg, self.store)['messages'], 0)

    @unittest.skipUnless(shutil.which('zstd'), 'zstd not installed')
    def test_compressed_log_reuses_uncompressed_identity_and_progress(self):
        sync_source(self.cfg, self.store)
        compressed = self.path.with_suffix('.jsonl.zst')
        subprocess.run(['zstd', '-q', str(self.path), '-o', str(compressed)], check=True)
        cfg = SyncConfig('device', 'account', 'codex', str(compressed))
        self.assertEqual(cfg.key, self.cfg.key)
        self.assertEqual(sync_source(cfg, self.store)['messages'], 0)
        fresh = MemoryStore()
        self.assertEqual(sync_source(cfg, fresh)['messages'], 1)
        self.assertEqual(fresh.batches[0][1].messages[0]['body'], 'hello')
        compressed.write_bytes(compressed.read_bytes()[:-5])
        broken = MemoryStore()
        with self.assertRaises(ValueError):
            sync_source(cfg, broken)
        self.assertEqual(broken.batches, [])

    def test_subagent_session_is_skipped_without_writes(self):
        self.path.write_bytes(b'{"type":"session_meta","payload":{"id":"sub","source":{"subagent":{"thread_spawn":{}}}}}\n' + sample())
        result = sync_source(self.cfg, self.store)
        self.assertEqual(result.get('skipped'), 'subagent_session')
        self.assertEqual(self.store.batches, [])

    def test_upload_boundary_contains_only_user_assistant_text(self):
        records = [
            {"type": "response_item", "payload": {"type": "message", "role": "system", "content": [{"type": "text", "text": "SYSTEM_SECRET"}]}},
            {"type": "response_item", "payload": {"type": "message", "role": "assistant", "channel": "analysis", "content": [{"type": "output_text", "text": "THINKING_SECRET"}]}},
            {"type": "response_item", "payload": {"type": "message", "role": "assistant", "phase": "final_answer", "content": [{"type": "output_text", "text": "answer"}, {"type": "image", "text": "IMAGE_SECRET", "url": "private-url"}, {"type": "tool_result", "text": "TOOL_SECRET"}]}},
        ]
        self.path.write_bytes(HEADER + sample() + b''.join((json.dumps(r) + '\n').encode() for r in records))
        result = sync_source(self.cfg, self.store)
        self.assertEqual(result['messages'], 2)
        raw, batch = self.store.batches[0]
        self.assertEqual(raw, b'')
        self.assertEqual([m['body'] for m in batch.messages], ['hello', 'answer'])
        self.assertTrue(all(m['blocks'] == [] and m['attachments'] == [] for m in batch.messages))

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / "chat.jsonl"
        self.path.write_bytes(HEADER + sample())
        self.cfg = SyncConfig("device", "account", "codex", str(self.path))
        self.store = MemoryStore()

    def test_append_and_duplicate(self):
        self.assertEqual(sync_source(self.cfg, self.store)["messages"], 1)
        self.assertEqual(sync_source(self.cfg, self.store)["bytes"], 0)
        with self.path.open("ab") as f:
            f.write(sample("hello", "m2"))
        self.assertEqual(sync_source(self.cfg, self.store)["messages"], 1)
        self.assertEqual(self.store.batches[-1][0], b'')
        self.assertEqual(self.store.batches[-1][1].messages[0]['body'], 'hello')

    def test_rewrite_same_size_and_truncation(self):
        sync_source(self.cfg, self.store)
        self.path.write_bytes(HEADER + sample("world"))
        self.assertEqual(sync_source(self.cfg, self.store)["messages"], 1)
        self.assertEqual(self.store.batches[-1][1].messages[0]["body"], "world")
        self.path.write_bytes(HEADER)
        sync_source(self.cfg, self.store)
        self.assertEqual(self.store.state["offset"], len(HEADER))

    def test_failure_rolls_back_and_partial_completes(self):
        self.store.fail = True
        with self.assertRaises(RuntimeError):
            sync_source(self.cfg, self.store)
        self.assertIsNone(self.store.state)
        self.store.fail = False
        self.path.write_bytes(HEADER + sample()[:-2])
        sync_source(self.cfg, self.store)
        self.assertEqual(self.store.state["offset"], len(HEADER))
        with self.path.open("ab") as f:
            f.write(sample()[-2:])
        self.assertEqual(sync_source(self.cfg, self.store)["messages"], 1)

    def test_dry_run_has_no_writes(self):
        result = sync_source(self.cfg, self.store, dry_run=True)
        self.assertEqual(result["messages"], 1)
        self.assertIsNone(self.store.state)
        self.assertEqual(self.store.batches, [])

    def test_no_session_rejected_and_config_requires_scope(self):
        self.path.write_bytes(sample())
        with self.assertRaises(ValueError):
            sync_source(self.cfg, self.store)
        config = Path(self.temp.name) / "config.json"
        config.write_text(json.dumps({"dsn": "host=localhost dbname=test", "device_id": "x", "account_scope": "", "sources": []}))
        config.chmod(0o600)
        with self.assertRaises(ValueError):
            load_config(config)


if __name__ == "__main__":
    unittest.main()
