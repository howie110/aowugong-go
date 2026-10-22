package chatarchive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const IngestParserVersion = "dialogue-only-3"

var ErrInvalidIngest = errors.New("invalid archive upload")
var ErrIngestConflict = errors.New("archive sync conflict")
var ErrArchiveIntegrity = errors.New("archive integrity mismatch")

// IngestValidationError 只携带固定字段路径，禁止包含客户端提供的值。
type IngestValidationError struct{ Field string }

func (e *IngestValidationError) Error() string { return ErrInvalidIngest.Error() + ": " + e.Field }
func (e *IngestValidationError) Unwrap() error { return ErrInvalidIngest }
func invalidIngestField(field string) error    { return &IngestValidationError{Field: field} }

type SyncState struct {
	Offset          int64    `json:"offset"`
	PrefixSHA256    string   `json:"prefix_sha256"`
	ConversationID  string   `json:"conversation_id"`
	ParserVersion   string   `json:"parser_version"`
	BackfillMarkers []string `json:"native_user_backfill_v1,omitempty"`
}
type IngestMessage struct {
	MessageKey      string  `json:"message_key"`
	ParentKey       *string `json:"parent_key"`
	IdentityQuality string  `json:"identity_quality"`
	Role            string  `json:"role"`
	Body            string  `json:"body"`
	Blocks          []any   `json:"blocks"`
	Attachments     []any   `json:"attachments"`
	Completeness    string  `json:"completeness"`
	SourceTime      *string `json:"source_time"`
	Position        int64   `json:"position"`
}
type IngestBatch struct {
	StartOffset    int64           `json:"start_offset"`
	ConversationID string          `json:"conversation_id"`
	Messages       []IngestMessage `json:"messages"`
	UnknownEvents  int64           `json:"unknown_events"`
}
type UploadRequest struct {
	SourceKey     string        `json:"source_key"`
	DeviceID      string        `json:"device_id"`
	AccountScope  string        `json:"account_scope"`
	Source        string        `json:"source"`
	SourcePath    string        `json:"source_path"`
	ExpectedState *SyncState    `json:"expected_state"`
	State         SyncState     `json:"state"`
	Batches       []IngestBatch `json:"batches"`
}
type LegacyCounts struct {
	Batches  int64 `json:"legacy_batches"`
	Messages int64 `json:"legacy_messages"`
}
type VerifyResult struct {
	Verified bool  `json:"verified"`
	Batches  int64 `json:"batches"`
	Messages int64 `json:"message_observations"`
}

