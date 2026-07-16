package database

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

const (
	backupPrefix = "aowugong-"
	backupSuffix = ".sql.gz"
)

type commandFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

// MySQLBackuper 使用 mysqldump 创建可恢复的压缩逻辑备份。
type MySQLBackuper struct {
	command commandFactory
}

// NewMySQLBackuper 创建使用系统 mysqldump 的备份器。
// 输入：无。
// 输出：返回可由定时任务重复使用的无状态备份器。
// 副作用：无，不立即启动外部进程。
func NewMySQLBackuper() MySQLBackuper {
	// 1. 注入标准命令构造函数，测试可替换为受控子进程。
	return MySQLBackuper{command: exec.CommandContext}
}

// Backup 创建单事务 MySQL 逻辑备份并执行保留策略。
// 输入：ctx 控制命令，cfg 提供数据库身份，directory/retention/now 控制产物。
// 输出：返回已验证 gzip 备份的绝对路径；命令、压缩、验证或清理失败时返回错误。
// 副作用：运行 mysqldump、创建压缩文件，并删除超出保留数量的旧应用备份。
func (b MySQLBackuper) Backup(ctx context.Context, cfg config.Database, directory string, retention int, now time.Time) (string, error) {
	// 1. 校验固定参数并准备权限受限的备份文件。
	if b.command == nil {
		return "", fmt.Errorf("MySQL 备份命令构造器不能为空")
	}
	if strings.TrimSpace(cfg.DumpCommand) == "" || strings.TrimSpace(cfg.Host) == "" || cfg.Port < 1 ||
		strings.TrimSpace(cfg.Name) == "" || strings.TrimSpace(cfg.User) == "" || cfg.Password == "" {
		return "", fmt.Errorf("MySQL 备份配置不完整")
	}
	if retention < 1 {
		return "", fmt.Errorf("备份保留数量必须大于零")
	}
	directory, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("解析 MySQL 备份目录: %w", err)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", fmt.Errorf("创建 MySQL 备份目录: %w", err)
	}
	path := filepath.Join(directory, backupPrefix+now.Format("20060102-150405")+backupSuffix)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("创建 MySQL 备份文件: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()

	// 2. 通过环境变量传密码，把一致性 SQL 流直接压缩到目标文件。
	arguments := []string{
		"--host=" + cfg.Host,
		"--port=" + strconv.Itoa(cfg.Port),
		"--user=" + cfg.User,
		"--single-transaction", "--quick", "--skip-lock-tables",
		"--routines", "--events", "--triggers", "--hex-blob",
		"--set-gtid-purged=OFF", "--no-tablespaces", "--default-character-set=utf8mb4",
		"--databases", cfg.Name,
	}
	command := b.command(ctx, cfg.DumpCommand, arguments...)
	baseEnvironment := command.Env
	if baseEnvironment == nil {
		baseEnvironment = os.Environ()
	}
	command.Env = replaceEnvironment(baseEnvironment, "MYSQL_PWD", cfg.Password)
	compressed := gzip.NewWriter(file)
	command.Stdout = compressed
	var commandError bytes.Buffer
	command.Stderr = &commandError
	runErr := command.Run()
	compressionErr := compressed.Close()
	syncErr := file.Sync()
	closeErr := file.Close()
	if runErr != nil {
		message := truncateErrorText(commandError.String(), 2000)
		return "", fmt.Errorf("执行 mysqldump: %w: %s", runErr, message)
	}
	if compressionErr != nil {
		return "", fmt.Errorf("完成 MySQL 备份压缩: %w", compressionErr)
	}
	if syncErr != nil {
		return "", fmt.Errorf("同步 MySQL 备份文件: %w", syncErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("关闭 MySQL 备份文件: %w", closeErr)
	}

	// 3. 验证产物头部并只清理符合应用命名规则的旧备份。
	if err := verifyMySQLBackup(path); err != nil {
		return "", err
	}
	if err := pruneMySQLBackups(directory, retention); err != nil {
		return "", err
	}
	success = true
	return path, nil
}

// verifyMySQLBackup 验证 gzip 可读取且包含 mysqldump 标准头。
// 输入：path 是新建压缩备份路径。
// 输出：格式可识别时返回 nil，否则返回错误。
// 副作用：只读备份文件的前 64 KiB。
func verifyMySQLBackup(path string) error {
	// 1. 打开压缩流并读取足以覆盖标准头部的有限内容。
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开 MySQL 备份: %w", err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("打开 MySQL 备份压缩流: %w", err)
	}
	defer reader.Close()
	header, err := io.ReadAll(io.LimitReader(reader, 64*1024))
	if err != nil {
		return fmt.Errorf("读取 MySQL 备份头: %w", err)
	}
	if !bytes.Contains(header, []byte("-- MySQL dump")) {
		return fmt.Errorf("MySQL 备份缺少 mysqldump 标准头")
	}
	return nil
}

// pruneMySQLBackups 删除超出保留数量的最旧应用备份。
// 输入：directory 是固定备份目录，retention 是保留份数。
// 输出：清理成功返回 nil，读取或删除失败返回错误。
// 副作用：只删除当前目录中 aowugong-*.sql.gz 文件。
func pruneMySQLBackups(directory string, retention int) error {
	// 1. 收集命名规则内的普通文件并按时间文件名排序。
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("读取 MySQL 备份目录: %w", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), backupPrefix) && strings.HasSuffix(entry.Name(), backupSuffix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	// 2. 从最旧文件开始删除超额备份。
	for index := 0; index < len(names)-retention; index++ {
		if err := os.Remove(filepath.Join(directory, names[index])); err != nil {
			return fmt.Errorf("删除旧 MySQL 备份 %s: %w", names[index], err)
		}
	}
	return nil
}

// replaceEnvironment 替换命令环境中的单个敏感变量。
// 输入：environment 是原环境，key/value 是待替换变量。
// 输出：返回不含同名旧值的新环境数组。
// 副作用：无，不修改输入切片。
func replaceEnvironment(environment []string, key, value string) []string {
	// 1. 过滤同名变量后在末尾添加唯一新值。
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(strings.ToUpper(item), strings.ToUpper(prefix)) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

// truncateErrorText 清理并限制外部命令错误文本长度。
// 输入：value 是标准错误，limit 是最大 rune 数。
// 输出：返回不超过限制的单段文本。
// 副作用：无。
func truncateErrorText(value string, limit int) string {
	// 1. 清理空白并按 rune 截断，避免错误日志无限增长。
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
