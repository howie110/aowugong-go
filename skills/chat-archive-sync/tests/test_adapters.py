import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from adapters import parse_records


def lines(*records):
    return b"".join(json.dumps(r, ensure_ascii=False).encode() + b"\n" for r in records)


class AdapterTests(unittest.TestCase):
    def test_codex_uses_content_origin_and_final_phase(self):
        def message(role, texts, kinds, phase=None):
            return {'type': 'response_item', 'payload': {'type': 'message', 'role': role, 'phase': phase,
                'internal_chat_message_metadata_passthrough': {'content_item_kinds': kinds},
                'content': [{'type': 'input_text' if role == 'user' else 'output_text', 'text': t} for t in texts]}}
        data = lines(
            message('user', ['AGENTS private', 'environment'], ['agents_md.instructions', 'environments.environment_context']),
            message('user', ['actual question', 'skill instructions'], ['user.text', 'skills.selected_skill_instructions']),
            message('assistant', ['progress'], ['assistant.text'], 'commentary'),
            message('assistant', ['answer'], ['assistant.text'], 'final_answer'),
            message('user', ['subagent report'], ['multi_agent.subagent_notification']),
        )
        batch = parse_records('codex', data, 0, 'device', conversation_id='s')
        self.assertEqual([m['body'] for m in batch.messages], ['actual question', 'answer'])

    def test_codex_preserves_distinct_messages_and_ignores_duplicate_event(self):
        data = lines(
            {"type": "session_meta", "payload": {"id": "session-a"}},
            {"type": "response_item", "payload": {"id": "m1", "type": "message", "role": "user", "internal_chat_message_metadata_passthrough": {"content_item_kinds": ["user.text"]}, "content": [{"type": "input_text", "text": "same"}]}},
            {"type": "response_item", "payload": {"id": "m2", "type": "message", "role": "user", "internal_chat_message_metadata_passthrough": {"content_item_kinds": ["user.text"]}, "content": [{"type": "input_text", "text": "same"}]}},
            {"type": "event_msg", "payload": {"type": "user_message", "message": "same"}},
        )
        batch = parse_records("codex", data, 0, "device/file")
        self.assertEqual(batch.conversation_id, "session-a")
        self.assertEqual([m["message_key"] for m in batch.messages], ["id:m1", "id:m2"])
        self.assertEqual([m["body"] for m in batch.messages], ["same", "same"])
        self.assertEqual(batch.consumed, len(data))

    def test_claude_parent_blocks_and_time(self):
        batch = parse_records("claude-code", lines({"type": "assistant", "uuid": "child", "parentUuid": "parent", "sessionId": "s", "timestamp": "2026-01-01T00:00:00Z", "message": {"role": "assistant", "content": [{"type": "text", "text": "hi"}, {"type": "image", "source": {"url": "https://example.invalid/x"}}]}}), 0, "local")
        message = batch.messages[0]
        self.assertEqual(message["parent_key"], "id:parent")
        self.assertEqual(message["source_time"], "2026-01-01T00:00:00Z")
        self.assertEqual(message['attachments'], [])
        self.assertEqual(message['blocks'], [])
        self.assertEqual(message["body"], "hi")

    def test_partial_line_waits_and_bad_complete_line_fails_without_body(self):
        header = lines({"type": "session_meta", "payload": {"id": "s"}})
        self.assertEqual(parse_records("codex", header + b'{"secret":', 0, "a").consumed, len(header))
        with self.assertRaisesRegex(ValueError, "offset") as caught:
            parse_records("codex", header + b'{"private text"\n', 0, "a")
        self.assertNotIn("private text", str(caught.exception))

    def test_weak_ids_scoped_and_summary_excluded(self):
        data = lines({"type": "response_item", "payload": {"type": "message", "role": "user", "internal_chat_message_metadata_passthrough": {"content_item_kinds": ["user.text"]}, "content": [{"type": "input_text", "text": "hello"}]}})
        a = parse_records("codex", data, 100, "device-a/file", conversation_id="s")
        b = parse_records("codex", data, 100, "device-b/file", conversation_id="s")
        self.assertNotEqual(a.messages[0]["message_key"], b.messages[0]["message_key"])
        self.assertEqual(a.messages[0]["completeness"], "source_text")
        self.assertEqual(a.messages[0]["position"], 100)
        summary = lines({"type": "compacted", "payload": {"message": "summary"}})
        self.assertEqual(parse_records('codex', summary, 0, 'a', conversation_id='s').messages, [])

    def test_unknown_counted_not_invented_message(self):
        data = lines({"type": "session_meta", "payload": {"id": "s"}}, {"type": "future_kind", "payload": {"text": "not a message"}})
        batch = parse_records("codex", data, 0, "a")
        self.assertEqual(batch.messages, [])
        self.assertEqual(batch.unknown, 1)
        self.assertEqual(batch.consumed, len(data))

    def test_unsupported_and_mixed_sessions_rejected(self):
        with self.assertRaises(ValueError):
            parse_records("unknown", b"", 0, "a")
        with self.assertRaises(ValueError):
            parse_records("codex", lines({"type": "session_meta", "payload": {"id": "a"}}, {"type": "session_meta", "payload": {"id": "b"}}), 0, "a")


if __name__ == "__main__":
    unittest.main()
