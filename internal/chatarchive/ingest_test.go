package chatarchive

import (
	"errors"
	"strings"
	"testing"
)

func TestIngestRejectionExplainsSafeReason(t *testing.T) {
	u := ingestFixture()
	u.State.ParserVersion = "private-untrusted-version"
	err := u.Validate()
	if !errors.Is(err, ErrInvalidIngest) || err.Error() != "invalid archive upload: state.parser_version" {
		t.Fatalf("missing safe reason: %v", err)
	}
}

func ingestFixture() UploadRequest {
	m := IngestMessage{MessageKey: "id:你好😀", IdentityQuality: "source_id", Role: "user", Body: "你好 😀 <>&\u2028\n\t", Blocks: []any{}, Attachments: []any{}, Completeness: "source_text", Position: 12}
	u := UploadRequest{DeviceID: "设备😀", AccountScope: "账户", Source: "codex", SourcePath: "/tmp/聊天.jsonl.zst", State: SyncState{Offset: 100, PrefixSHA256: strings.Repeat("a", 64), ConversationID: "session", ParserVersion: IngestParserVersion}, Batches: []IngestBatch{{StartOffset: 0, ConversationID: "session", Messages: []IngestMessage{m}}}}
	u.SourceKey = sourceKey(u)
	return u
}

func TestIngestPythonHashes(t *testing.T) {
	u := ingestFixture()
	if got := messageHash(u.Batches[0].Messages[0]); got != "8a643e33ad573b3d5c64ec6ff3e16d89cadee29abade4d7a1e776acd6a1197bc" {
		t.Fatalf("message hash %s", got)
	}
	if got := batchHash(u.Batches[0].Messages); got != "cc4856576ef3b00e413540b8eb63252dd68e704109ec645d4004108bc30afd47" {
		t.Fatalf("batch hash %s", got)
	}
	if got := u.SourceKey; got != "0ea1449104bbc66abd94cb644f04a3a4c7dd1a81eb3ed4bae105832a038f7820" {
		t.Fatalf("key %s", got)
	}
	u.SourcePath = strings.TrimSuffix(u.SourcePath, ".zst")
	if sourceKey(u) != u.SourceKey {
		t.Fatal("compressed key changed")
	}
}

func TestIngestValidation(t *testing.T) {
	if err := ingestFixture().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*UploadRequest){
		"key":              func(u *UploadRequest) { u.SourceKey = strings.Repeat("b", 64) },
		"role":             func(u *UploadRequest) { u.Batches[0].Messages[0].Role = "tool" },
		"blank":            func(u *UploadRequest) { u.Batches[0].Messages[0].Body = " \n" },
		"blocks":           func(u *UploadRequest) { u.Batches[0].Messages[0].Blocks = []any{"secret"} },
		"null attachments": func(u *UploadRequest) { u.Batches[0].Messages[0].Attachments = nil },
		"position":         func(u *UploadRequest) { u.Batches[0].Messages[0].Position = 100 },
		"session":          func(u *UploadRequest) { u.Batches[0].ConversationID = "other" },
		"version":          func(u *UploadRequest) { u.State.ParserVersion = "old" },
		"nul":              func(u *UploadRequest) { u.Batches[0].Messages[0].Body = "a\x00b" },
		"order":            func(u *UploadRequest) { u.Batches = append(u.Batches, u.Batches[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			u := ingestFixture()
			mutate(&u)
			if u.Validate() == nil {
				t.Fatal("accepted invalid upload")
			}
		})
	}
}

func TestIngestWindowsPathPythonKeys(t *testing.T) {
	for _, tc := range []struct{ path, key string }{
		{`C:\Users\你好😀\chat.jsonl.zst`, "d60bba7a69a655648e8b6dacd50879f060fbca7183dd76e85d49da20afd7c8d3"},
		{`\\server\share\聊天.jsonl`, "144fb7b8efc8147bb3b3772797a842b5a7d5cd2220c6b123dc450684b108d402"},
	} {
		u := ingestFixture()
		u.SourcePath = tc.path
		u.SourceKey = tc.key
		if sourceKey(u) != tc.key {
			t.Fatal("Python Windows key mismatch")
		}
		if err := u.Validate(); err != nil {
			t.Fatal("absolute Windows path rejected", err)
		}
	}
}

func TestIngestPythonDELSerialization(t *testing.T) {
	if got := pythonJSON("\x7f", true); got != `"\u007f"` {
		t.Fatalf("Python ASCII DEL mismatch: %q", got)
	}
	if got := pythonJSON("\x7f", false); got != "\"\x7f\"" {
		t.Fatalf("Python Unicode DEL mismatch: %q", got)
	}
}
