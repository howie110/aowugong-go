"""仅使用显式临时数据库；默认跳过，绝不读取项目 .env。"""
import concurrent.futures
import hashlib
import os
from pathlib import Path
import sys
import tempfile
import json
import subprocess
import threading
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from store import ArchiveStore
from sync import SyncConfig, sync_source
from test_sync import HEADER, sample


@unittest.skipUnless(os.environ.get("CHAT_ARCHIVE_TEST_DSN"), "isolated PostgreSQL DSN not supplied")
class StoreTests(unittest.TestCase):
    def test_second_batch_failure_rolls_back_first_batch_and_cursor(self):
        with self.path.open('ab') as stream:
            for index in range(505):
                stream.write(sample('bulk', f'bulk-{index}'))
        with ArchiveStore(self.dsn) as store:
            original = store.save_batch
            calls = 0
            def fail_second(*args):
                nonlocal calls
                calls += 1
                original(*args)
                if calls == 2:
                    raise RuntimeError('second batch fails')
            store.save_batch = fail_second
            with self.assertRaises(RuntimeError):
                sync_source(self.cfg, store)
        self.assertEqual(calls, 2)
        for table in ('chat_archive_conversations','chat_archive_raw_batches','chat_archive_message_versions','chat_archive_sync_states'):
            self.assertEqual(self.db.execute('SELECT count(*) FROM ' + table).fetchone()[0], 0)

    def test_streamed_batches_commit_together_and_verify(self):
        with self.path.open('ab') as stream:
            for index in range(505):
                stream.write(sample('bulk', f'bulk-{index}'))
        with ArchiveStore(self.dsn) as store:
            result = sync_source(self.cfg, store)
            self.assertEqual(result['messages'], 506)
            self.assertEqual(store.verify()['message_observations'], 506)
            self.assertEqual(sync_source(self.cfg, store)['messages'], 0)
        self.assertEqual(self.db.execute('SELECT count(*) FROM chat_archive_raw_batches').fetchone()[0], 2)

    def test_verification_rejects_message_assigned_to_another_conversation(self):
        with ArchiveStore(self.dsn) as store:
            sync_source(self.cfg, store)
            other = self.db.execute("INSERT INTO chat_archive_conversations(source,account_scope,source_id) VALUES ('codex','account','other') RETURNING id").fetchone()[0]
            self.db.execute('UPDATE chat_archive_message_versions SET conversation_id=%s', (other,))
            with self.assertRaises(ValueError):
                store.verify()

    def test_cli_refuses_legacy_data_before_upload(self):
        with ArchiveStore(self.dsn) as store:
            sync_source(self.cfg, store)
        self.db.execute("UPDATE chat_archive_raw_batches SET raw='legacy'::bytea,parser_version='1'")
        config = Path(self.temp.name) / 'private.json'
        config.write_text(json.dumps({'dsn': self.dsn, 'device_id': 'a', 'account_scope': 'account', 'sources': [{'source': 'codex', 'path': str(self.path)}]}))
        config.chmod(0o600)
        with self.path.open('ab') as stream:
            stream.write(sample('must not upload', 'second'))
        script = str(Path(__file__).resolve().parents[1] / 'scripts/sync.py')
        result = subprocess.run([sys.executable, script, '--config', str(config)], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(result.stdout)['legacy_batches'], 1)
        self.assertEqual(self.db.execute('SELECT count(*) FROM chat_archive_message_versions').fetchone()[0], 1)

    def test_database_contains_no_source_bytes_or_nontext_metadata(self):
        with ArchiveStore(self.dsn) as store:
            sync_source(self.cfg, store)
            self.assertTrue(store.verify()['verified'])
        self.assertEqual(self.db.execute('SELECT raw FROM chat_archive_raw_batches').fetchone()[0], b'')
        self.assertEqual(self.db.execute('SELECT body,blocks,attachments FROM chat_archive_message_versions').fetchone(), ('hello', [], []))

    @classmethod
    def setUpClass(cls):
        import psycopg
        cls.dsn = os.environ["CHAT_ARCHIVE_TEST_DSN"]
        cls.db = psycopg.connect(cls.dsn, autocommit=True, client_encoding="UTF8")
        if cls.db.execute("SELECT current_database()").fetchone()[0] != "chat_archive_test":
            raise RuntimeError("refusing non-test database")
        migration = Path(__file__).resolve().parents[3] / "migrations/postgres/00013_chat_archive.sql"
        cls.db.execute("DROP TABLE IF EXISTS chat_archive_sync_states, chat_archive_message_versions, chat_archive_raw_batches, chat_archive_conversations CASCADE")
        cls.db.execute(migration.read_text().split("-- +goose Down")[0])

    @classmethod
    def tearDownClass(cls):
        cls.db.close()

    def setUp(self):
        self.db.execute("TRUNCATE chat_archive_sync_states, chat_archive_message_versions, chat_archive_raw_batches, chat_archive_conversations RESTART IDENTITY CASCADE")
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / "chat.jsonl"
        self.path.write_bytes(HEADER + sample())
        self.cfg = SyncConfig("a", "account", "codex", str(self.path))

    def test_concurrent_repeat_cross_device_and_edits(self):
        def upload(cfg):
            with ArchiveStore(self.dsn) as store:
                return sync_source(cfg, store)
        with concurrent.futures.ThreadPoolExecutor(2) as pool:
            results = list(pool.map(upload, [self.cfg, self.cfg]))
        self.assertEqual(sum(r["messages"] for r in results), 1)
        upload(SyncConfig("b", "account", "codex", str(self.path)))
        self.path.write_bytes(HEADER + sample("world"))
        upload(self.cfg)
        self.assertEqual(self.db.execute("SELECT count(*) FROM chat_archive_conversations").fetchone()[0], 1)
        self.assertEqual(self.db.execute("SELECT count(DISTINCT message_key), count(DISTINCT version_hash), count(*) FROM chat_archive_message_versions").fetchone(), (1, 2, 3))
        with ArchiveStore(self.dsn) as store:
            self.assertEqual(store.verify()["batches"], 3)

    def test_rollback_dry_run_and_verification_detect_corruption(self):
        with ArchiveStore(self.dsn) as store:
            sync_source(self.cfg, store, dry_run=True)
            self.assertEqual(self.db.execute("SELECT count(*) FROM chat_archive_sync_states").fetchone()[0], 0)
            original = store.save_batch
            def broken(*args):
                original(*args)
                raise RuntimeError("synthetic crash before commit")
            store.save_batch = broken
            with self.assertRaises(RuntimeError):
                sync_source(self.cfg, store)
            self.assertEqual(self.db.execute("SELECT count(*) FROM chat_archive_raw_batches").fetchone()[0], 0)
            store.save_batch = original
            sync_source(self.cfg, store)
            self.db.execute("UPDATE chat_archive_raw_batches SET raw='broken'::bytea")
            with self.assertRaises(ValueError):
                store.verify()

    def test_cli_preview_upload_repeat_verify_and_bad_file(self):
        config = Path(self.temp.name) / "private.json"
        config.write_text(json.dumps({"dsn": self.dsn, "device_id": "a", "account_scope": "account", "sources": [{"source": "codex", "path": str(self.path)}]}))
        config.chmod(0o600)
        script = str(Path(__file__).resolve().parents[1] / "scripts/sync.py")
        def run(*args):
            result = subprocess.run([sys.executable, script, "--config", str(config), *args], capture_output=True, text=True)
            self.assertNotIn(self.dsn, result.stdout + result.stderr)
            self.assertNotIn("hello", result.stdout + result.stderr)
            return result
        preview = run("--dry-run")
        self.assertEqual(preview.returncode, 0, preview.stderr)
        self.assertEqual(self.db.execute("SELECT count(*) FROM chat_archive_raw_batches").fetchone()[0], 0)
        self.assertEqual(run().returncode, 0)
        self.assertEqual(json.loads(run().stdout)["bytes"], 0)
        self.assertTrue(json.loads(run("--verify").stdout)["verified"])
        with self.path.open("ab") as stream:
            stream.write(b'{"not valid"\n')
        before = self.db.execute("SELECT state FROM chat_archive_sync_states").fetchone()[0]
        self.assertEqual(run().returncode, 1)
        after, error = self.db.execute("SELECT state,last_error FROM chat_archive_sync_states").fetchone()
        self.assertEqual(before, after)
        self.assertEqual(error, "sync_failed")

    def test_first_message_after_header_updates_fallback_title(self):
        self.path.write_bytes(HEADER)
        with ArchiveStore(self.dsn) as store:
            sync_source(self.cfg, store)
            with self.path.open("ab") as stream:
                stream.write(sample("first prompt"))
            sync_source(self.cfg, store)
        self.assertEqual(self.db.execute("SELECT title FROM chat_archive_conversations").fetchone()[0], "first prompt")

    def test_unrepresentable_json_strings_do_not_block_text_archive(self):
        self.path.write_bytes(HEADER + sample("NUL\x00 and surrogate\ud800") + sample("next", "m2"))
        with ArchiveStore(self.dsn) as store:
            result = sync_source(self.cfg, store)
            self.assertEqual(result["messages"], 2)
            self.assertEqual(store.verify()["message_observations"], 2)
        body, completeness, blocks = self.db.execute("SELECT body,completeness,blocks FROM chat_archive_message_versions ORDER BY id LIMIT 1").fetchone()
        self.assertEqual(completeness, "escaped_source_text")
        self.assertIn("\\u0000", body)
        self.assertEqual(blocks, [])
        self.assertEqual(self.db.execute("SELECT raw FROM chat_archive_raw_batches").fetchone()[0], b'')

    def test_overlapping_append_and_rollback_keep_success_cursor(self):
        held, release, second_started, second_read = (threading.Event() for _ in range(4))
        def first_upload():
            with ArchiveStore(self.dsn) as store:
                original = store.save_batch
                def held_save(*args):
                    held.set()
                    if not release.wait(5):
                        raise RuntimeError("test release timed out")
                    return original(*args)
                store.save_batch = held_save
                return sync_source(self.cfg, store)
        def second_upload():
            with ArchiveStore(self.dsn) as store:
                original = store.get_state
                def observed_read(*args):
                    second_read.set()
                    return original(*args)
                store.get_state = observed_read
                second_started.set()
                return sync_source(self.cfg, store)
        with concurrent.futures.ThreadPoolExecutor(2) as pool:
            first = pool.submit(first_upload)
            self.assertTrue(held.wait(5))
            with self.path.open("ab") as stream:
                stream.write(sample("append", "second"))
            second = pool.submit(second_upload)
            try:
                self.assertTrue(second_started.wait(5))
                self.assertFalse(second_read.wait(0.1), "second sync read state before lock release")
            finally:
                release.set()
            self.assertEqual(first.result(timeout=5)["messages"], 1)
            self.assertEqual(second.result(timeout=5)["messages"], 1)
        before = self.db.execute("SELECT state FROM chat_archive_sync_states").fetchone()[0]
        self.assertEqual(before["offset"], self.path.stat().st_size)
        with self.path.open("ab") as stream:
            stream.write(sample("failed append", "third"))
        with ArchiveStore(self.dsn) as store:
            original = store.save_batch
            def fail_after_save(*args):
                original(*args)
                raise RuntimeError("crash after insert")
            store.save_batch = fail_after_save
            with self.assertRaises(RuntimeError):
                sync_source(self.cfg, store)
        self.assertEqual(self.db.execute("SELECT state FROM chat_archive_sync_states").fetchone()[0], before)
        self.assertEqual(self.db.execute("SELECT count(*) FROM chat_archive_message_versions").fetchone()[0], 2)


if __name__ == "__main__":
    unittest.main()
