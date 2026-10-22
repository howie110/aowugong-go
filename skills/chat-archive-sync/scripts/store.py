"""归档表专用 PostgreSQL 访问；所有写入仅经过此入口。"""
from contextlib import contextmanager
import hashlib
import json

from adapters import PARSER_VERSION


def message_hash(message):
    # 位置不是内容版本的一部分：同一来源消息搬迁后仍可核对内容。
    content = {k: v for k, v in message.items() if k != "position"}
    return hashlib.sha256(json.dumps(content, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


def batch_hash(messages):
    """校验纯文字消息和顺序，不保存源文件字节。"""
    return hashlib.sha256(json.dumps([(m['position'], message_hash(m)) for m in messages]).encode()).hexdigest()


class ArchiveStore:
    def __init__(self, dsn):
        import psycopg
        self.db = psycopg.connect(dsn, autocommit=True, connect_timeout=10, client_encoding="UTF8")
        self.db.execute("SET statement_timeout = '30s'")
        self.db.execute("SET lock_timeout = '10s'")

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.db.close()

    @contextmanager
    def sync_transaction(self, key):
        with self.db.transaction():
            # 事务 advisory lock 也覆盖首次写入；dry-run 不需要创建状态行。
            lock_id = int.from_bytes(bytes.fromhex(key)[:8], "big", signed=True)
            self.db.execute("SELECT pg_advisory_xact_lock(%s)", (lock_id,))
            yield self

    def get_state(self, key):
        row = self.db.execute("SELECT state FROM chat_archive_sync_states WHERE source_key=%s", (key,)).fetchone()
        return row[0] if row else None

    def legacy_counts(self):
        """只返回不兼容数据数量，阻止新上传掩盖旧原始记录。"""
        batches = self.db.execute("SELECT count(*) FROM chat_archive_raw_batches WHERE parser_version<>%s OR octet_length(raw)>0", (PARSER_VERSION,)).fetchone()[0]
        messages = self.db.execute("""SELECT count(*) FROM chat_archive_message_versions
            WHERE role NOT IN ('user','assistant') OR blocks<>'[]'::jsonb OR attachments<>'[]'::jsonb
            OR completeness NOT IN ('source_text','escaped_source_text')""").fetchone()[0]
        return {'legacy_batches': batches, 'legacy_messages': messages}

    def save_batch(self, config, key, state, raw, parsed, offset):
        from psycopg.types.json import Jsonb
        if raw or any(m['role'] not in ('user', 'assistant') or m['blocks'] or m['attachments'] or not m['body'].strip() for m in parsed.messages):
            raise ValueError('only plain user/assistant text is allowed')
        title = next((m["body"][:120] for m in parsed.messages if m["role"] == "user" and m["body"]), parsed.conversation_id)
        conversation_id = self.db.execute("""INSERT INTO chat_archive_conversations(source,account_scope,source_id,title)
            VALUES (%s,%s,%s,%s) ON CONFLICT(source,account_scope,source_id)
            DO UPDATE SET updated_at=now(),title=CASE
            WHEN chat_archive_conversations.title=chat_archive_conversations.source_id THEN EXCLUDED.title
            ELSE chat_archive_conversations.title END RETURNING id""", (config.source, config.account_scope, parsed.conversation_id, title)).fetchone()[0]
        batch_id = self.db.execute("""INSERT INTO chat_archive_raw_batches
            (conversation_id,source_key,device_id,source_path,start_offset,raw,sha256,parser_version,unknown_events)
            VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s) RETURNING id""", (conversation_id, key, config.device_id, config.path, offset, b"", batch_hash(parsed.messages), PARSER_VERSION, parsed.unknown)).fetchone()[0]
        for m in parsed.messages:
            self.db.execute("""INSERT INTO chat_archive_message_versions
                (conversation_id,message_key,version_hash,batch_id,position,parent_key,identity_quality,role,body,blocks,attachments,completeness,source_time)
                VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)""", (conversation_id, m["message_key"], message_hash(m), batch_id, m["position"], m["parent_key"], m["identity_quality"], m["role"], m["body"], Jsonb(m["blocks"]), Jsonb(m["attachments"]), m["completeness"], m["source_time"]))
        self.db.execute("""INSERT INTO chat_archive_sync_states(source_key,state,last_success_at)
            VALUES (%s,%s,now()) ON CONFLICT(source_key) DO UPDATE SET state=EXCLUDED.state,
            last_success_at=now(),last_error=NULL""", (key, Jsonb(state)))

    def record_failure(self, key):
        # 固定错误码，不把原始异常/凭证写入云端。
        self.db.execute("""INSERT INTO chat_archive_sync_states(source_key,last_error,last_failure_at)
            VALUES (%s,'sync_failed',now()) ON CONFLICT(source_key)
            DO UPDATE SET last_error='sync_failed',last_failure_at=now()""", (key,))

    def verify(self):
        """检查纯文字消息哈希和批次顺序；不声称能重建或核验源文件。"""
        batches = messages = 0
        with self.db.transaction():
            self.db.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
            if self.db.execute("""SELECT EXISTS(SELECT 1 FROM chat_archive_message_versions m
                JOIN chat_archive_raw_batches b ON b.id=m.batch_id WHERE m.conversation_id<>b.conversation_id)""").fetchone()[0]:
                raise ValueError('archive conversation mismatch')
            with self.db.cursor(name="archive_verify") as cursor:
                cursor.execute("""SELECT b.id,b.raw,b.sha256,b.source_key,b.start_offset,c.source,c.source_id,b.parser_version
                    FROM chat_archive_raw_batches b JOIN chat_archive_conversations c ON c.id=b.conversation_id ORDER BY b.id""")
                for bid, raw, sha, key, offset, source, sid, version in cursor:
                    if version != PARSER_VERSION or raw:
                        raise ValueError("archive integrity mismatch")
                    rows = self.db.execute("""SELECT message_key,parent_key,identity_quality,role,body,blocks,attachments,completeness,source_time,position,version_hash
                        FROM chat_archive_message_versions WHERE batch_id=%s ORDER BY position""", (bid,)).fetchall()
                    names = ["message_key", "parent_key", "identity_quality", "role", "body", "blocks", "attachments", "completeness", "source_time", "position"]
                    actual_messages = []
                    for row in rows:
                        actual = dict(zip(names, row[:-1]))
                        if actual['role'] not in ('user', 'assistant') or actual['blocks'] or actual['attachments'] or row[-1] != message_hash(actual):
                            raise ValueError("archive message mapping mismatch")
                        actual_messages.append(actual)
                    if batch_hash(actual_messages) != sha:
                        raise ValueError('archive batch mismatch')
                    batches += 1
                    messages += len(rows)
        return {"verified": True, "batches": batches, "message_observations": messages}
