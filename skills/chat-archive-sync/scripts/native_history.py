"""通过 Codex 本机只读 RPC 取得 userMessage 分类；不打开/续写/修改会话。"""
import json
import os
from pathlib import Path
import queue
import shutil
import subprocess
import threading
import time


def read_evidence(call, session):
    records, cursors, turns = [], set(), set()
    cursor = None
    while True:
        page = call('thread/turns/list', {'threadId': session, 'cursor': cursor,
                    'limit': 10, 'sortDirection': 'asc', 'itemsView': 'full'})
        if not isinstance(page.get('data'), list) or 'nextCursor' not in page:
            raise ValueError('archive_native_page_invalid')
        for turn in page['data']:
            if turn.get('itemsView', 'full') != 'full' or not isinstance(turn.get('items'), list):
                raise ValueError('archive_native_summary_not_full')
            if not turn.get('id') or turn['id'] in turns:
                raise ValueError('archive_native_duplicate_turn')
            turns.add(turn['id'])
            for item in turn['items']:
                if item.get('type') == 'userMessage':
                    texts = [c['text'] for c in item.get('content', [])
                             if c.get('type') == 'text' and isinstance(c.get('text'), str)]
                    records.append({'turn_id': turn['id'], 'item_id': item.get('id'), 'texts': texts})
        cursor = page['nextCursor']
        if cursor is None:
            return {'complete': True, 'records': records}
        if not isinstance(cursor, str) or cursor in cursors:
            raise ValueError('archive_native_cursor_invalid')
        cursors.add(cursor)


def default_command():
    executable = shutil.which('codex')
    if not executable and os.name == 'nt':
        # 只定位应用二进制，不扫描配置、密钥或其他项目。
        root = Path(os.environ.get('LOCALAPPDATA', '')) / 'OpenAI' / 'Codex' / 'bin'
        candidates = sorted(root.glob('*/codex.exe'), key=lambda p: p.stat().st_mtime, reverse=True)
        executable = str(candidates[0]) if candidates else None
    if not executable:
        raise ValueError('archive_codex_executable_missing')
    return [executable, 'app-server', 'proxy']


class NativeHistory:
    def __init__(self, command=None):
        self.command, self.process, self.disabled = command, None, False
        self.allow_fallback, self.fallback_used = command is None, False
        self.responses = queue.Queue()
        self.serial = 0

    def __enter__(self): return self

    def __exit__(self, *args): self.close()

    def close(self):
        if self.process:
            self.process.terminate()
            try: self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill(); self.process.wait()
            self.process.stdin.close()
            self.process.stdout.close()
            self.process = None

    def _start(self):
        self.responses = queue.Queue()
        command = self.command or default_command()
        if not isinstance(command, list) or not command or not all(isinstance(v, str) and v for v in command):
            raise ValueError('archive_codex_command_invalid')
        self.process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=subprocess.DEVNULL)
        def reader(stream):
            try:
                while True:
                    line = stream.readline(128 * 1024 * 1024 + 1)
                    if not line or len(line) > 128 * 1024 * 1024:
                        break
                    value = json.loads(line)
                    if 'id' in value: self.responses.put(value)
            except (ValueError, OSError):
                pass
            finally: self.responses.put(None)
        threading.Thread(target=reader, args=(self.process.stdout,), daemon=True).start()
        self.call('initialize', {'clientInfo': {'name':'chat-archive-sync','version':'2.0'},
                                'capabilities': {'experimentalApi':True}})
        self.process.stdin.write(b'{"method":"initialized"}\n'); self.process.stdin.flush()

    def call(self, method, params):
        if method not in ('initialize', 'thread/turns/list'):
            raise ValueError('archive_native_method_not_allowed')
        self.serial += 1
        request_id = self.serial
        self.process.stdin.write((json.dumps({'id':request_id,'method':method,'params':params})+'\n').encode())
        self.process.stdin.flush()
        deadline = time.monotonic() + 45
        while True:
            try: response = self.responses.get(timeout=max(0, deadline-time.monotonic()))
            except queue.Empty: raise ValueError('archive_native_timeout') from None
            if response is None: raise ValueError('archive_native_closed')
            if response.get('id') != request_id: continue
            if 'error' in response: raise ValueError('archive_native_read_failed')
            return response['result']

    def read(self, session):
        if self.disabled: raise ValueError('archive_native_unavailable')
        if not self.process:
            try: self._start()
            except (OSError, ValueError):
                self.close()
                # 没有桌面 daemon 时使用 CLI 的只读历史服务，不恢复/续写会话。
                if self.allow_fallback and not self.fallback_used:
                    try:
                        self.fallback_used = True
                        self.command = default_command()[:-1] + ['--stdio']
                        self._start()
                    except (OSError, ValueError):
                        self.disabled = True
                        self.close()
                else:
                    self.disabled = True
                if self.disabled:
                    raise ValueError('archive_native_unavailable') from None
        try:
            return read_evidence(self.call, session)
        except (ValueError, OSError) as error:
            transport_failed = isinstance(error, OSError)
            if (not self.allow_fallback or self.fallback_used or (not transport_failed and str(error) not in
                    ('archive_native_read_failed','archive_native_closed','archive_native_timeout'))):
                if transport_failed:
                    self.close()
                    raise ValueError('archive_native_transport_failed') from None
                raise
            self.close()
            self.fallback_used = True
            self.command = default_command()[:-1] + ['--stdio']
            try:
                self._start()
                return read_evidence(self.call, session)
            except OSError:
                self.close()
                raise ValueError('archive_native_transport_failed') from None
