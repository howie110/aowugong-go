package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const sqliteBackupPrefix = "aowugong-"

// SQLiteBackuper 使用 VACUUM INTO 创建可恢复的一致性快照。
type SQLiteBackuper struct{}

// NewSQLiteBackuper 创建 SQLite 在线备份器。
// 输入：无。
// 输出：返回无可变状态的备份器。
// 副作用：无。
func NewSQLiteBackuper() SQLiteBackuper {
	// 1. 返回按调用参数执行的轻量值对象。
	return SQLiteBackuper{}
}

// Backup 创建 SQLite 一致性快照并执行保留策略。
// 输入：ctx 控制备份，db 是线上 SQLite，directory 是目录，retention 是保留数，now 用于命名。
// 输出：返回最终快照绝对路径；创建、校验或清理失败时返回错误。
// 副作用：读取线上 SQLite，创建快照文件并删除超额旧快照。
func (SQLiteBackuper) Backup(
	ctx context.Context,
	db *sql.DB,
	directory string,
	retention int,
	now time.Time,
) (string, error) {
	// 1. 校验依赖并创建备份目录。
	if db == nil {
		return "", fmt.Errorf("SQLite 备份数据库不能为空")
	}
	if strings.TrimSpace(directory) == "" || retention < 1 {
		return "", fmt.Errorf("SQLite 备份目录和保留数量无效")
	}
	absoluteDirectory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("解析 SQLite 备份目录: %w", err)
	}
	if err := os.MkdirAll(absoluteDirectory, 0o700); err != nil {
		return "", fmt.Errorf("创建 SQLite 备份目录: %w", err)
	}

	// 2. 先写临时文件，验证完整后再原子改名为正式快照。
	name := sqliteBackupPrefix + now.Format("20060102-150405") + ".db"
	path := filepath.Join(absoluteDirectory, name)
	temporaryPath := path + ".tmp"
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("SQLite 备份已存在: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查 SQLite 备份目标: %w", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", temporaryPath); err != nil {
		return "", fmt.Errorf("创建 SQLite 一致性快照: %w", err)
	}
	cleanupTemporary := true
	defer func() {
		if cleanupTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	// 3. 使用只读连接执行完整性检查并同步文件。
	if err := verifySQLiteBackup(ctx, temporaryPath); err != nil {
		return "", err
	}
	if err := syncFile(temporaryPath); err != nil {
		return "", err
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return "", fmt.Errorf("限制 SQLite 备份权限: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", fmt.Errorf("发布 SQLite 备份: %w", err)
	}
	cleanupTemporary = false

	// 4. 只清理本应用命名的超额旧快照。
	if err := pruneSQLiteBackups(absoluteDirectory, retention); err != nil {
		return "", err
	}
	return path, nil
}

// verifySQLiteBackup 使用只读连接检查快照完整性。
// 输入：ctx 控制检查，path 是临时快照。
// 输出：quick_check 返回 ok 时返回 nil，否则返回错误。
// 副作用：只读快照文件。
func verifySQLiteBackup(ctx context.Context, path string) error {
	// 1. 构造只读 file URL 并打开独立连接。
	urlPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	dsn := url.URL{Scheme: "file", Path: urlPath}
	query := dsn.Query()
	query.Set("mode", "ro")
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return fmt.Errorf("打开 SQLite 备份校验连接: %w", err)
	}
	defer db.Close()

	// 2. 要求 SQLite 自检明确返回 ok。
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("校验 SQLite 备份: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("SQLite 备份完整性异常: %s", result)
	}
	return nil
}

// syncFile 把快照内容同步到磁盘。
// 输入：path 是已完成写入的临时快照。
// 输出：打开或同步失败时返回错误。
// 副作用：调用文件系统同步。
func syncFile(path string) error {
	// 1. 以可写方式打开并同步文件元数据与内容。
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("打开 SQLite 备份同步文件: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步 SQLite 备份文件: %w", err)
	}
	return nil
}

// pruneSQLiteBackups 删除超出保留数量的最旧应用快照。
// 输入：directory 是备份目录，retention 是最终保留数量。
// 输出：目录读取或删除失败时返回错误。
// 副作用：删除旧 SQLite 快照文件。
func pruneSQLiteBackups(directory string, retention int) error {
	// 1. 只收集符合固定前缀和扩展名的普通文件。
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("读取 SQLite 备份目录: %w", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), sqliteBackupPrefix) && strings.HasSuffix(entry.Name(), ".db") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	// 2. 时间戳文件名按字典序等同时间顺序。
	for index := 0; index < len(names)-retention; index++ {
		path := filepath.Join(directory, names[index])
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("删除旧 SQLite 备份 %s: %w", names[index], err)
		}
	}
	return nil
}
