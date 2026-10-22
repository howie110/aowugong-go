package chatarchive

import (
	"context"
	"testing"
	"time"
)

func TestStatisticsLargeArchive(t *testing.T) {
	db := ingestTestDB(t)
	ctx := context.Background()
	r := NewRepository(db)
	if _, err := db.Exec(`INSERT INTO chat_archive_conversations(source,account_scope,source_id) VALUES('codex','fixture','fixture');
 INSERT INTO chat_archive_raw_batches(conversation_id,source_key,device_id,source_path,start_offset,raw,sha256,parser_version) VALUES(1,'fixture','fixture','/fixture',0,''::bytea,'fixture','dialogue-only-3');
 INSERT INTO chat_archive_message_versions(conversation_id,message_key,version_hash,batch_id,position,identity_quality,role,body,blocks,attachments,completeness,source_time)
 SELECT 1,'id:'||i,'fixture',1,i,'source_id',CASE WHEN i%2=0 THEN 'user' ELSE 'assistant' END,repeat(md5(i::text),128),'[]','[]','source_text',to_char(timestamp '2026-10-17 12:00:00' - (i%30)*interval '1 day','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM generate_series(1,24000) i`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s, err := r.Statistics(ctx, 0, "2026-10-18")
	elapsed := time.Since(start)
	if err != nil || s.Totals.User != 12000 || s.Totals.Assistant != 12000 || len(s.Days) != 7 || !s.HasMore {
		t.Fatal(s, err)
	}
	start = time.Now()
	page, err := r.ListAudit(ctx, AuditFilter{Limit: 20, Order: "desc", Snapshot: s.Snapshot})
	listTime := time.Since(start)
	if err != nil || len(page.Messages) != 20 || page.Total != 24000 {
		t.Fatal(page.Total, err)
	}
	earlier, err := r.Statistics(ctx, s.Snapshot, s.Before)
	if err != nil || earlier.Days[0].Date != "2026-10-10" || earlier.Totals != s.Totals {
		t.Fatal(earlier, err)
	}
	t.Logf("24000 messages, 4KiB bodies: statistics=%s, first page=%s; only 20 bodies returned", elapsed, listTime)
}
