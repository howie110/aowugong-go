"""全量只读扫描、云端逐条核对和幂等补齐；不依赖旧增量游标。"""
from collections import Counter
from dataclasses import dataclass, field
import hashlib
import json
from pathlib import Path
import re

from adapters import parse_records
from sync import MAX_LINE_BYTES, open_source


def text_hash(text):
    return hashlib.sha256(text.encode('utf-8', errors='surrogatepass')).hexdigest()


def automatic(text):
    text = text.strip()
    return bool(text.startswith('<heartbeat>') and text.endswith('</heartbeat>')
                and re.search(r'<current_time_iso>[\s\S]*?</current_time_iso>', text, re.I)
                and re.search(r'<instructions>[\s\S]*?</instructions>', text, re.I))


def known_exclusion(kind):
    return isinstance(kind, str) and (kind.startswith(('agents_md.', 'environments.', 'skills.', 'multi_agent.'))
            or kind in ('user.image', 'user.audio', 'user.file', 'developer.instructions', 'system.instructions'))


def events(path):
    offset = 0
    with open_source(path, complete=True) as stream:
        while True:
            line = stream.readline(MAX_LINE_BYTES + 1)
            if len(line) > MAX_LINE_BYTES:
                raise ValueError('archive_event_too_large')
            if not line:
                break
            if not line.endswith(b'\n'):
                if str(path).endswith('.zst'):
                    raise ValueError('archive_compressed_incomplete_tail')
                break
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError('archive_invalid_event')
            yield offset, line, event
            offset += len(line)


def snapshot(path):
    st = Path(path).stat()
    return st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns


def user_blocks(event):
    p = event.get('payload', {})
    if event.get('type') != 'response_item' or p.get('type') != 'message' or p.get('role') != 'user':
        return []
    content = p.get('content', [])
    return [(i, b['text']) for i, b in enumerate(content) if isinstance(b, dict)
            and b.get('type') in ('text', 'input_text', 'output_text')
            and isinstance(b.get('text'), str) and b['text'].strip()]


@dataclass
class Scan:
    conversation_id: str | None = None
    messages: list = field(default_factory=list)
    recovered: int = 0
    unclassified: int = 0
    automatic_excluded: int = 0
    native_status: str = 'not_needed'
    native_error: str | None = None
    skipped: str | None = None


def scan_source(config, native_reader=None):
    """无来源标记的旧 user 块仅接受原生 userMessage 的身份/次数证据。"""
    before = snapshot(config.path)
    result = Scan()
    raw, raw_id, raw_turn = Counter(), Counter(), Counter()
    needs_native = False
    for _, _, e in events(config.path):
        p = e.get('payload', {})
        if config.source == 'codex' and e.get('type') == 'session_meta':
            origin = p.get('source')
            if origin == 'subagent' or isinstance(origin, dict) and 'subagent' in origin:
                result.skipped = 'subagent_session'
                return result
            result.conversation_id = p.get('id')
        if config.source != 'codex':
            continue
        meta = p.get('internal_chat_message_metadata_passthrough') or {}
        if not isinstance(meta, dict):
            meta = {}
        for _, text in user_blocks(e):
            h = text_hash(text)
            raw[h] += 1
            raw_id[p.get('id'), h] += 1
            raw_turn[meta.get('turn_id'), h] += 1
            if meta.get('content_item_kinds') is None and not automatic(text):
                needs_native = True
    native, by_id, by_turn = Counter(), Counter(), Counter()
    complete = False
    if needs_native:
        result.native_status = 'unavailable'
        if native_reader and result.conversation_id:
            try:
                evidence = native_reader(result.conversation_id)
                complete = evidence['complete'] is True
                for record in evidence['records']:
                    for text in record['texts']:
                        h = text_hash(text)
                        native[h] += 1
                        by_id[record.get('item_id'), h] += 1
                        by_turn[record.get('turn_id'), h] += 1
                result.native_status = 'complete' if complete else 'partial'
            except (ValueError, OSError, TimeoutError) as error:
                code = str(error)
                result.native_error = code if code.startswith('archive_native_') and code.replace('_','').isalnum() else 'archive_native_unavailable'
    for pos, line, e in events(config.path):
        if config.source == 'codex':
            p = e.get('payload', {})
            if e.get('type') == 'response_item' and p.get('type') == 'message' and p.get('role') == 'assistant':
                phase, channel = p.get('phase'), p.get('channel')
                if ((not phase and not channel) or phase not in (None, 'final_answer', 'commentary', 'analysis')
                        or channel not in (None, 'final', 'commentary', 'analysis')):
                    result.unclassified += 1
            blocks = user_blocks(e)
            if blocks:
                meta = p.get('internal_chat_message_metadata_passthrough') or {}
                meta = meta if isinstance(meta, dict) else {}
                kinds = meta.get('content_item_kinds')
                content = p.get('content', [])
                valid = isinstance(kinds, list) and len(kinds) == len(content)
                chosen = list(kinds) if valid else ['archive.excluded'] * len(content)
                recovered = False
                for index, text in blocks:
                    if automatic(text):
                        chosen[index] = 'archive.excluded'
                        result.automatic_excluded += 1
                        continue
                    if valid:
                        if chosen[index] != 'user.text' and not known_exclusion(chosen[index]):
                            result.unclassified += 1
                        continue
                    h, mid, tid = text_hash(text), p.get('id'), meta.get('turn_id')
                    trusted = kinds is None and (
                        (mid and by_id[mid, h] > 0 and by_id[mid, h] == raw_id[mid, h])
                        or (tid and by_turn[tid, h] > 0 and by_turn[tid, h] == raw_turn[tid, h])
                        or (complete and native[h] > 0 and native[h] == raw[h]))
                    if trusted:
                        chosen[index] = 'user.text'
                        recovered = True
                    else:
                        result.unclassified += 1
                p['internal_chat_message_metadata_passthrough'] = {**meta, 'content_item_kinds': chosen}
                line = (json.dumps(e, ensure_ascii=True) + '\n').encode()
                result.recovered += int(recovered)
        else:
            # Claude 明确标记的元消息、工具处理中间文字和子任务不属于真人对话。
            if e.get('isSidechain'):
                result.skipped = 'subagent_session'
                result.messages.clear()
                return result
            if e.get('isMeta') or e.get('isCompactSummary'):
                continue
            message = e.get('message', {})
            if e.get('type') == 'assistant' and isinstance(message, dict):
                content = message.get('content', [])
                if isinstance(content, list) and any(b.get('type') == 'tool_use' for b in content if isinstance(b, dict)):
                    continue
                if message.get('stop_reason') != 'end_turn':
                    if isinstance(content, list) and any(b.get('type') == 'text' for b in content if isinstance(b, dict)):
                        result.unclassified += 1
                    continue
        parsed = parse_records(config.source, line, pos, config.key, result.conversation_id)
        result.conversation_id = parsed.conversation_id
        result.messages.extend(parsed.messages)
    if snapshot(config.path) != before:
        raise ValueError('archive_source_changed')
    if not result.conversation_id:
        result.skipped = 'empty_source' if before[1] == 0 else None
        if not result.skipped:
            raise ValueError('archive_session_identity_missing')
    return result


