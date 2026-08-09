package vaultwardenbackup

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/howiedata/aowugong-go/internal/client"
)

type recordingMailer struct {
	message client.EmailMessage
	content []byte
}

// Send 记录邮件并读取临时附件供测试解密。
// 输入：ctx 是调用上下文，message 是待发送邮件。
// 输出：读取成功返回 nil，否则返回文件错误。
// 副作用：读取临时加密附件并修改测试记录器。
func (m *recordingMailer) Send(_ context.Context, message client.EmailMessage) error {
	// 1. 保存邮件字段并在服务删除临时文件前读取附件。
	m.message = message
	content, err := os.ReadFile(message.Attachments[0].Path)
	if err != nil {
		return err
	}
	m.content = content
	return nil
}

// TestSendLatestEncryptsNewestBackup 验证服务只发送最新备份且附件可由本地私钥解密。
// 输入：两份不同时间的临时备份和 age 测试密钥。
// 输出：邮件附件解密为最新备份内容。
// 副作用：创建临时文件并模拟发送邮件。
func TestSendLatestEncryptsNewestBackup(t *testing.T) {
	// 1. 创建密钥和两份具有明确修改时间的备份。
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("age.GenerateX25519Identity() error = %v", err)
	}
	directory := t.TempDir()
	oldPath := filepath.Join(directory, "vaultwarden-20260801-034500.tar.gz")
	newPath := filepath.Join(directory, "vaultwarden-20260808-034500.tar.gz")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old backup: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new-backup"), 0o600); err != nil {
		t.Fatalf("write new backup: %v", err)
	}
	oldTime := time.Date(2026, 8, 1, 3, 45, 0, 0, time.UTC)
	newTime := time.Date(2026, 8, 8, 3, 45, 0, 0, time.UTC)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("os.Chtimes(old) error = %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("os.Chtimes(new) error = %v", err)
	}
	scriptsDirectory := t.TempDir()
	for _, scriptName := range recoveryScriptNames {
		if err := os.WriteFile(filepath.Join(scriptsDirectory, scriptName), []byte("#!/usr/bin/env bash\n"), 0o700); err != nil {
			t.Fatalf("write recovery script %s: %v", scriptName, err)
		}
	}

	// 2. 发送最新备份并使用私钥解密记录的邮件附件。
	mailer := &recordingMailer{}
	service := NewService(mailer, Options{
		Directory: directory, RecoveryScriptsDirectory: scriptsDirectory,
		AgeRecipient: identity.Recipient().String(), EmailTo: "825360699@qq.com", MaxAttachmentBytes: 8192,
		Now: func() time.Time { return time.Date(2026, 8, 9, 5, 0, 0, 0, time.Local) },
	})
	result, err := service.SendLatest(context.Background())
	if err != nil {
		t.Fatalf("SendLatest() error = %v", err)
	}
	if result.SourcePath != newPath || result.EmailTo != "825360699@qq.com" {
		t.Errorf("SendLatest() result = %+v", result)
	}
	packageReader, err := zip.NewReader(bytes.NewReader(mailer.content), int64(len(mailer.content)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	var encryptedContent []byte
	foundInstructions := false
	foundScripts := make(map[string]bool, len(recoveryScriptNames))
	for _, file := range packageReader.File {
		fileReader, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("zip file.Open() error = %v", openErr)
		}
		content, readErr := io.ReadAll(fileReader)
		_ = fileReader.Close()
		if readErr != nil {
			t.Fatalf("io.ReadAll(zip file) error = %v", readErr)
		}
		if file.Name == "使用说明.md" {
			foundInstructions = strings.Contains(string(content), "旧服务器已经完全损坏") &&
				strings.Contains(string(content), "全新 Linux 服务器") &&
				!strings.Contains(string(content), "8.138.123.59")
		}
		if strings.HasSuffix(file.Name, ".tar.gz.age") {
			encryptedContent = content
		}
		for _, scriptName := range recoveryScriptNames {
			if file.Name == "scripts/"+scriptName {
				foundScripts[scriptName] = len(content) > 0
			}
		}
	}
	if !foundInstructions || len(encryptedContent) == 0 || len(foundScripts) != len(recoveryScriptNames) {
		t.Fatalf("recovery package files = %+v", packageReader.File)
	}
	reader, err := age.Decrypt(bytes.NewReader(encryptedContent), identity)
	if err != nil {
		t.Fatalf("age.Decrypt() error = %v", err)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if string(plaintext) != "new-backup" {
		t.Errorf("decrypted content = %q", plaintext)
	}
}
