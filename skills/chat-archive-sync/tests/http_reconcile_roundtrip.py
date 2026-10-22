"""由 Go 隔离 HTTP/数据库测试执行，所有对话均为合成测试数据。"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

SCRIPTS = Path(__file__).resolve().parents[1] / 'scripts'
sys.path.insert(0, str(SCRIPTS))
from sync import SyncConfig, sync_source
from http_store import HTTPArchiveStore


def event(mid, text, marked=False, assistant=False):
    payload = {'type':'message','id':mid,'role':'assistant' if assistant else 'user',
               'content':[{'type':'output_text' if assistant else 'input_text','text':text}]}
    if marked: payload['internal_chat_message_metadata_passthrough']={'content_item_kinds':['user.text']}
    if assistant: payload['phase']='final_answer'
    return {'type':'response_item','timestamp':'2026-07-31T00:00:00Z','payload':payload}


with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    path = root/'source.jsonl'
    records = [{'type':'session_meta','payload':{'id':'synthetic-session'}}]
    records += [event(f'legacy-{i}','重复测试 😀') for i in range(502)]
    records += [event('injected','# AGENTS.md instructions for synthetic'),
                event('a','最终回复',assistant=True),event('u','现代用户',marked=True)]
    path.write_text(''.join(json.dumps(e,ensure_ascii=False)+'\n' for e in records),encoding='utf-8')
    rpc = root/'rpc.py'
    rpc.write_text('''import json,sys
for line in sys.stdin:
 r=json.loads(line)
 if 'id' not in r: continue
 if r['method']=='initialize': result={}
 elif r['method']=='thread/turns/list':
  offset=int(r['params']['cursor'] or '0')
  end=min(offset+10,502)
  result={'data':[{'id':str(i),'itemsView':'full','items':[{'type':'userMessage','id':'native-'+str(i),'content':[{'type':'text','text':'重复测试 😀'}]}]} for i in range(offset,end)],'nextCursor':str(end) if end<502 else None}
 else: raise RuntimeError('unexpected method')
 print(json.dumps({'id':r['id'],'result':result}),flush=True)
''',encoding='utf-8')
    cfg=SyncConfig('device-test','account-test','codex',str(path))
    private=root/'config.json'
    private.write_text(json.dumps({'api_url':os.environ['ARCHIVE_URL'],'api_key':'t'*40,
        'device_id':cfg.device_id,'account_scope':cfg.account_scope,
        'sources':[{'source':'codex','path':str(path)}],
        'codex_command':[sys.executable,str(rpc)]}),encoding='utf-8')
    private.chmod(0o600)
    with HTTPArchiveStore(os.environ['ARCHIVE_URL'],'t'*40) as store:
        assert sync_source(cfg,store)['messages']==2
        old_state=store.get_state(cfg.key)
        def run(*flags):
            p=subprocess.run([sys.executable,str(SCRIPTS/'sync.py'),'--config',str(private),*flags],capture_output=True,text=True,encoding='utf-8',timeout=90)
            rows=[json.loads(line) for line in p.stdout.splitlines()]
            assert p.returncode==2, (p.returncode,rows,p.stderr)
            return rows[-1]
        preview=run('--dry-run')
        assert preview['missing_before']==502 and preview['inserted']==0, preview
        assert store.verify()['message_observations']==2
        first=run()
        assert first['inserted']==502 and first['verified']==504 and first['unclassified']==1,first
        assert first['confirmed_records_verified'],first
        assert store.get_state(cfg.key)==old_state
        second=run()
        assert second['inserted']==0 and second['verified']==504,second
        assert run('--verify')['confirmed_records_verified']
        assert store.verify()['message_observations']==504
        # 修改保留旧版本，追加增量也走同一入口，旧游标不决定完整性。
        records[-1]=event('u','修改后用户',marked=True)
        records.append(event('next','下一句话',marked=True))
        path.write_text(''.join(json.dumps(e,ensure_ascii=False)+'\n' for e in records),encoding='utf-8')
        changed=run()
        assert changed['inserted']==2 and changed['different_before']==1 and changed['verified']==505,changed
        assert run()['inserted']==0
        assert store.verify()['message_observations']==506
        print('PASS: preview, native 51-page matching, 502 historical repairs, repeat zero, preserved cursor, edited version, appended message, full hash verification')
