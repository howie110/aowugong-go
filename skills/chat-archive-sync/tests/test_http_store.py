import json
import sys
import unittest
from pathlib import Path
from unittest.mock import patch
from http.server import BaseHTTPRequestHandler, HTTPServer
from threading import Thread

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))
from http_store import HTTPArchiveStore
from sync import SyncConfig
from adapters import ParsedBatch


class HTTPStoreTests(unittest.TestCase):
    def test_reconciliation_checks_hashes_and_retries_transient_response(self):
        seen = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args): pass
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                seen.append((self.path, body))
                if len(seen) == 1:
                    self.send_response(503); self.end_headers(); return
                self.send_response(200); self.end_headers()
                self.wfile.write(json.dumps({'statuses':['missing']} if self.path.endswith('/check') else {'inserted':1,'verified':1}).encode())
        server = HTTPServer(('127.0.0.1', 0), Handler)
        thread = Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            store = HTTPArchiveStore(f'http://127.0.0.1:{server.server_port}', 'k'*40)
            cfg = SyncConfig('d','a','codex','/s.jsonl')
            message = {'message_key':'id:m','body':'正文','role':'user','position':1}
            self.assertEqual(store.check_messages(cfg,'s',[message]), ['missing'])
            self.assertNotIn('body', seen[-1][1]['messages'][0])
            self.assertEqual(len(seen[-1][1]['messages'][0]['version_hash']), 64)
            self.assertEqual(store.repair_messages(cfg,'s',[message])['inserted'],1)
            self.assertEqual(seen[-1][1]['messages'][0]['body'],'正文')
        finally:
            server.shutdown(); server.server_close(); thread.join()

    def test_http_transport_auth_and_redirect_never_forward_key(self):
        seen = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_GET(self):
                seen.append((self.path, self.headers.get('Authorization')))
                if self.path.endswith('/status'):
                    self.send_response(200)
                    self.end_headers()
                    self.wfile.write(b'{"legacy_batches":0,"legacy_messages":0}')
                elif self.path.endswith('/verify'):
                    self.send_response(302)
                    self.send_header('Location', '/must-not-follow')
                    self.end_headers()
                else:
                    self.send_response(401)
                    self.end_headers()
            def do_POST(self):
                seen.append((self.path, json.loads(self.rfile.read(int(self.headers['Content-Length'])))))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"state":null}')
        server = HTTPServer(('127.0.0.1', 0), Handler)
        thread = Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            store = HTTPArchiveStore(f'http://127.0.0.1:{server.server_port}', 'k' * 40)
            self.assertEqual(store.legacy_counts(), {'legacy_batches': 0, 'legacy_messages': 0})
            self.assertEqual(seen[0], ('/api/v1/chat-archive-sync/status', 'Bearer ' + 'k' * 40))
            self.assertIsNone(store.get_state('a' * 64))
            self.assertEqual(seen[1][1], {'source_key': 'a' * 64})
            with self.assertRaisesRegex(ValueError, 'archive_redirect_rejected'):
                store.verify()
            self.assertEqual(len(seen), 3)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_oversized_upload_never_advances(self):
        store = HTTPArchiveStore('https://archive.example', 'k' * 40)
        cfg = SyncConfig('device', 'account', 'codex', '/sessions/a.jsonl')
        with patch.object(store, '_request', return_value={'state': None}) as request, patch('http_store.MAX_UPLOAD_BYTES', 100):
            with self.assertRaisesRegex(ValueError, 'archive_upload_too_large'):
                with store.sync_transaction(cfg.key) as tx:
                    tx.get_state(cfg.key)
                    tx.save_batch(cfg, cfg.key, {'offset': 20}, b'', ParsedBatch(conversation_id='c'), 0)
            self.assertEqual(request.call_count, 1)

    def test_upload_is_one_request_after_whole_source_success(self):
        store = HTTPArchiveStore('https://archive.example', 'k' * 40)
        cfg = SyncConfig('device', 'account', 'codex', '/sessions/a.jsonl')
        calls = []
        def request(path, payload=None):
            calls.append((path, payload))
            return {'state': None} if path == '/state' else {'ok': True}
        state = {'offset': 20, 'prefix_sha256': 'a' * 64, 'conversation_id': 'c', 'parser_version': 'dialogue-only-3'}
        with patch.object(store, '_request', side_effect=request):
            with store.sync_transaction(cfg.key) as tx:
                self.assertIsNone(tx.get_state(cfg.key))
                tx.save_batch(cfg, cfg.key, state, b'', ParsedBatch(conversation_id='c'), 0)
                self.assertEqual(len(calls), 1)
            self.assertEqual(calls[-1][0], '/upload')
            self.assertEqual(calls[-1][1]['expected_state'], None)
            self.assertEqual(calls[-1][1]['state'], state)
            self.assertEqual(calls[-1][1]['source_key'], cfg.key)

    def test_source_failure_does_not_upload_or_advance(self):
        store = HTTPArchiveStore('https://archive.example', 'k' * 40)
        cfg = SyncConfig('device', 'account', 'codex', '/sessions/a.jsonl')
        with patch.object(store, '_request', return_value={'state': None}) as request:
            with self.assertRaises(ValueError):
                with store.sync_transaction(cfg.key) as tx:
                    tx.get_state(cfg.key)
                    tx.save_batch(cfg, cfg.key, {'offset': 2}, b'', ParsedBatch(conversation_id='c'), 0)
                    raise ValueError('source changed')
            self.assertEqual(request.call_count, 1)

    def test_rejects_insecure_remote_urls_and_embedded_secrets(self):
        for url in ['http://example.com', 'https://u:p@example.com', 'https://example.com?key=x', 'https://example.com/#x']:
            with self.assertRaises(ValueError):
                HTTPArchiveStore(url, 'k' * 40)

    def test_rejects_raw_source_data(self):
        store = HTTPArchiveStore('https://archive.example', 'k' * 40)
        cfg = SyncConfig('d', 'a', 'codex', '/x.jsonl')
        with patch.object(store, '_request', return_value={'state': None}):
            with self.assertRaises(ValueError):
                with store.sync_transaction(cfg.key) as tx:
                    tx.get_state(cfg.key)
                    tx.save_batch(cfg, cfg.key, {}, b'source bytes', ParsedBatch(conversation_id='c'), 0)


if __name__ == '__main__':
    unittest.main()
