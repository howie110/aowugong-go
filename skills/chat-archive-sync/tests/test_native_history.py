import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))


class NativeHistoryTests(unittest.TestCase):
    def test_broken_proxy_pipe_uses_fallback_instead_of_repeating_failure(self):
        from native_history import NativeHistory
        from unittest.mock import patch
        class BrokenProxy(NativeHistory):
            starts=0
            def _start(self): self.starts+=1;self.process=object()
            def close(self): self.process=None
            def call(self,*args):
                if self.starts==1: raise BrokenPipeError('proxy stopped')
                return {'data':[],'nextCursor':None}
        with patch('native_history.default_command',return_value=['codex','app-server','proxy']):
            reader=BrokenProxy()
            self.assertTrue(reader.read('s')['complete'])
            self.assertTrue(reader.read('s2')['complete'])
            self.assertEqual(reader.starts,2)

    def test_proxy_history_failure_falls_back_once_even_after_successful_initialize(self):
        from native_history import NativeHistory
        from unittest.mock import patch
        class FakeProcessHistory(NativeHistory):
            starts=0
            def _start(self): self.starts+=1; self.process=object()
            def close(self): self.process=None
            def call(self,*args):
                if self.starts==1: raise ValueError('archive_native_read_failed')
                return {'data':[],'nextCursor':None}
        with patch('native_history.default_command',return_value=['codex','app-server','proxy']):
            reader=FakeProcessHistory()
            self.assertTrue(reader.read('s')['complete'])
            self.assertEqual(reader.starts,2)

    def test_all_pages_full_items_only_users_and_no_truncated_claim(self):
        from native_history import read_evidence
        calls = []
        def rpc(method, params):
            calls.append((method, params))
            self.assertEqual(method, 'thread/turns/list')
            self.assertEqual(params['itemsView'], 'full')
            if params['cursor'] is None:
                return {'data': [{'id':'t1','itemsView':'full','items':[
                    {'id':'u1','type':'userMessage','content':[{'type':'text','text':'hello'},{'type':'image','url':'private'}]},
                    {'id':'a','type':'reasoning','text':'not user'}]}], 'nextCursor':'next'}
            return {'data':[{'id':'t2','itemsView':'full','items':[
                {'id':'u2','type':'userMessage','content':[{'type':'text','text':'hello'}]}]}], 'nextCursor':None}
        got = read_evidence(rpc, 'session')
        self.assertTrue(got['complete'])
        self.assertEqual(len(calls), 2)
        self.assertEqual(got['records'], [{'turn_id':'t1','item_id':'u1','texts':['hello']}, {'turn_id':'t2','item_id':'u2','texts':['hello']}])

    def test_summary_or_repeating_cursor_is_not_full_evidence(self):
        from native_history import read_evidence
        for page in [
            {'data':[{'id':'t','itemsView':'summary','items':[]}], 'nextCursor':None},
            {'data':[], 'nextCursor':'repeat'},
        ]:
            with self.assertRaises(ValueError): read_evidence(lambda *args: page, 's')


if __name__ == '__main__': unittest.main()
