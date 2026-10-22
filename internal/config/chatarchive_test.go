package config

import (
	"strings"
	"testing"
)

func TestChatArchiveSyncKeyConfig(t *testing.T) {
	t.Setenv("AOWUGONG_CHAT_ARCHIVE_SYNC_KEY", strings.Repeat("global", 8))
	cfg, err := Load(newLookup(nil))
	if err != nil || cfg.ChatArchiveSyncKey != "" {
		t.Fatal("must use supplied lookup only")
	}
	for _, key := range []string{"short", strings.Repeat("x", 31), strings.Repeat("x", 32) + "\n"} {
		_, err := Load(newLookup(map[string]string{"AOWUGONG_CHAT_ARCHIVE_SYNC_KEY": key}))
		if err == nil || strings.Contains(err.Error(), key) {
			t.Fatal("invalid key must fail without leaking it")
		}
	}
	key := strings.Repeat("x", 32)
	cfg, err = Load(newLookup(map[string]string{"AOWUGONG_CHAT_ARCHIVE_SYNC_KEY": key}))
	if err != nil || cfg.ChatArchiveSyncKey != key {
		t.Fatal("valid key rejected")
	}
}
