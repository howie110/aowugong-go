package app

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

func TestServeRuntimeClosesListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address := reserveAddress(t)
	done := make(chan error, 1)
	go func() {
		done <- serveRuntime(ctx, config.Config{HTTP: config.HTTP{Address: address}}, &appRuntime{handler: http.NotFoundHandler()})
	}()
	waitForServer(t, address, done)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listener leaked: %v", err)
	}
	ln.Close()
}

func TestNotificationRequiresWebhookBeforeDatabase(t *testing.T) {
	_, err := buildRuntime(context.Background(), config.Config{NotificationTokens: map[string]string{"test": strings.Repeat("a", 32)}})
	if err == nil || !strings.Contains(err.Error(), "WECOM_BOT_WEBHOOK_URL") {
		t.Fatalf("error=%v", err)
	}
}
