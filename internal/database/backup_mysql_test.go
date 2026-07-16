package database

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

type recordingCommandFactory struct {
	name string
	args []string
}

// Command 创建由当前测试进程模拟的 mysqldump 命令并记录参数。
// 输入：ctx 控制进程，name 和 args 是正式备份实现生成的命令。
// 输出：返回只输出固定 SQL 的测试子进程。
// 副作用：记录命令名和参数，随后启动测试子进程。
func (f *recordingCommandFactory) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	// 1. 保存参数并让当前测试二进制进入辅助进程分支。
	f.name = name
	f.args = append([]string(nil), args...)
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestMySQLDumpHelperProcess")
	command.Env = append(os.Environ(), "AOWUGONG_MYSQL_DUMP_HELPER=1")
	return command
}

// TestMySQLBackuperCreatesGzipAndAppliesRetention 验证逻辑备份压缩、密钥传递和保留策略。
// 输入：模拟 mysqldump、两个旧备份和保留两份策略。
// 输出：生成可解压 SQL，只保留最新两份，命令参数不含密码。
// 副作用：在测试临时目录创建、读取和删除压缩备份文件。
func TestMySQLBackuperCreatesGzipAndAppliesRetention(t *testing.T) {
	// 1. 准备模拟命令、数据库配置和旧备份。
	root := t.TempDir()
	backupDir := filepath.Join(root, "backup")
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	for _, name := range []string{"aowugong-20260101-000000.sql.gz", "aowugong-20260102-000000.sql.gz"} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("old"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%s) error = %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(backupDir, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(keep.txt) error = %v", err)
	}
	factory := &recordingCommandFactory{}
	backuper := MySQLBackuper{command: factory.Command}
	cfg := config.Database{
		Host: "127.0.0.1", Port: 3306, Name: "aowugong", User: "backup", Password: "test-password",
		DumpCommand: "fake-mysqldump",
	}

	// 2. 执行备份并读取 gzip 中的 SQL 文本。
	path, err := backuper.Backup(context.Background(), cfg, backupDir, 2, time.Date(2026, 1, 3, 3, 30, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open() error = %v", err)
	}
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	_ = reader.Close()
	_ = file.Close()
	if !strings.Contains(string(content), "-- MySQL dump") {
		t.Errorf("backup content = %q, want MySQL dump header", content)
	}

	// 3. 核对密码未进入参数且最旧应用备份被清理。
	if factory.name != "fake-mysqldump" {
		t.Errorf("command name = %q, want fake-mysqldump", factory.name)
	}
	for _, argument := range factory.args {
		if strings.Contains(argument, cfg.Password) {
			t.Fatalf("mysqldump argument leaked password")
		}
	}
	if _, err := os.Stat(filepath.Join(backupDir, "aowugong-20260101-000000.sql.gz")); !os.IsNotExist(err) {
		t.Errorf("oldest backup still exists, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "keep.txt")); err != nil {
		t.Errorf("unrelated file was removed: %v", err)
	}
}

// TestMySQLDumpHelperProcess 模拟 mysqldump 向标准输出写入 SQL。
// 输入：AOWUGONG_MYSQL_DUMP_HELPER 和 MYSQL_PWD 环境变量。
// 输出：辅助模式输出固定 SQL，普通测试运行直接返回。
// 副作用：辅助模式校验密码环境并写标准输出，失败时退出进程。
func TestMySQLDumpHelperProcess(t *testing.T) {
	// 1. 普通 go test 收集阶段不执行辅助进程逻辑。
	if os.Getenv("AOWUGONG_MYSQL_DUMP_HELPER") != "1" {
		return
	}

	// 2. 确认密码通过环境变量传入并输出可验证 SQL。
	if os.Getenv("MYSQL_PWD") != "test-password" {
		os.Exit(2)
	}
	_, _ = os.Stdout.WriteString("-- MySQL dump 8.4\nCREATE DATABASE `aowugong`;\n")
	os.Exit(0)
}
