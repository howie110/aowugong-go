import contextlib
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'scripts'))
from archive_run import run


class API:
    def __init__(self): self.saved=[]; self.fail_second=False; self.fail_verify=False
    def __enter__(self): return self
    def __exit__(self,*args): pass
    def legacy_counts(self): return {'legacy_batches':0,'legacy_messages':0}
    def check_messages(self,cfg,sid,messages):
        if self.fail_second and messages[0]['message_key']=='id:500': raise ValueError('archive_http_503')
        return ['present' if m in self.saved else 'missing' for m in messages]
    def repair_messages(self,cfg,sid,messages):
        self.saved.extend(messages); return {'inserted':len(messages),'verified':len(messages)}
    def verify(self):
        if self.fail_verify: raise ValueError('archive_http_503')
        return {'verified':True}


class RunTests(unittest.TestCase):
    def fixture(self,root,n=1):
        p=root/'s.jsonl'
        records=[{'type':'session_meta','payload':{'id':'s'}}]
        records += [{'type':'response_item','payload':{'type':'message','id':str(i),'role':'user',
           'internal_chat_message_metadata_passthrough':{'content_item_kinds':['user.text']},
           'content':[{'type':'text','text':str(i)}]}} for i in range(n)]
        p.write_text(''.join(json.dumps(e)+'\n' for e in records))
        return {'api_url':'https://example.com','api_key':'k'*40,'device_id':'d','account_scope':'a',
                'sources':[{'source':'codex','path':str(p)}]}

    def execute(self,cfg,api):
        output=io.StringIO()
        with patch('archive_run.HTTPArchiveStore',return_value=api),contextlib.redirect_stdout(output):
            code=run(cfg)
        return code,[json.loads(line) for line in output.getvalue().splitlines()]

    def test_missing_pattern_is_visible_and_not_full_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg=self.fixture(Path(tmp));cfg['sources'].append({'source':'codex','path':str(Path(tmp)/'missing.jsonl')})
            code,rows=self.execute(cfg,API())
            self.assertEqual(code,2)
            self.assertTrue(rows[-1]['unresolved_source_evidence'])
            self.assertTrue(any(r.get('kind')=='source_unavailable' for r in rows))

    def test_only_missing_sources_is_incomplete_not_execution_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg=self.fixture(Path(tmp));Path(cfg['sources'][0]['path']).unlink()
            code,rows=self.execute(cfg,API())
            self.assertEqual(code,2)
            self.assertFalse(rows[-1]['confirmed_records_verified'])
            self.assertTrue(rows[-1]['unresolved_source_evidence'])

    def test_failed_later_batch_keeps_successful_counts(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg=self.fixture(Path(tmp),501);api=API();api.fail_second=True
            code,rows=self.execute(cfg,api)
            self.assertEqual(code,1)
            self.assertEqual(rows[-1]['inserted'],500)
            self.assertEqual(rows[-1]['verified'],500)
            self.assertEqual(rows[-1]['failed_sources'],1)

    def test_integrity_error_still_produces_summary_with_completed_work(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg=self.fixture(Path(tmp));api=API();api.fail_verify=True
            code,rows=self.execute(cfg,api)
            self.assertEqual(code,1)
            self.assertEqual(rows[-1]['kind'],'summary')
            self.assertEqual(rows[-1]['inserted'],1)
            self.assertEqual(rows[-1]['integrity_error'],'archive_http_503')


if __name__=='__main__': unittest.main()
