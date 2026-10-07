package config

import (
	"strings"
	"testing"
)

func TestNotificationTokensConfiguration(t *testing.T) {
	const token = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, value := range []string{"oops", `[]`, `{"test":"short"}`, `{"bad source":"` + token + `"}`, `{"one":"` + token + `","two":"` + token + `"}`, `{"test":null}`} {
		_, err := Load(newLookup(map[string]string{"AOWUGONG_NOTIFICATION_TOKENS": value}))
		if err == nil {
			t.Errorf("invalid token config accepted")
		} else if strings.Contains(err.Error(), token) {
			t.Fatal("configuration error leaked token")
		}
	}
	for _, value := range []string{"", "{}"} {
		cfg, err := Load(newLookup(map[string]string{"AOWUGONG_NOTIFICATION_TOKENS": value}))
		if err != nil || len(cfg.NotificationTokens) != 0 {
			t.Fatalf("default disabled: %v", err)
		}
	}
	cfg, err := Load(newLookup(map[string]string{"AOWUGONG_NOTIFICATION_TOKENS": `{"receipt-split":"` + token + `"}`}))
	if err != nil || cfg.NotificationTokens["receipt-split"] != token {
		t.Fatalf("valid config not loaded: %v", err)
	}
}
