package databaseview

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServiceSummarizesAndReadsRows 验证只读服务返回表概况、搜索结果和脱敏字段。
// 输入：隔离 SQLite 中的两个测试用户。
// 输出：用户表计数正确，搜索仅返回匹配用户且密码隐藏。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestServiceSummarizesAndReadsRows(t *testing.T) {
	// 1. 写入两条包含敏感密码的用户记录。
	ctx := context.Background()
	db := testdatabase.Open(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO aowugong_fastapi_users(username,email,password)
		VALUES('alice','alice@example.com','secret-a'),('bob','bob@example.com','secret-b')`); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	service := NewService(db)

	// 2. 数据库概况必须包含用户表和准确行数。
	summary, err := service.Summary(ctx)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Engine != "SQLite" || summary.JournalMode != "WAL" || summary.SizeBytes <= 0 {
		t.Errorf("summary = %#v", summary)
	}
	userRows := int64(-1)
	for _, table := range summary.Tables {
		if table.Name == "aowugong_fastapi_users" {
			userRows = table.RowCount
		}
	}
	if userRows != 2 {
		t.Errorf("user row count = %d, want 2", userRows)
	}

	// 3. 搜索结果只返回 Alice，并把密码替换为固定隐藏文本。
	page, err := service.Rows(ctx, "aowugong_fastapi_users", "alice", 1, 20)
	if err != nil {
		t.Fatalf("Rows() error = %v", err)
	}
	if page.Total != 1 || len(page.Rows) != 1 || page.Rows[0]["username"] != "alice" {
		t.Fatalf("page = %#v", page)
	}
	if page.Rows[0]["password"] != "已隐藏" {
		t.Errorf("password = %#v, want hidden", page.Rows[0]["password"])
	}
}

// TestServiceRejectsUnknownTableAndLiteralSearchWildcards 验证表名和搜索均不能注入 SQL。
// 输入：伪造表名和包含百分号的字面搜索。
// 输出：伪造表被拒绝，百分号不作为任意匹配通配符。
// 副作用：创建并写入隔离 SQLite 测试库。
func TestServiceRejectsUnknownTableAndLiteralSearchWildcards(t *testing.T) {
	// 1. 写入包含字面百分号的任务摘要。
	ctx := context.Background()
	db := testdatabase.Open(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO job_execution(job_id,status,source,message)
		VALUES('percent','success','manual','完成 100%')`); err != nil {
		t.Fatalf("insert job execution: %v", err)
	}
	service := NewService(db)

	// 2. 表名注入必须在执行 SQL 前被拒绝。
	if _, err := service.Rows(ctx, `job_execution"; DROP TABLE job_execution; --`, "", 1, 20); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Rows(injected table) error = %v, want ErrTableNotFound", err)
	}

	// 3. 搜索百分号只匹配实际包含百分号的记录。
	page, err := service.Rows(ctx, "job_execution", "%", 1, 20)
	if err != nil {
		t.Fatalf("Rows(literal percent) error = %v", err)
	}
	if page.Total != 1 {
		t.Errorf("Rows(literal percent).Total = %d, want 1", page.Total)
	}
}

// TestServiceExportsRedactedCSV 验证 CSV 与页面使用同一脱敏规则。
// 输入：一条包含密码的用户记录。
// 输出：CSV 含用户名和“已隐藏”，不包含原始密码。
// 副作用：创建并读取隔离 SQLite 测试库，在内存生成 CSV。
func TestServiceExportsRedactedCSV(t *testing.T) {
	// 1. 写入测试用户并导出用户表。
	ctx := context.Background()
	db := testdatabase.Open(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO aowugong_fastapi_users(username,email,password)
		VALUES('export-user','export@example.com','raw-password')`); err != nil {
		t.Fatalf("insert export user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE aowugong_fastapi_users SET full_name = '=HYPERLINK("https://example.com")'
		WHERE username = 'export-user'
	`); err != nil {
		t.Fatalf("set formula-like text: %v", err)
	}
	var content bytes.Buffer
	err := NewService(db).ExportCSV(ctx, "aowugong_fastapi_users", "", &content)
	if err != nil {
		t.Fatalf("ExportCSV() error = %v", err)
	}

	// 2. 导出结果必须可识别中文且不泄露原密码。
	text := content.String()
	if !strings.HasPrefix(text, "\xef\xbb\xbf") || !strings.Contains(text, "export-user") ||
		!strings.Contains(text, "已隐藏") || strings.Contains(text, "raw-password") ||
		!strings.Contains(text, `'=HYPERLINK`) {
		t.Errorf("CSV content = %q", text)
	}
}
