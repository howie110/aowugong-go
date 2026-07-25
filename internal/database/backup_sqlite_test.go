package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

// TestSQLiteBackuperCreatesVerifiedSnapshotAndPrunesOldFiles 验证在线快照可读并执行保留策略。
// 输入：含一条业务记录的 SQLite 和三次不同时间备份。
// 输出：仅保留最近两份，最新快照包含原记录。
// 副作用：在测试临时目录创建和删除 SQLite 快照。
func TestSQLiteBackuperCreatesVerifiedSnapshotAndPrunesOldFiles(t *testing.T) {
	// 1. 创建源数据库并写入可核对记录。
	ctx := context.Background()
	source, err := OpenSQLite(ctx, config.Database{
		Path: filepath.Join(t.TempDir(), "source.db"), MaxOpenConns: 2,
		MaxIdleConns: 1, BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer source.Close()
	if _, err := source.Exec(`CREATE TABLE sample(id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO sample(id,name) VALUES(1,'snapshot')`); err != nil {
		t.Fatalf("prepare source: %v", err)
	}

	// 2. 连续创建三份快照并只保留最近两份。
	directory := t.TempDir()
	backuper := NewSQLiteBackuper()
	base := time.Date(2026, 7, 25, 3, 30, 0, 0, time.Local)
	for offset := 0; offset < 3; offset++ {
		if _, err := backuper.Backup(ctx, source, directory, 2, base.Add(time.Duration(offset)*time.Second)); err != nil {
			t.Fatalf("Backup(%d) error = %v", offset, err)
		}
	}
	files, err := filepath.Glob(filepath.Join(directory, "aowugong-*.db"))
	if err != nil {
		t.Fatalf("filepath.Glob() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("backup count = %d, want 2", len(files))
	}

	// 3. 最新快照应能只读打开并包含完整记录。
	backup, err := OpenSQLite(ctx, config.Database{
		Path: files[len(files)-1], MaxOpenConns: 1,
		MaxIdleConns: 1, BusyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backup.Close()
	var name string
	if err := backup.QueryRow("SELECT name FROM sample WHERE id = 1").Scan(&name); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if name != "snapshot" {
		t.Errorf("backup name = %q, want snapshot", name)
	}
}
