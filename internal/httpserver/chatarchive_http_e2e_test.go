package httpserver

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/chatarchive"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// 在隔离 PostgreSQL 中走真实 Python -> HTTP -> Go -> PostgreSQL，核对旧数据续传和双向哈希兼容。
func TestChatArchivePythonHTTPRoundTrip(t *testing.T) {
	dsn, python := os.Getenv("CHAT_ARCHIVE_TEST_DSN"), os.Getenv("CHAT_ARCHIVE_TEST_PYTHON")
	if dsn == "" || python == "" {
		t.Skip("isolated DB and Python environment required")
	}
	db, err := database.OpenPostgres(context.Background(), config.Database{URL: dsn, MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow("SELECT current_database()").Scan(&name); err != nil || name != "chat_archive_test" {
		t.Fatal("refusing non-test database")
	}
	if _, err = db.Exec("CREATE SCHEMA chat_archive_http_test"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA chat_archive_http_test CASCADE")
	if _, err = db.Exec("SET search_path=chat_archive_http_test"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/postgres/00013_chat_archive.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	migration, err = os.ReadFile("../../migrations/postgres/00014_chat_archive_time.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewRouter(Dependencies{ChatArchiveSync: chatarchive.NewRepository(db), ChatArchiveSyncKey: strings.Repeat("t", 40)}))
	defer server.Close()
	scripts, err := filepath.Abs("../../skills/chat-archive-sync/scripts")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, "-c", `
import os, sys, json, tempfile, hashlib, zipfile, types
from pathlib import Path
sys.path.insert(0, os.environ['ARCHIVE_SCRIPTS'])
from sync import SyncConfig, sync_source
from store import ArchiveStore
from http_store import HTTPArchiveStore
from psycopg.conninfo import make_conninfo
archive_dsn = make_conninfo(os.environ['ARCHIVE_DSN'], options='-c search_path=chat_archive_http_test')

class MetadataHTTPStore(HTTPArchiveStore):
 def _request(self,path,payload=None):
  if path=='/upload':
   payload['state']['client_metadata']={'fixture':'must-not-be-persisted'}
  return super()._request(path,payload)

def event(mid, text):
 return json.dumps({'type':'response_item','timestamp':'2026-10-17T00:00:00Z','payload':{'id':mid,'type':'message','role':'user','internal_chat_message_metadata_passthrough':{'content_item_kinds':['user.text']},'content':[{'type':'input_text','text':text}]}},ensure_ascii=False)+'\n'
with tempfile.TemporaryDirectory() as tmp:
 p=Path(tmp)/'会话.jsonl'
 p.write_text('{"type":"session_meta","payload":{"id":"session-http"}}\n'+event('a','旧记录 😀 <>&\u2028'),encoding='utf-8')
 cfg=SyncConfig('test-device','test-account','codex',str(p))
 with ArchiveStore(archive_dsn) as old:
  assert sync_source(cfg,old)['messages']==1
 with p.open('a',encoding='utf-8') as f: f.write(event('b','HTTP 新消息 " \\'))
 with MetadataHTTPStore(os.environ['ARCHIVE_URL'],'t'*40) as api:
  assert api.legacy_counts()=={'legacy_batches':0,'legacy_messages':0}
  assert sync_source(cfg,api,True)['messages']==1
  assert sync_source(cfg,api)['messages']==1
  assert sync_source(cfg,api)['messages']==0
  assert set(api.get_state(cfg.key))=={'offset','prefix_sha256','conversation_id','parser_version'}
  assert api.verify()['message_observations']==2
  p2=Path(tmp)/'batch.jsonl'
  p2.write_text('{"type":"session_meta","payload":{"id":"session-batch"}}\n'+''.join(event(str(i),'batch中文'+str(i)) for i in range(501)),encoding='utf-8')
  assert sync_source(SyncConfig('test-device','test-account','codex',str(p2)),api)['messages']==501
  assert api.verify()['message_observations']==503
 with ArchiveStore(archive_dsn) as old:
  assert old.verify()['message_observations']==503
 if os.environ.get('ARCHIVE_DIAGNOSTIC_ZIP'):
  with zipfile.ZipFile(os.environ['ARCHIVE_DIAGNOSTIC_ZIP']) as z:
   source=z.read('code/work/proposed-backfill/backfill_native.py').decode('utf-8')
   company_sync=z.read('code/installed/scripts/sync.py').decode('utf-8')
  sync_module=types.ModuleType('sync');sys.modules['sync']=sync_module
  exec(compile(company_sync,'reviewed_company_sync.py','exec'),sync_module.__dict__)
  SyncConfig,sync_source=sync_module.SyncConfig,sync_module.sync_source
  module=types.ModuleType('reviewed_company_backfill')
  exec(compile(source,'reviewed_company_backfill.py','exec'),module.__dict__)
  p3=Path(tmp)/'historical.jsonl'
  data=b'{"type":"session_meta","payload":{"id":"session-history"}}\n'
  selections=[]
  for i in range(502):
   text='same fictional input'
   missing=json.loads(event('historical-'+str(i),text))
   del missing['payload']['internal_chat_message_metadata_passthrough']
   line=(json.dumps(missing,ensure_ascii=False)+'\n').encode()
   selections.append({'position':len(data),'end':len(data)+len(line),'blocks':[{'index':0,'sha256':hashlib.sha256(text.encode()).hexdigest(),'evidence':'native_item_id_and_text'}]})
   data+=line
  data+=event('known','already archived').encode()
  p3.write_bytes(data)
  c3=SyncConfig('test-device','test-account','codex',str(p3))
  def snapshot():
   b=p3.read_bytes();s=p3.stat()
   return {'size':s.st_size,'mtime_ns':s.st_mtime_ns,'prefix_offset':len(b),'prefix_sha256':hashlib.sha256(b).hexdigest()}
  plan={'id':'session-history','path':str(p3),'source_key':c3.key,'snapshot':snapshot(),'messages':selections}
  with HTTPArchiveStore(os.environ['ARCHIVE_URL'],'t'*40) as api:
   assert sync_source(c3,api)['messages']==1
   before=api.get_state(c3.key)
   assert module.backfill_source(c3,api,plan,True)['messages']==502
   assert api.get_state(c3.key)==before
   assert module.backfill_source(c3,api,plan)['messages']==502
   after=api.get_state(c3.key)
   assert after['offset']==before['offset'] and after['prefix_sha256']==before['prefix_sha256']
   assert len(after['native_user_backfill_v1'])==502
   assert module.backfill_source(c3,api,plan)['messages']==0
   assert api.verify()['message_observations']==1006
   with p3.open('ab') as f:f.write(event('new-after-backfill','next message').encode())
   assert sync_source(c3,api)['messages']==1
   assert len(api.get_state(c3.key)['native_user_backfill_v1'])==502
   plan['snapshot']=snapshot()
   assert module.backfill_source(c3,api,plan)['messages']==0
   assert api.verify()['message_observations']==1007
  print('Actual company backfill: 502 historical messages, multi-batch, preview, repeat, marker persistence and subsequent regular sync passed')
print('Python HTTP round-trip: existing data, incremental upload, repeat, multi-batch, dual verification passed')
`)
	command.Env = append(os.Environ(), "ARCHIVE_SCRIPTS="+scripts, "ARCHIVE_URL="+server.URL, "ARCHIVE_DSN="+dsn)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("round trip: %v\n%s", err, output)
	}
	t.Log(string(output))
}
