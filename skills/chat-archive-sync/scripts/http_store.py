"""仅通过专用 HTTPS 接口归档，不需要数据库驱动或 SSH 凭据。"""
from contextlib import contextmanager
import copy
import json
import hashlib
import time
from urllib.parse import urlsplit
from urllib.request import Request, build_opener, HTTPRedirectHandler
from urllib.error import HTTPError, URLError

MAX_UPLOAD_BYTES = 16 * 1024 * 1024


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # 不把 Authorization 或正文转发到重定向目标。
        raise ValueError('archive_redirect_rejected')


class HTTPArchiveStore:
    def __init__(self, api_url, api_key):
        url = urlsplit(api_url)
        local = url.scheme == 'http' and url.hostname in ('127.0.0.1', 'localhost', '::1')
        if (url.scheme != 'https' and not local) or not url.hostname or url.username or url.password or url.query or url.fragment or url.path not in ('', '/'):
            raise ValueError('archive_api_url_invalid')
        if not isinstance(api_key, str) or len(api_key) < 32 or any(c.isspace() for c in api_key):
            raise ValueError('archive_api_key_invalid')
        self.base = api_url.rstrip('/') + '/api/v1/chat-archive-sync'
        self.api_key = api_key
        self.opener = build_opener(NoRedirect())
        self.pending = None

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.pending = None

    def _request(self, path, payload=None):
        data = None if payload is None else json.dumps(payload, ensure_ascii=False).encode('utf-8')
        if data is not None and len(data) > MAX_UPLOAD_BYTES:
            raise ValueError('archive_upload_too_large')
        req = Request(self.base + path, data=data, headers={'Authorization': 'Bearer ' + self.api_key, 'Content-Type': 'application/json', 'Accept': 'application/json'})
        try:
            with self.opener.open(req, timeout=90) as response:
                body = response.read(1024 * 1024 + 1)
                if len(body) > 1024 * 1024:
                    raise ValueError('archive_response_too_large')
                return json.loads(body)
        except HTTPError as error:
            # 服务器错误页可能含数据，绝不输出原始响应或请求。
            raise ValueError(f'archive_http_{error.code}') from None
        except (URLError, TimeoutError, OSError, json.JSONDecodeError):
            raise ValueError('archive_connection_failed') from None

    @contextmanager
    def sync_transaction(self, key):
        if self.pending is not None:
            raise ValueError('archive_nested_transaction')
        self.pending = {'source_key': key, 'batches': []}
        try:
            yield self
            # sync_source 已检查文件读取前后不变；此时才一次性提交本来源。
            if self.pending['batches']:
                self._request('/upload', self.pending)
        finally:
            self.pending = None

    def get_state(self, key):
        state = self._request('/state', {'source_key': key})['state']
        if self.pending is not None:
            self.pending['expected_state'] = copy.deepcopy(state)
        return state

    def legacy_counts(self):
        return self._reconcile_request('/status', None)

    def save_batch(self, config, key, state, raw, parsed, offset):
        if raw or self.pending is None or self.pending['source_key'] != key:
            raise ValueError('archive_invalid_batch')
        for m in parsed.messages:
            if m['role'] not in ('user', 'assistant') or m['blocks'] or m['attachments'] or not m['body'].strip():
                raise ValueError('archive_text_only')
        batch = {'start_offset': offset, 'conversation_id': parsed.conversation_id, 'messages': parsed.messages, 'unknown_events': parsed.unknown}
        # 有界纯文字缓冲，不缓存源文件；超限失败，不推进进度。
        self.pending.update(device_id=config.device_id, account_scope=config.account_scope, source=config.source, source_path=config.path, state=copy.deepcopy(state))
        self.pending['batches'].append(batch)
        if len(json.dumps(self.pending, ensure_ascii=False).encode('utf-8')) > MAX_UPLOAD_BYTES:
            raise ValueError('archive_upload_too_large')

    def record_failure(self, key):
        # 断网不能可靠记录远端失败；CLI 明确返回非零，不冒充上传成功。
        pass

    def verify(self):
        result = self._reconcile_request('/verify', None)
        if result.get('verified') is not True:
            raise ValueError('archive_verification_failed')
        return result

    def _reconcile_request(self, path, payload):
        # check只读，repair按消息身份/版本幂等；响应丢失后可安全有限重试。
        for attempt in range(3):
            try:
                result = self._request(path, payload)
                if attempt and path == '/repair': result['retried'] = True
                return result
            except ValueError as error:
                if str(error) not in ('archive_http_429', 'archive_http_500', 'archive_http_502',
                                      'archive_http_503', 'archive_http_504', 'archive_connection_failed') or attempt == 2:
                    raise
                time.sleep(attempt + 1)

    def check_messages(self, config, conversation_id, messages):
        fingerprints = []
        for message in messages:
            value = {k: v for k, v in message.items() if k != 'position'}
            digest = hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False).encode('utf-8')).hexdigest()
            fingerprints.append({'message_key': message['message_key'], 'version_hash': digest})
        result = self._reconcile_request('/check', {'source': config.source,
            'account_scope': config.account_scope, 'conversation_id': conversation_id, 'messages': fingerprints})
        statuses = result.get('statuses')
        if not isinstance(statuses, list) or len(statuses) != len(messages) or any(s not in ('present','missing','different') for s in statuses):
            raise ValueError('archive_check_response_invalid')
        return statuses

    def repair_messages(self, config, conversation_id, messages):
        result = self._reconcile_request('/repair', {'source_key': config.key,
            'device_id': config.device_id, 'account_scope': config.account_scope,
            'source': config.source, 'source_path': config.path,
            'conversation_id': conversation_id, 'messages': messages})
        if type(result.get('inserted')) is not int or not 0 <= result['inserted'] <= len(messages) or result.get('verified') != len(messages):
            raise ValueError('archive_repair_response_invalid')
        return result
