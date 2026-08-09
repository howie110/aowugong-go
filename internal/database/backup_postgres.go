package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const postgresBackupPrefix = "aowugong-"

// PostgresBackuper 使用 PostgreSQL 官方工具创建可恢复的一致性备份。
type PostgresBackuper struct {
	databaseURL string
}

// NewPostgresBackuper 创建 PostgreSQL 备份器。
// 输入：databaseURL 是应用数据库连接地址。
// 输出：返回可复用备份器。
// 副作用：无。
func NewPostgresBackuper(databaseURL string) PostgresBackuper {
	// 1. 只保存连接地址，执行任务时才启动官方工具。
	return PostgresBackuper{databaseURL: strings.TrimSpace(databaseURL)}
}

// Backup 创建 PostgreSQL custom-format 备份并执行保留策略。
// 输入：ctx 控制备份，directory 是目录，retention 是保留数，now 用于命名。
// 输出：返回最终备份路径。
// 副作用：调用 pg_dump 和 pg_restore，创建备份文件并删除超额旧备份。
func (b PostgresBackuper) Backup(ctx context.Context, directory string, retention int, now time.Time) (string, error) {
	// 1. 校验配置并建立私有备份目录与临时目标。
	if b.databaseURL == "" || strings.TrimSpace(directory) == "" || retention < 1 {
		return "", fmt.Errorf("PostgreSQL 备份配置无效")
	}
	absoluteDirectory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("解析 PostgreSQL 备份目录: %w", err)
	}
	if err := os.MkdirAll(absoluteDirectory, 0o700); err != nil {
		return "", fmt.Errorf("创建 PostgreSQL 备份目录: %w", err)
	}
	name := postgresBackupPrefix + now.Format("20060102-150405") + ".dump"
	path := filepath.Join(absoluteDirectory, name)
	temporaryPath := path + ".tmp"
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("PostgreSQL 备份已存在: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查 PostgreSQL 备份目标: %w", err)
	}
	defer os.Remove(temporaryPath)

	// 2. 使用环境变量传递凭据，避免密码出现在命令参数中。
	environment, err := postgresCommandEnvironment(b.databaseURL)
	if err != nil {
		return "", err
	}
	dump := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file", temporaryPath)
	dump.Env = environment
	if output, err := dump.CombinedOutput(); err != nil {
		return "", fmt.Errorf("执行 pg_dump: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// 3. 用 pg_restore 读取目录验证备份格式，再原子发布文件。
	verify := exec.CommandContext(ctx, "pg_restore", "--list", temporaryPath)
	if output, err := verify.CombinedOutput(); err != nil {
		return "", fmt.Errorf("校验 PostgreSQL 备份: %s: %w", strings.TrimSpace(string(output)), err)
	}
	if err := syncPostgresBackup(temporaryPath); err != nil {
		return "", fmt.Errorf("同步 PostgreSQL 备份文件: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return "", fmt.Errorf("限制 PostgreSQL 备份权限: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", fmt.Errorf("发布 PostgreSQL 备份: %w", err)
	}
	if err := prunePostgresBackups(absoluteDirectory, retention); err != nil {
		return "", err
	}
	return path, nil
}

// syncPostgresBackup 把完整备份内容同步到磁盘。
// 输入：path 是已关闭写入的临时备份路径。
// 输出：同步成功返回 nil。
// 副作用：打开文件并调用 fsync。
func syncPostgresBackup(path string) error {
	// 1. 以读写模式打开并同步文件元数据与内容。
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

// postgresCommandEnvironment 把 PostgreSQL URL 转为 libpq 环境变量。
// 输入：databaseURL 是 postgres 或 postgresql URL。
// 输出：返回包含连接参数的进程环境。
// 副作用：无。
func postgresCommandEnvironment(databaseURL string) ([]string, error) {
	// 1. 解析地址并要求主机、用户和数据库完整。
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("解析 PostgreSQL 备份地址: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return nil, fmt.Errorf("PostgreSQL 备份地址协议无效")
	}
	username := parsed.User.Username()
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if parsed.Hostname() == "" || username == "" || databaseName == "" {
		return nil, fmt.Errorf("PostgreSQL 备份地址缺少主机、用户或数据库")
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}

	// 2. 继承基础环境并覆盖 pg_dump 需要的连接字段。
	environment := append([]string{}, os.Environ()...)
	environment = append(environment,
		"PGHOST="+parsed.Hostname(), "PGPORT="+port,
		"PGUSER="+username, "PGDATABASE="+databaseName,
	)
	if password, ok := parsed.User.Password(); ok {
		environment = append(environment, "PGPASSWORD="+password)
	}
	if sslMode := parsed.Query().Get("sslmode"); sslMode != "" {
		environment = append(environment, "PGSSLMODE="+sslMode)
	}
	return environment, nil
}

// prunePostgresBackups 删除超出保留数量的最旧应用备份。
// 输入：directory 是备份目录，retention 是保留数。
// 输出：清理成功返回 nil。
// 副作用：删除匹配命名规则的旧 dump 文件。
func prunePostgresBackups(directory string, retention int) error {
	// 1. 只收集应用自身的正式 custom-format 备份。
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("读取 PostgreSQL 备份目录: %w", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), postgresBackupPrefix) && strings.HasSuffix(entry.Name(), ".dump") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for index := 0; index < len(names)-retention; index++ {
		if err := os.Remove(filepath.Join(directory, names[index])); err != nil {
			return fmt.Errorf("删除旧 PostgreSQL 备份 %s: %w", names[index], err)
		}
	}
	return nil
}
