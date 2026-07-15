package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// TestResolveSourceConfigUsesFlagThenEnvironment 验证迁移源配置优先级和旧 DATABASE_URL 兼容。
// 输入：显式 URL、环境 URL 和空配置三种情况。
// 输出：显式值优先，环境值可解析，空配置返回错误。
// 副作用：无。
func TestResolveSourceConfigUsesFlagThenEnvironment(t *testing.T) {
	// 1. 显式 URL 必须覆盖环境中的错误值。
	lookup := func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return "postgres://invalid", true
		}
		return "", false
	}
	config, err := resolveSourceConfig(options{mysqlURL: "mysql://user:pass@localhost/app"}, lookup)
	if err != nil || config.Schema != "app" {
		t.Errorf("explicit config = %+v error = %v", config, err)
	}

	// 2. 未传 flag 时兼容旧项目 DATABASE_URL。
	environmentLookup := func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return "mysql+pymysql://user:pass@localhost/legacy", true
		}
		return "", false
	}
	config, err = resolveSourceConfig(options{}, environmentLookup)
	if err != nil || config.Schema != "legacy" {
		t.Errorf("environment config = %+v error = %v", config, err)
	}

	// 3. 所有来源为空时返回明确错误。
	if _, err := resolveSourceConfig(options{}, func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("resolveSourceConfig() error = nil, want missing source error")
	}
}

// TestCommandReportUsesStableJSONFields 验证迁移审计报告使用稳定蛇形字段。
// 输入：最小命令报告。
// 输出：JSON 包含 mode/sqlite_path 且不暴露 Go 字段名。
// 副作用：无。
func TestCommandReportUsesStableJSONFields(t *testing.T) {
	// 1. 编码最小报告并检查公开字段名称。
	data, err := json.Marshal(commandReport{Mode: "migrate", SQLitePath: "aowugong.db"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	value := string(data)
	if !strings.Contains(value, `"mode":"migrate"`) || !strings.Contains(value, `"sqlite_path":"aowugong.db"`) {
		t.Fatalf("report json = %s", value)
	}
	if strings.Contains(value, `"Mode"`) || strings.Contains(value, `"SQLitePath"`) {
		t.Errorf("report leaks Go field names: %s", value)
	}
}

// TestCheckpointAndPublishSQLiteAtomicallyReplacesTarget 验证完整临时库可在 checkpoint 后发布到最终路径。
// 输入：一个已有旧文件的最终路径和一个包含可查询表的临时 SQLite。
// 输出：发布后最终路径是完整新库，临时路径消失。
// 副作用：在测试临时目录创建、同步、替换并读取 SQLite 文件。
func TestCheckpointAndPublishSQLiteAtomicallyReplacesTarget(t *testing.T) {
	// 1. 创建旧目标文件和使用 WAL 的完整临时 SQLite。
	ctx := context.Background()
	directory := t.TempDir()
	finalPath := filepath.Join(directory, "aowugong.db")
	stagedPath := filepath.Join(directory, "aowugong.db.migrating")
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	db, err := database.OpenSQLite(ctx, config.Database{Path: stagedPath})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE marker(value TEXT NOT NULL); INSERT INTO marker(value) VALUES('new')"); err != nil {
		t.Fatalf("create marker: %v", err)
	}

	// 2. checkpoint、关闭并发布后，最终文件必须可独立读取。
	if err := checkpointAndCloseSQLite(ctx, db); err != nil {
		t.Fatalf("checkpointAndCloseSQLite() error = %v", err)
	}
	if err := publishStagedSQLite(stagedPath, finalPath); err != nil {
		t.Fatalf("publishStagedSQLite() error = %v", err)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("staged path still exists: %v", err)
	}
	published, err := database.OpenSQLite(ctx, config.Database{Path: finalPath})
	if err != nil {
		t.Fatalf("open published SQLite: %v", err)
	}
	defer published.Close()
	var value string
	if err := published.QueryRowContext(ctx, "SELECT value FROM marker").Scan(&value); err != nil || value != "new" {
		t.Fatalf("published marker = %q, error = %v", value, err)
	}
}

// TestPublishStagedSQLiteRejectsTargetSidecars 验证存在目标 WAL 时不会覆盖正式数据库。
// 输入：最终文件、临时文件和模拟仍存活的目标 WAL。
// 输出：发布返回错误且两个主文件内容保持不变。
// 副作用：在测试临时目录创建并读取普通文件。
func TestPublishStagedSQLiteRejectsTargetSidecars(t *testing.T) {
	// 1. 创建代表旧库、新库和未 checkpoint 提交的三个文件。
	directory := t.TempDir()
	finalPath := filepath.Join(directory, "aowugong.db")
	stagedPath := filepath.Join(directory, "aowugong.db.migrating")
	for path, content := range map[string]string{
		finalPath: "old", stagedPath: "new", finalPath + "-wal": "pending",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	// 2. 发布必须失败且不移动或删除任一主文件。
	if err := publishStagedSQLite(stagedPath, finalPath); err == nil {
		t.Fatal("publishStagedSQLite() error = nil, want sidecar rejection")
	}
	for path, want := range map[string]string{finalPath: "old", stagedPath: "new"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("ReadFile(%s) = %q, %v; want %q", path, data, err, want)
		}
	}
}

// TestSnapshotExistingSQLitePreservesUnselectedData 验证局部迁移临时库从现有完整库克隆。
// 输入：一个包含未选业务表数据的最终 SQLite 和空临时路径。
// 输出：快照库保留原表结构与记录，可供后续只刷新选中表。
// 副作用：在测试临时目录创建、快照并读取 SQLite 文件。
func TestSnapshotExistingSQLitePreservesUnselectedData(t *testing.T) {
	// 1. 创建包含一条未选数据的现有最终库。
	ctx := context.Background()
	directory := t.TempDir()
	finalPath := filepath.Join(directory, "aowugong.db")
	stagedPath := filepath.Join(directory, "aowugong.db.migrating")
	db, err := database.OpenSQLite(ctx, config.Database{Path: finalPath})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE untouched(value TEXT NOT NULL); INSERT INTO untouched VALUES('keep')"); err != nil {
		t.Fatalf("create untouched: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	// 2. 一致快照必须保留未选表记录。
	if err := snapshotExistingSQLite(ctx, finalPath, stagedPath); err != nil {
		t.Fatalf("snapshotExistingSQLite() error = %v", err)
	}
	staged, err := database.OpenSQLite(ctx, config.Database{Path: stagedPath})
	if err != nil {
		t.Fatalf("open staged: %v", err)
	}
	defer staged.Close()
	var value string
	if err := staged.QueryRowContext(ctx, "SELECT value FROM untouched").Scan(&value); err != nil || value != "keep" {
		t.Fatalf("staged untouched = %q, error = %v", value, err)
	}
}
