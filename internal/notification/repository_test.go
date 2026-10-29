package notification

import (
	"context"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// nullable 把空文本转为 SQL NULL，保留 source/request_id 的可空语义。
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// TestSaveFormatsSentAt 验证新写入的发送时间统一为到秒的文本。
// 输入：一条普通通知记录。
// 输出：sent_at 能被固定格式解析且不含单调时钟后缀。
// 副作用：创建并写入隔离测试数据库。
func TestSaveFormatsSentAt(t *testing.T) {
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.Save(context.Background(), Record{
		Channel: "wecom_bot", Title: "test", Message: "body", Status: "success",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var sentAt string
	if err := db.QueryRow("SELECT sent_at FROM notification_log ORDER BY id DESC LIMIT 1").Scan(&sentAt); err != nil {
		t.Fatalf("query sent_at: %v", err)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", sentAt); err != nil {
		t.Fatalf("sent_at = %q, want format 2006-01-02 15:04:05", sentAt)
	}
}

// TestListTrimsLegacySentAtSuffix 验证查询兼容旧数据里的单调时钟后缀。
// 输入：一条带后缀的历史 sent_at。
// 输出：List 返回截取到秒的时间。
// 副作用：创建并写入隔离测试数据库。
func TestListTrimsLegacySentAtSuffix(t *testing.T) {
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if _, err := db.Exec(`INSERT INTO notification_log(
		channel,title,message,status,error_message,sent_at
	) VALUES(?,?,?,?,?,?)`, "wecom_bot", "legacy", "body", "success", nil,
		"2026-10-29 15:24:25.123456789 +0800 CST m=+1"); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	page, err := repository.List(context.Background(), LogFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].SentAt != "2026-10-29 15:24:25" {
		t.Fatalf("items = %#v", page.Items)
	}
}

// TestListNotificationHistoryFiltersAndPagination 验证通知历史的关键字、状态、分类和分页。
// 输入：包含任务、监控、备份、项目和旧格式的记录。
// 输出：各筛选返回正确总数和记录。
// 副作用：创建并写入隔离测试数据库。
func TestListNotificationHistoryFiltersAndPagination(t *testing.T) {
	// 1. 写入覆盖各分类和跨项目来源的记录。
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	sentAt := time.Now().Format("2006-01-02 15:04:05")
	rows := []struct {
		title     string
		message   string
		status    string
		source    string
		requestID string
	}{
		{"任务 / backup_postgres", "PostgreSQL 备份失败", "failed", "", ""},
		{"监控 / 服务异常", "Vaultwarden 不可用", "failed", "", ""},
		{"备份 / backup_postgres", "PostgreSQL 备份失败", "failed", "", ""},
		{"receipt-split / 小票识别失败", "账单识别失败", "success", "receipt-split", "req-1"},
		{"server-alerts / 自定义备份标题", "Vaultwarden 备份失败", "success", "server-alerts", "req-2"},
		{"AOWUGONG / JOB / ERROR / legacy", "旧格式失败", "failed", "", ""},
	}
	for _, row := range rows {
		if _, err := db.ExecContext(context.Background(), `INSERT INTO notification_log(
			channel,title,message,status,error_message,sent_at,source,request_id
		) VALUES(?,?,?,?,?,?,?,?)`, "wecom_bot", row.title, row.message, row.status, nil,
			sentAt, nullable(row.source), nullable(row.requestID)); err != nil {
			t.Fatalf("insert notification_log: %v", err)
		}
	}

	// 2. 逐项核对筛选和分页结果。
	tests := []struct {
		name      string
		filter    LogFilter
		wantTotal int
		wantFirst string
	}{
		{name: "全部", filter: LogFilter{Page: 1, PageSize: 20}, wantTotal: 6, wantFirst: "AOWUGONG / JOB / ERROR / legacy"},
		{name: "关键字", filter: LogFilter{Query: "Vaultwarden", Page: 1, PageSize: 20}, wantTotal: 2},
		{name: "状态", filter: LogFilter{Status: "failed", Page: 1, PageSize: 20}, wantTotal: 4},
		{name: "任务分类", filter: LogFilter{Category: "任务", Page: 1, PageSize: 20}, wantTotal: 1, wantFirst: "任务 / backup_postgres"},
		{name: "备份分类", filter: LogFilter{Category: "备份", Page: 1, PageSize: 20}, wantTotal: 2},
		{name: "监控分类", filter: LogFilter{Category: "监控", Page: 1, PageSize: 20}, wantTotal: 1, wantFirst: "监控 / 服务异常"},
		{name: "订阅分类为空", filter: LogFilter{Category: "订阅", Page: 1, PageSize: 20}, wantTotal: 0},
		{name: "项目分类", filter: LogFilter{Category: "项目", Page: 1, PageSize: 20}, wantTotal: 1, wantFirst: "receipt-split / 小票识别失败"},
		{name: "其他分类", filter: LogFilter{Category: "其他", Page: 1, PageSize: 20}, wantTotal: 1, wantFirst: "AOWUGONG / JOB / ERROR / legacy"},
		{name: "组合筛选", filter: LogFilter{Query: "PostgreSQL", Status: "failed", Page: 1, PageSize: 20}, wantTotal: 2},
		{name: "分页第二页", filter: LogFilter{Page: 2, PageSize: 2}, wantTotal: 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := repository.List(context.Background(), test.filter)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if page.Total != test.wantTotal {
				t.Fatalf("total = %d, want %d", page.Total, test.wantTotal)
			}
			if test.filter.Page == 2 && test.filter.PageSize == 2 {
				if len(page.Items) != 2 || page.Page != 2 {
					t.Fatalf("page = %#v", page)
				}
				return
			}
			if len(page.Items) != test.wantTotal {
				t.Fatalf("items = %d, want %d", len(page.Items), test.wantTotal)
			}
			if test.wantFirst != "" && page.Items[0].Title != test.wantFirst {
				t.Fatalf("first title = %q, want %q", page.Items[0].Title, test.wantFirst)
			}
		})
	}
}