def chunks(messages):
    """按条数和实际编码大小拆分，不因整会话超过16MiB而阻止上传。"""
    batch, size = [], 0
    for m in messages:
        length = len(json.dumps(m, ensure_ascii=False).encode('utf-8')) + 2
        if length > 14 * 1024 * 1024:
            raise ValueError('archive_single_message_too_large')
        if batch and (len(batch) >= 500 or size + length > 14 * 1024 * 1024):
            yield batch
            batch, size = [], 0
        batch.append(m)
        size += length
    if batch:
        yield batch


class PartialSyncError(ValueError):
    def __init__(self, error, report):
        super().__init__(str(error))
        self.report = report


def reconcile_source(config, store, native_reader=None, dry_run=False):
    scan = scan_source(config, native_reader)
    report = {'source_key': config.key, 'conversation_id': scan.conversation_id,
              'confirmed': len(scan.messages), 'user': sum(m['role'] == 'user' for m in scan.messages),
              'assistant': sum(m['role'] == 'assistant' for m in scan.messages),
              'recovered': scan.recovered, 'unclassified': scan.unclassified,
              'automatic_excluded': scan.automatic_excluded, 'native_status': scan.native_status,
              'native_error': scan.native_error,
              'inserted': 0, 'restored': 0, 'retried_repair_batches': 0,
              'verified': 0, 'missing_before': 0, 'different_before': 0,
              'dry_run': dry_run, 'skipped': scan.skipped}
    try:
        for batch in chunks(scan.messages):
            statuses = store.check_messages(config, scan.conversation_id, batch)
            report['missing_before'] += statuses.count('missing')
            report['different_before'] += statuses.count('different')
            missing = [m for m, status in zip(batch, statuses) if status != 'present']
            if missing and not dry_run:
                result = store.repair_messages(config, scan.conversation_id, missing)
                report['inserted'] += result['inserted']
                report['retried_repair_batches'] += int(result.get('retried', False))
            if not dry_run:
                after = store.check_messages(config, scan.conversation_id, batch)
                if after != ['present'] * len(batch):
                    raise ValueError('archive_reconcile_mismatch')
                report['verified'] += len(batch)
                report['restored'] += len(missing)
            else:
                report['verified'] += statuses.count('present')
    except Exception as error:
        raise PartialSyncError(error, report) from None
    return report
