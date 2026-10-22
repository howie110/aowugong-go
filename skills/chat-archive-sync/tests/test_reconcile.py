import copy
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))
from sync import SyncConfig


def event(mid, text, kinds=None, role='user'):
    p = {'id': mid, 'type': 'message', 'role': role,
         'content': [{'type': 'input_text', 'text': text}]}
    if kinds is not None:
        p['internal_chat_message_metadata_passthrough'] = {'content_item_kinds': kinds}
    if role == 'assistant':
        p['phase'] = 'final_answer'
    return {'type': 'response_item', 'timestamp': '2026-07-31T01:00:00Z', 'payload': p}


class ReconcileTests(unittest.TestCase):
    def test_unknown_nonempty_classification_is_reported_not_silently_excluded(self):
        import reconcile
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'s.jsonl'
            assistant=event('a','unknown',role='assistant');assistant['payload']['phase']='new-unknown-phase'
            records=[{'type':'session_meta','payload':{'id':'s'}}, assistant,
                     event('u','unknown',['new.unknown']),event('injection','known',['agents_md.instructions']),
                     event('real','real',['user.text'])]
            path.write_text(''.join(json.dumps(e)+'\n' for e in records))
            scan=reconcile.scan_source(SyncConfig('d','a','codex',str(path)))
            self.assertEqual(scan.unclassified,2)
            self.assertEqual([m['message_key'] for m in scan.messages],['id:real'])

    def test_unknown_assistant_phase_and_claude_injected_or_tool_text_not_claimed_complete(self):
        import reconcile
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/'s.jsonl'
            unknown=event('a','未知阶段',role='assistant'); del unknown['payload']['phase']
            path.write_text(json.dumps({'type':'session_meta','payload':{'id':'s'}})+'\n'+json.dumps(unknown)+'\n')
            result=reconcile.scan_source(SyncConfig('d','a','codex',str(path)))
            self.assertEqual(result.messages,[])
            self.assertEqual(result.unclassified,1)
            records=[
                {'type':'user','sessionId':'s','uuid':'injected','isMeta':True,'message':{'role':'user','content':'system instruction'}},
                {'type':'assistant','sessionId':'s','uuid':'tool','message':{'role':'assistant','content':[{'type':'text','text':'progress'},{'type':'tool_use','id':'x'}]}},
                {'type':'user','sessionId':'s','uuid':'human','message':{'role':'user','content':'hello'}},
                {'type':'assistant','sessionId':'s','uuid':'final','message':{'role':'assistant','stop_reason':'end_turn','content':[{'type':'text','text':'done'}]}},
            ]
            path.write_text(''.join(json.dumps(e)+'\n' for e in records))
            result=reconcile.scan_source(SyncConfig('d','a','claude-code',str(path)))
            self.assertEqual([m['body'] for m in result.messages],['hello','done'])

    def test_historical_user_requires_native_evidence_and_keeps_repeated_text(self):
        # Old parser drops both real legacy users; the new scanner must recover
        # them without absorbing an injected AGENTS message or heartbeat.
        import reconcile
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'source.jsonl'
            records = [{'type': 'session_meta', 'payload': {'id': 'session'}},
                       event('u1', '相同文字'), event('u2', '相同文字'),
                       event('sys', '# AGENTS.md instructions for project'),
                       event('a', '回复', role='assistant'),
                       event('auto', '<heartbeat><current_time_iso>x</current_time_iso><instructions>x</instructions></heartbeat>')]
            path.write_text(''.join(json.dumps(x, ensure_ascii=False) + '\n' for x in records))
            cfg = SyncConfig('device', 'account', 'codex', str(path))
            native = {'complete': True, 'records': [
                {'item_id': 'n1', 'turn_id': 't1', 'texts': ['相同文字']},
                {'item_id': 'n2', 'turn_id': 't2', 'texts': ['相同文字']},
                {'item_id': 'n3', 'turn_id': 't3', 'texts': [records[-1]['payload']['content'][0]['text']]}]}
            result = reconcile.scan_source(cfg, lambda sid: native)
            self.assertEqual([m['message_key'] for m in result.messages], ['id:u1', 'id:u2', 'id:a'])
            self.assertEqual(result.recovered, 2)
            self.assertEqual(result.automatic_excluded, 1)
            self.assertEqual(result.unclassified, 1)
            partial = reconcile.scan_source(cfg, lambda sid: {**native, 'complete': False})
            self.assertEqual([m['message_key'] for m in partial.messages], ['id:a'])

    def test_confirmed_messages_upload_despite_unclassified_ones_and_rerun_is_zero(self):
        import reconcile
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'source.jsonl'
            records = [{'type': 'session_meta', 'payload': {'id': 'session'}},
                       event('u', '确实是用户', ['user.text']), event('x', '待确认')]
            path.write_text(''.join(json.dumps(x) + '\n' for x in records))
            cfg = SyncConfig('d', 'a', 'codex', str(path))
            class Store:
                def __init__(self): self.messages = []
                def check_messages(self, cfg, sid, messages):
                    return ['present' if m in self.messages else 'missing' for m in messages]
                def repair_messages(self, cfg, sid, messages):
                    self.messages.extend(copy.deepcopy(messages))
                    return {'inserted': len(messages), 'verified': len(messages)}
            store = Store()
            self.assertEqual(reconcile.reconcile_source(cfg, store, None, True)['inserted'], 0)
            self.assertEqual(store.messages, [])
            first = reconcile.reconcile_source(cfg, store, None)
            self.assertEqual(first['inserted'], 1)
            self.assertEqual(first['verified'], 1)
            self.assertEqual(first['unclassified'], 1)
            self.assertEqual(reconcile.reconcile_source(cfg, store, None)['inserted'], 0)

    def test_cloud_missing_after_repair_is_not_success(self):
        import reconcile
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / 's.jsonl'
            p.write_text(json.dumps({'type':'session_meta','payload':{'id':'s'}})+'\n'+json.dumps(event('u','hi',['user.text']))+'\n')
            class Broken:
                def check_messages(self, *args): return ['missing']
                def repair_messages(self, *args): return {'inserted': 1, 'verified': 1}
            with self.assertRaisesRegex(ValueError, 'archive_reconcile_mismatch'):
                reconcile.reconcile_source(SyncConfig('d','a','codex',str(p)), Broken(), None)


if __name__ == '__main__': unittest.main()
