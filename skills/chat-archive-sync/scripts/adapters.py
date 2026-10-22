"""只解析已知 JSONL 事件；不读取文件、不请求附件、不执行归档内容。"""
from dataclasses import dataclass, field
import hashlib
import json
import math

PARSER_VERSION = "dialogue-only-3"


@dataclass
class ParsedBatch:
    consumed: int = 0
    conversation_id: str | None = None
    messages: list = field(default_factory=list)
    unknown: int = 0


def parse_records(source, data, start_offset, identity_scope, conversation_id=None):
    """解析完整行，偏移始终指向原始字节；末尾未完成行留待下次同步。"""
    if source not in ("codex", "claude-code"):
        raise ValueError("unsupported source")
    result = ParsedBatch(conversation_id=conversation_id)
    for line in data.splitlines(keepends=True):
        if not line.endswith(b"\n"):
            break
        position = start_offset + result.consumed
        try:
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError()
            payload = event.get("payload", {})
            if not isinstance(payload, dict):
                raise ValueError()
            session = (payload.get("id") if event.get("type") == "session_meta" else None) if source == "codex" else event.get("sessionId")
            if session:
                if not isinstance(session, str) or (result.conversation_id and result.conversation_id != session):
                    raise ValueError()
                result.conversation_id = session
            message = _codex(event) if source == "codex" else _claude(event)
            if message:
                original_id, parent_id, role, content, completeness = message
                blocks = [{"type": "text", "text": content}] if isinstance(content, str) else content
                if not isinstance(blocks, list) or not all(isinstance(b, dict) for b in blocks):
                    raise ValueError()
                body = "\n".join(b["text"] for b in blocks if b.get("type") in ("text", "input_text", "output_text") and isinstance(b.get("text"), str))
                # 只允许用户/助手的正文；结构块可能含附件字节、工具结果和私密参数。
                if role not in ("user", "assistant") or completeness != "source_text" or not body.strip():
                    result.consumed += len(line)
                    continue
                blocks, attachments = [], []
                if _needs_escape(body):
                    # TEXT 不接受 NUL/孤立 surrogate，转义正文但不保存源文件。
                    body = json.dumps(body, ensure_ascii=True)[1:-1]
                    completeness = "escaped_" + completeness
                weak = hashlib.sha256(f"{identity_scope}:{position}".encode()).hexdigest()
                result.messages.append({"message_key": f"id:{original_id}" if original_id else f"local:{weak}", "identity_quality": "source_id" if original_id else "local_position", "parent_key": f"id:{parent_id}" if parent_id else None, "role": role, "body": body, "blocks": blocks, "attachments": attachments, "completeness": completeness, "source_time": event.get("timestamp"), "position": position})
            elif event.get("type") not in ("session_meta", "event_msg", "turn_context", "progress", "file-history-snapshot", "queue-operation"):
                result.unknown += 1
        except (ValueError, TypeError, KeyError, UnicodeError):
            # 不回显 JSON 内容，聊天里可能包含密钥。
            raise ValueError(f"invalid source event at offset {position}") from None
        result.consumed += len(line)
    return result


def _needs_escape(value):
    if isinstance(value, str):
        return any(c == "\x00" or 0xD800 <= ord(c) <= 0xDFFF for c in value)
    if isinstance(value, float):
        return not math.isfinite(value)
    if isinstance(value, list):
        return any(_needs_escape(v) for v in value)
    if isinstance(value, dict):
        return any(_needs_escape(k) or _needs_escape(v) for k, v in value.items())
    return False


def _codex(event):
    payload = event.get("payload", {})
    if event.get("type") == "response_item" and payload.get("type") == "message":
        role = payload.get('role')
        content = payload.get('content', [])
        if role == 'assistant':
            if payload.get('channel') in ('analysis', 'commentary') or payload.get('phase') not in (None, 'final_answer'):
                return None
            if payload.get('phase') != 'final_answer' and payload.get('channel') != 'final':
                return None
        elif role == 'user':
            metadata = payload.get('internal_chat_message_metadata_passthrough') or {}
            kinds = metadata.get('content_item_kinds') if isinstance(metadata, dict) else None
            # 逐块来源标记比 role 可靠；长度不匹配时不能猜哪些块是人类输入。
            if not isinstance(content, list) or not isinstance(kinds, list) or len(content) != len(kinds):
                return None
            content = [block for block, kind in zip(content, kinds) if kind == 'user.text']
        else:
            return None
        return payload.get("id"), payload.get("parent_id"), role, content, "source_text"
    if event.get("type") == "compacted":
        return None, None, "system", payload.get("message", ""), "source_summary"
    return None


def _claude(event):
    if event.get("type") in ("user", "assistant") and isinstance(event.get("message"), dict):
        message = event["message"]
        return event.get("uuid"), event.get("parentUuid"), message.get("role", event["type"]), message.get("content", []), "source_summary" if event.get("isCompactSummary") else "source_text"
    if event.get("type") == "summary":
        return event.get("uuid"), None, "system", event.get("summary", ""), "source_summary"
    return None
