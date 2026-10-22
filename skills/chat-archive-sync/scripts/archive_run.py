"""默认 HTTPS 完整核对入口：确认的对话自动补齐，例外单独报告。"""
from collections import Counter
import hashlib
import json
from pathlib import Path

from http_store import HTTPArchiveStore
from native_history import NativeHistory
from reconcile import reconcile_source, PartialSyncError
from sync import SyncConfig, check_codex_index, discover_paths


def safe_error(error):
    code = str(error)
    # 固定代码，不把异常中的源正文、路径、HTTP错误页或密钥输出。
    if code.startswith('archive_') and len(code) < 100 and all(c.isalnum() or c == '_' for c in code):
        return code
    return 'archive_source_failed'


def run(config, dry_run=False, verify_only=False):
    count_fields = ('confirmed','user','assistant','recovered','unclassified','automatic_excluded',
                    'inserted','restored','retried_repair_batches','verified','missing_before','different_before')
    totals = Counter({name:0 for name in count_fields})
    seen = set()
    with HTTPArchiveStore(config['api_url'], config['api_key']) as store, NativeHistory(config.get('codex_command')) as native:
        if any(store.legacy_counts().values()):
            raise ValueError('archive_legacy_conflict')
        if config.get('codex_index'):
            coverage = check_codex_index(config['codex_index'], config['sources'])
            print(json.dumps({'kind':'source_coverage', **coverage}), flush=True)
            totals['missing_sources'] = len(coverage['missing_session_ids'])
        for entry in config['sources']:
            paths = discover_paths(entry['path'])
            if not paths:
                totals['empty_patterns'] += 1
                print(json.dumps({'kind':'source_unavailable','source':entry['source'],
                    'pattern_key':hashlib.sha256(entry['path'].encode()).hexdigest(),'reason':'empty_source_pattern'}),flush=True)
            for path in paths:
                source = SyncConfig(config['device_id'], config['account_scope'], entry['source'], str(Path(path).resolve()))
                if source.key in seen: continue
                seen.add(source.key)
                try:
                    for attempt in range(2):
                        try:
                            result = reconcile_source(source, store, native.read, dry_run or verify_only)
                            break
                        except ValueError as error:
                            if str(error) != 'archive_source_changed' or attempt:
                                raise
                    for name in count_fields:
                        totals[name] += result[name]
                    if result['native_status'] in ('unavailable','partial'):
                        totals['native_unavailable_sources'] += 1
                    if result['skipped']: totals[result['skipped']] += 1
                    else: totals['checked_sources'] += 1
                    print(json.dumps({'kind':'source_result', **result}), flush=True)
                except Exception as error:
                    if isinstance(error, PartialSyncError):
                        for name in count_fields: totals[name] += error.report[name]
                        print(json.dumps({'kind':'partial_source_result',**error.report}),flush=True)
                    totals['failed_sources'] += 1
                    print(json.dumps({'source_key':source.key,'error':safe_error(error)}), flush=True)
        unresolved = bool(totals['unclassified'] or totals['native_unavailable_sources'] or totals['missing_sources'] or totals['empty_patterns'] or not totals['checked_sources'])
        failed = bool(totals['failed_sources'])
        remaining = totals['missing_before'] + totals['different_before'] if dry_run or verify_only else 0
        integrity = None
        integrity_error = None
        if not dry_run and not failed:
            try: integrity = store.verify()['verified']
            except Exception as error:
                integrity_error = safe_error(error)
                failed = True
        print(json.dumps({'kind':'summary', 'mode':'preview' if dry_run else 'verify' if verify_only else 'sync',
                          **totals, 'confirmed_records_verified': not failed and remaining == 0 and bool(totals['checked_sources']),
                          'cloud_integrity_verified': integrity,
                          'integrity_error': integrity_error,
                          'unresolved_source_evidence': unresolved,
                          'scope':'configured_readable_local_sources_only'}), flush=True)
        return 1 if failed else 2 if unresolved or remaining else 0
