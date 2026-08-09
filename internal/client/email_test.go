package client

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteEmailMessageIncludesBodyAndAttachment 验证邮件正文和附件可被完整写入。
// 输入：临时附件和固定邮件字段。
// 输出：生成内容包含主题、正文、附件名和 Base64 数据。
// 副作用：创建测试临时文件。
func TestWriteEmailMessageIncludesBodyAndAttachment(t *testing.T) {
	// 1. 创建附件并写入可识别测试内容。
	path := filepath.Join(t.TempDir(), "backup.tar.gz.age")
	if err := os.WriteFile(path, []byte("encrypted-backup"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	// 2. 生成 MIME 邮件并断言关键字段和附件内容存在。
	var output bytes.Buffer
	err := writeEmailMessage(&output, "sender@qq.com", EmailMessage{
		To:      []string{"receiver@qq.com"},
		Subject: "Vaultwarden 备份",
		Body:    "备份成功",
		Attachments: []EmailAttachment{{
			Name: "backup.tar.gz.age", Path: path,
		}},
	})
	if err != nil {
		t.Fatalf("writeEmailMessage() error = %v", err)
	}
	for _, fragment := range []string{"receiver@qq.com", "backup.tar.gz.age", "备份成功", "ZW5jcnlwdGVkLWJhY2t1cA=="} {
		if !strings.Contains(output.String(), fragment) {
			t.Errorf("email missing %q", fragment)
		}
	}
}
