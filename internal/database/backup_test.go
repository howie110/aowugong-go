package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestBackupSQLiteCreatesVerifiedSnapshotAndAppliesRetention 验证在线快照和保留数量。
// 输入：含一行数据的 WAL 数据库、已有两个旧备份和保留两份策略。
// 输出：创建可查询的新快照，并只保留最新两份。
// 副作用：在测试临时目录创建、读取和删除 SQLite 备份文件。
func TestBackupSQLiteCreatesVerifiedSnapshotAndAppliesRetention(t *testing.T) {
	// 1. 创建源数据库和需要被保留策略清理的旧备份。
	ctx := context.Background()
	root := t.TempDir()
	db, err := OpenSQLite(ctx, config.Database{Path: filepath.Join(root, "source.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE sample(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO sample(value) VALUES('ok')"); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, name := range []string{"aowugong-20260101-000000.db", "aowugong-20260102-000000.db"} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("old"), 0o640); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	// 2. 创建新快照并核对其中的数据可独立读取。
	path, err := BackupSQLite(ctx, db, backupDir, 2, time.Date(2026, 1, 3, 3, 30, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("BackupSQLite() error = %v", err)
	}
	backup, err := OpenSQLite(ctx, config.Database{Path: path})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	var value string
	if err := backup.QueryRowContext(ctx, "SELECT value FROM sample WHERE id = 1").Scan(&value); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if value != "ok" {
		t.Errorf("backup value = %q, want ok", value)
	}
	if err := backup.Close(); err != nil {
		t.Fatalf("close backup: %v", err)
	}

	// 3. 核对目录只保留最新两份命名备份。
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("backup count = %d, want 2", len(entries))
	}
	if _, err := os.Stat(filepath.Join(backupDir, "aowugong-20260101-000000.db")); !os.IsNotExist(err) {
		t.Errorf("oldest backup still exists, stat error = %v", err)
	}
}
