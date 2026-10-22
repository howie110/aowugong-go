"""显式配置、云端进度的单次同步入口；不自动发现凭证或启动隧道。"""
import argparse
from dataclasses import dataclass
from contextlib import contextmanager
import glob
import hashlib
import json
import os
from pathlib import Path
import sys
import subprocess
import sqlite3
import fnmatch

from adapters import PARSER_VERSION, ParsedBatch, parse_records

# 限制单个异常事件而非整份日志；批次只累积筛选后的文字。
MAX_LINE_BYTES = 128 * 1024 * 1024
BATCH_MESSAGES = 500


@dataclass(frozen=True)
class SyncConfig:
    device_id: str
    account_scope: str
    source: str
    path: str

    @property
    def key(self):
        path = self.path[:-4] if self.path.endswith('.jsonl.zst') else self.path
        return hashlib.sha256(json.dumps([self.device_id, self.account_scope, self.source, path]).encode()).hexdigest()


@contextmanager
def open_source(path, complete=False):
    """解压只流入内存，不落盘；完整读取时必须检查解压器退出码。"""
    if not str(path).endswith('.zst'):
        with open(path, 'rb') as stream:
            yield stream
        return
    process = subprocess.Popen(['zstd', '-q', '-d', '-c', '--', str(path)], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    try:
        yield process.stdout
        if complete and process.wait() != 0:
            raise ValueError('compressed source is invalid')
    finally:
        process.stdout.close()
        if process.poll() is None:
            process.terminate()
        process.wait()


def prefix_hash(path, size):
    digest = hashlib.sha256()
    with open_source(path) as stream:
        remaining = size
        while remaining:
            part = stream.read(min(remaining, 1024 * 1024))
            if not part:
                return None
            digest.update(part)
            remaining -= len(part)
    return digest.hexdigest()


def discover_paths(pattern):
    """同一逻辑日志在压缩前后只处理一次，优先仍在写入的普通文件。"""
    candidates = glob.glob(pattern, recursive=True)
    if pattern.endswith('.jsonl'):
        candidates += glob.glob(pattern + '.zst', recursive=True)
    found = {}
    for path in sorted(candidates):
        if not path.endswith(('.jsonl', '.jsonl.zst')) or not Path(path).is_file():
            continue
        canonical = path[:-4] if path.endswith('.zst') else path
        if canonical not in found or not path.endswith('.zst'):
            found[canonical] = path
    return sorted(found.values())


def check_codex_index(index_path, sources):
    """只读 Agent 自己的会话索引，核对覆盖率；这不是本地同步状态库。"""
    patterns = [entry['path'].removesuffix('.zst').replace('\\', '/') for entry in sources if entry['source'] == 'codex']
    missing, expected, located = [], 0, 0
    with sqlite3.connect(Path(index_path).as_uri() + '?mode=ro', uri=True) as db:
        for session_id, path, origin in db.execute('SELECT id,rollout_path,source FROM threads'):
            try:
                source = json.loads(origin)
            except (ValueError, TypeError):
                source = origin
            if source == 'subagent' or isinstance(source, dict) and 'subagent' in source:
                continue
            match_path = path.replace('\\', '/')
            if not any(fnmatch.fnmatchcase(match_path, pattern) or fnmatch.fnmatchcase(match_path, pattern.replace('/**/', '/')) for pattern in patterns):
                continue
            expected += 1
            if Path(path).is_file() or Path(path + '.zst').is_file():
                located += 1
            else:
                missing.append(session_id)
    return {'expected_sessions': expected, 'located_sessions': located, 'missing_session_ids': sorted(missing)}


def sync_source(config, store, dry_run=False):
    """逐行解析、分批写入同一事务；读取期间变化或解压失败则整体回滚。"""
    with store.sync_transaction(config.key) as tx:
        previous = tx.get_state(config.key)
        path = Path(config.path)
        before = path.stat()
        if config.source == 'codex':
            with open_source(path) as stream:
                first_line = stream.readline(MAX_LINE_BYTES + 1)
            if len(first_line) > MAX_LINE_BYTES:
                raise ValueError('single event exceeds memory safety limit')
            header = json.loads(first_line)
            origin = header.get('payload', {}).get('source') if header.get('type') == 'session_meta' else None
            if origin == 'subagent' or isinstance(origin, dict) and 'subagent' in origin:
                return {'source_key': config.key, 'messages': 0, 'bytes': 0, 'skipped': 'subagent_session', 'dry_run': dry_run}
        offset = previous["offset"] if previous else 0
        unchanged = previous and previous.get("parser_version") == PARSER_VERSION and prefix_hash(path, offset) == previous['prefix_sha256']
        if not unchanged:
            offset = 0
        session = previous["conversation_id"] if unchanged else None
        digest = hashlib.sha256()
        end, batch_start = 0, offset
        batch = ParsedBatch(conversation_id=session)
        result = {'source_key': config.key, 'bytes': 0, 'messages': 0, 'unknown_events': 0, 'dry_run': dry_run}
        state = previous
        with open_source(path, complete=True) as stream:
            while True:
                line = stream.readline(MAX_LINE_BYTES + 1)
                if len(line) > MAX_LINE_BYTES:
                    raise ValueError('single event exceeds memory safety limit')
                if line and not line.endswith(b'\n') and str(path).endswith('.zst'):
                    raise ValueError('compressed source has incomplete final record')
                if not line or not line.endswith(b'\n'):
                    break
                start = end
                end += len(line)
                digest.update(line)
                if end <= offset:
                    continue
                if start < offset:
                    raise ValueError('invalid source cursor')
                parsed = parse_records(config.source, line, start, config.key, conversation_id=batch.conversation_id)
                batch.conversation_id = parsed.conversation_id
                batch.messages.extend(parsed.messages)
                batch.unknown += parsed.unknown
                result['bytes'] += len(line)
                result['messages'] += len(parsed.messages)
                result['unknown_events'] += parsed.unknown
                state = {'offset': end, 'prefix_sha256': digest.hexdigest(), 'conversation_id': batch.conversation_id, 'parser_version': PARSER_VERSION}
                if len(batch.messages) >= BATCH_MESSAGES:
                    if not dry_run:
                        tx.save_batch(config, config.key, state, b'', batch, batch_start)
                    batch = ParsedBatch(conversation_id=batch.conversation_id)
                    batch_start = end
        if not batch.conversation_id:
            raise ValueError('unsupported source: missing session identity')
        after = path.stat()
        if (before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns):
            raise ValueError('source changed during read; retry later')
        if not dry_run and state is not None and (end > batch_start or state != previous) and (batch.messages or end > batch_start):
            tx.save_batch(config, config.key, state, b'', batch, batch_start)
        return result


def load_config(path):
    path = Path(path)
    if os.name != "nt" and path.stat().st_mode & 0o077:
        raise ValueError("private config must have mode 600")
    config = json.loads(path.read_text(encoding='utf-8'))
    for key in ("device_id", "account_scope"):
        if not isinstance(config.get(key), str) or not config[key].strip():
            raise ValueError(f"missing config: {key}")
    if config.get('api_url'):
        from http_store import HTTPArchiveStore
        HTTPArchiveStore(config['api_url'], config.get('api_key'))
    elif not isinstance(config.get('dsn'), str) or not config['dsn'].strip():
        raise ValueError('missing api_url/api_key')
    if not isinstance(config.get("sources"), list) or not config["sources"]:
        raise ValueError("sources must explicitly list source/path entries")
    for entry in config["sources"]:
        if entry.get("source") not in ("codex", "claude-code") or not isinstance(entry.get("path"), str) or not Path(entry["path"]).is_absolute():
            raise ValueError("unsupported source or non-absolute path")
    if config.get('codex_index') is not None and (not isinstance(config['codex_index'], str) or not Path(config['codex_index']).is_absolute()):
        raise ValueError('codex_index must be an explicit absolute path')
    return config


def main():
    parser = argparse.ArgumentParser(description="Archive dialogue text from configured JSONL / JSONL.ZST sources; never alters source records.")
    parser.add_argument("--config", required=True, help="private JSON file (mode 600): api_url, api_key, device_id, account_scope, sources [{source,path}]")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--dry-run", action="store_true", help="read cloud progress and sources without writes")
    mode.add_argument("--verify", action="store_true", help="HTTPS: compare all confirmed local messages with cloud without uploads; legacy DSN: integrity only")
    args = parser.parse_args()
    try:
        config = load_config(args.config)
        if config.get('api_url'):
            from archive_run import run
            return run(config, args.dry_run, args.verify)
        else:
            from store import ArchiveStore
            archive = ArchiveStore(config['dsn'])
        with archive as store:
            legacy = store.legacy_counts()
            if any(legacy.values()):
                print(json.dumps({'error': 'legacy_archive_requires_cleanup', **legacy}))
                return 1
            if args.verify:
                print(json.dumps(store.verify()))
                return 0
            failures = 0
            if config.get('codex_index'):
                coverage = check_codex_index(config['codex_index'], config['sources'])
                print(json.dumps({'kind': 'source_coverage', **coverage}))
                failures += bool(coverage['missing_session_ids'])
            seen = set()
            for entry in config["sources"]:
                paths = discover_paths(entry["path"])
                if not paths:
                    print(json.dumps({'skipped': 'empty_source_pattern', 'source': entry['source']}))
                for path in paths:
                    source = SyncConfig(config["device_id"], config["account_scope"], entry["source"], str(Path(path).resolve()))
                    if source.key in seen:
                        continue
                    seen.add(source.key)
                    try:
                        try:
                            result = sync_source(source, store, args.dry_run)
                        except ValueError as error:
                            if str(error) not in ('archive_http_409', 'source changed during read; retry later'):
                                raise
                            # 最多重试一次；重新读取进度和源文件，不能盲目覆盖旧游标。
                            result = sync_source(source, store, args.dry_run)
                        print(json.dumps(result))
                    except Exception:
                        failures += 1
                        if not args.dry_run:
                            store.record_failure(source.key)
                        print(json.dumps({"source_key": source.key, "error": "sync_failed", "action": "check source stability/format, decompressor, upload size, API key and connectivity; retry stale progress once"}))
            return 1 if failures or not seen else 0
    except Exception as error:
        # 数据库错误可能包含凭证或 INSERT 正文，只报告固定错误码。
        code = str(error)
        if not code.startswith('archive_') or len(code) >= 100 or not all(c.isalnum() or c == '_' for c in code):
            code = 'configuration_or_connection_failed'
        print(json.dumps({"error": code, "action": "check explicit private config, API availability and dedicated key"}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
