package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const backupPrefix = "aowugong-"

// BackupSQLite 使用 SQLite VACUUM INTO 创建一致快照并执行保留策略。
// 输入：ctx 控制操作，db 是源库，directory 是目录，retention 是份数，now 决定文件名。
// 输出：返回已通过完整性检查的快照绝对路径；失败时返回错误。
// 副作用：创建 SQLite 快照文件，并删除超出保留数量的旧应用快照。
func BackupSQLite(ctx context.Context, db *sql.DB, directory string, retention int, now time.Time) (string, error) {
	// 1. 校验参数并创建备份目录。
	if db == nil {
		return "", fmt.Errorf("SQLite 连接不能为空")
	}
	if retention < 1 {
		return "", fmt.Errorf("备份保留数量必须大于零")
	}
	directory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("解析备份目录: %w", err)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", fmt.Errorf("创建备份目录: %w", err)
	}
	path := filepath.Join(directory, backupPrefix+now.Format("20060102-150405")+".db")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("备份文件已存在: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查备份文件: %w", err)
	}

	// 2. 让 SQLite 从 WAL 视图创建一致的新数据库快照。
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", fmt.Errorf("创建 SQLite 安全快照: %w", err)
	}

	// 3. 在执行清理前验证新快照完整性，失败时保留文件供排查。
	if err := verifySQLiteBackup(ctx, path); err != nil {
		return "", err
	}
	if err := pruneSQLiteBackups(directory, retention); err != nil {
		return "", err
	}
	return path, nil
}

// verifySQLiteBackup 对独立快照执行 SQLite 完整性检查。
// 输入：ctx 控制查询，path 是新快照路径。
// 输出：完整时返回 nil，否则返回检查错误。
// 副作用：只读打开快照文件。
func verifySQLiteBackup(ctx context.Context, path string) error {
	// 1. 使用独立连接打开快照并执行完整性检查。
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("打开 SQLite 快照: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("检查 SQLite 快照完整性: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("SQLite 快照完整性检查失败: %s", result)
	}
	return nil
}

// pruneSQLiteBackups 仅删除应用命名规则内超出保留数量的旧快照。
// 输入：directory 是固定备份目录，retention 是保留份数。
// 输出：清理成功返回 nil，读取或删除失败返回错误。
// 副作用：删除目录内最旧的 aowugong-*.db 文件。
func pruneSQLiteBackups(directory string, retention int) error {
	// 1. 只收集当前目录中符合应用命名规则的普通文件。
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("读取 SQLite 备份目录: %w", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), backupPrefix) && strings.HasSuffix(entry.Name(), ".db") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	// 2. 文件名含时间且可排序，从最旧项开始删除超额快照。
	removeCount := len(names) - retention
	for index := 0; index < removeCount; index++ {
		path := filepath.Join(directory, names[index])
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("删除旧 SQLite 备份 %s: %w", names[index], err)
		}
	}
	return nil
}
