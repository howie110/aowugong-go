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

// 使用实际 CLI 与合成 Codex RPC，覆盖旧进度漏采到逐条补齐的完整路径。
func TestChatArchiveReconcileCLI(t *testing.T) {
	dsn, python := os.Getenv("CHAT_ARCHIVE_TEST_DSN"), os.Getenv("CHAT_ARCHIVE_TEST_PYTHON")
	if dsn == "" || python == "" {
		t.Skip("isolated PostgreSQL and Python required")
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
	if _, err = db.Exec("CREATE SCHEMA chat_archive_reconcile_cli_test"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA chat_archive_reconcile_cli_test CASCADE")
	if _, err = db.Exec("SET search_path=chat_archive_reconcile_cli_test"); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"00013_chat_archive.sql", "00014_chat_archive_time.sql"} {
		migration, e := os.ReadFile("../../migrations/postgres/" + filename)
		if e != nil {
			t.Fatal(e)
		}
		if _, err = db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(NewRouter(Dependencies{ChatArchiveSync: chatarchive.NewRepository(db), ChatArchiveSyncKey: strings.Repeat("t", 40)}))
	defer server.Close()
	script, _ := filepath.Abs("../../skills/chat-archive-sync/tests/http_reconcile_roundtrip.py")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, python, script)
	command.Env = append(os.Environ(), "ARCHIVE_URL="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("reconcile CLI failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}