func ValidSourceKey(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func ingestText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= max && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func (s SyncState) valid() bool {
	if len(s.BackfillMarkers) > 100000 {
		return false
	}
	for i, marker := range s.BackfillMarkers {
		if !ValidSourceKey(marker) || (i > 0 && marker <= s.BackfillMarkers[i-1]) {
			return false
		}
	}
	return s.Offset >= 0 && ValidSourceKey(s.PrefixSHA256) && ingestText(s.ConversationID, 1000) && s.ParserVersion == IngestParserVersion
}
func (m IngestMessage) valid() bool {
	if !ingestText(m.MessageKey, 2000) || !ingestText(m.Body, 16<<20) || m.Blocks == nil || len(m.Blocks) != 0 || m.Attachments == nil || len(m.Attachments) != 0 {
		return false
	}
	if m.Role != "user" && m.Role != "assistant" {
		return false
	}
	if m.Completeness != "source_text" && m.Completeness != "escaped_source_text" {
		return false
	}
	if m.IdentityQuality != "source_id" && m.IdentityQuality != "local_position" {
		return false
	}
	if m.IdentityQuality == "source_id" && !strings.HasPrefix(m.MessageKey, "id:") || m.IdentityQuality == "local_position" && (!strings.HasPrefix(m.MessageKey, "local:") || !ValidSourceKey(strings.TrimPrefix(m.MessageKey, "local:"))) {
		return false
	}
	return (m.ParentKey == nil || ingestText(*m.ParentKey, 2000)) && (m.SourceTime == nil || ingestText(*m.SourceTime, 1000))
}
func (u UploadRequest) Validate() error {
	for _, check := range []struct {
		field string
		valid bool
	}{
		{"source_key", ValidSourceKey(u.SourceKey)},
		{"device_id", ingestText(u.DeviceID, 1000)},
		{"account_scope", ingestText(u.AccountScope, 1000)},
		{"source_path", ingestText(u.SourcePath, 8192) && absoluteIngestPath(u.SourcePath)},
		{"source", u.Source == "codex" || u.Source == "claude-code"},
		{"source_key.hash", sourceKey(u) == u.SourceKey},
		{"state.parser_version", u.State.ParserVersion == IngestParserVersion},
		{"state", u.State.valid()},
		{"batches", len(u.Batches) > 0 && len(u.Batches) <= 10000},
	} {
		if !check.valid {
			return invalidIngestField(check.field)
		}
	}
	if u.ExpectedState != nil && !u.ExpectedState.valid() {
		return invalidIngestField("expected_state")
	}
	first := u.Batches[0].StartOffset
	backfill, err := u.validateBackfill()
	if err != nil {
		return err
	}
	if !backfill && first != 0 && (u.ExpectedState == nil || first != u.ExpectedState.Offset) {
		return invalidIngestField("batches[0].start_offset")
	}
	if u.ExpectedState != nil && u.ExpectedState.ConversationID != u.State.ConversationID {
		return invalidIngestField("expected_state.conversation_id")
	}
	for i, b := range u.Batches {
		end := u.State.Offset
		if i+1 < len(u.Batches) {
			end = u.Batches[i+1].StartOffset
		}
		if b.StartOffset < 0 || b.StartOffset >= end || b.ConversationID != u.State.ConversationID || b.UnknownEvents < 0 || b.UnknownEvents > 2147483647 || b.Messages == nil {
			return invalidIngestField(fmt.Sprintf("batches[%d].bounds_or_metadata", i))
		}
		prev := b.StartOffset - 1
		for j, m := range b.Messages {
			if !m.valid() {
				return invalidIngestField(fmt.Sprintf("batches[%d].messages[%d].fields", i, j))
			}
			if m.Position <= prev || m.Position >= end {
				return invalidIngestField(fmt.Sprintf("batches[%d].messages[%d].position", i, j))
			}
			prev = m.Position
		}
	}
	return nil
}

// The client path is metadata; Windows paths must remain byte-for-byte unchanged on Linux.
func absoluteIngestPath(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}
	if len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return true
	}
	if strings.HasPrefix(p, `\\`) {
		parts := strings.Split(p[2:], `\`)
		return len(parts) >= 3 && parts[0] != "" && parts[1] != ""
	}
	return false
}

func digestString(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func sourceKey(u UploadRequest) string {
	p := u.SourcePath
	if strings.HasSuffix(p, ".jsonl.zst") {
		p = strings.TrimSuffix(p, ".zst")
	}
	return digestString(pythonJSON([]string{u.DeviceID, u.AccountScope, u.Source, p}, true))
}
func messageHash(m IngestMessage) string {
	raw, _ := json.Marshal(m)
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	delete(value, "position")
	return digestString(pythonJSON(value, false))
}
func batchHash(messages []IngestMessage) string {
	pairs := make([]any, 0, len(messages))
	for _, m := range messages {
		pairs = append(pairs, []any{m.Position, messageHash(m)})
	}
	return digestString(pythonJSON(pairs, true))
}

// Python json.dumps uses spaces, sorted message keys, and optional ASCII escapes.
// encoding/json's HTML and U+2028 escaping cannot be used for these persisted hashes.
func pythonJSON(value any, ascii bool) string {
	raw, _ := json.Marshal(value)
	var normalized any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&normalized)
	var out strings.Builder
	var write func(any)
	quote := func(s string) {
		out.WriteByte('"')
		for _, r := range s {
			switch r {
			case '"', '\\':
				out.WriteByte('\\')
				out.WriteRune(r)
			case '\b':
				out.WriteString(`\b`)
			case '\f':
				out.WriteString(`\f`)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			default:
				if r < 32 || ascii && r >= 127 {
					if r > 0xffff {
						a, b := utf16.EncodeRune(r)
						fmt.Fprintf(&out, `\u%04x\u%04x`, a, b)
					} else {
						fmt.Fprintf(&out, `\u%04x`, r)
					}
				} else {
					out.WriteRune(r)
				}
			}
		}
		out.WriteByte('"')
	}
	write = func(v any) {
		switch v := v.(type) {
		case nil:
			out.WriteString("null")
		case string:
			quote(v)
		case json.Number:
			out.WriteString(string(v))
		case bool:
			fmt.Fprint(&out, v)
		case []any:
			out.WriteByte('[')
			for i, x := range v {
				if i > 0 {
					out.WriteString(", ")
				}
				write(x)
			}
			out.WriteByte(']')
		case map[string]any:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					out.WriteString(", ")
				}
				quote(k)
				out.WriteString(": ")
				write(v[k])
			}
			out.WriteByte('}')
		}
	}
	write(normalized)
	return out.String()
}
