package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type fakeSender struct {
	error error
	text  string
}

// SendText 记录测试文本并返回预设错误。
// 输入：上下文、正文和接收人。
// 输出：成功时返回 ok 响应，失败时返回预设错误。
// 副作用：修改测试发送器的 text 字段。
func (s *fakeSender) SendText(_ context.Context, content, _ string) (map[string]any, error) {
	// 1. 保存正文并返回预设结果。
	s.text = content
	return map[string]any{"ok": s.error == nil}, s.error
}

// SendFile 模拟发送本地文件。
// 输入：上下文、路径和接收人。
// 输出：成功时返回 ok 响应，失败时返回预设错误。
// 副作用：无。
func (s *fakeSender) SendFile(_ context.Context, _, _ string) (map[string]any, error) {
	// 1. 返回预设结果。
	return map[string]any{"ok": s.error == nil}, s.error
}

// SendImage 模拟发送本地图片。
// 输入：上下文、路径和接收人。
// 输出：成功时返回 ok 响应，失败时返回预设错误。
// 副作用：无。
func (s *fakeSender) SendImage(_ context.Context, _, _ string) (map[string]any, error) {
	// 1. 返回预设结果。
	return map[string]any{"ok": s.error == nil}, s.error
}

// TestServiceFormatsTextAndWritesNotificationLog 验证统一标题格式和发送日志。
// 输入：三级标题、正文和成功的模拟 OpeniLink 发送器。
// 输出：发送统一文本，并在 MySQL 写入 success 日志。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestServiceFormatsTextAndWritesNotificationLog(t *testing.T) {
	// 1. 创建数据库、模拟发送器和通知服务。
	ctx := context.Background()
	db := testdatabase.Open(t)
	sender := &fakeSender{}
	service := NewService(NewRepository(db), sender)

	// 2. 发送通知并核对统一文本。
	if err := service.Text(ctx, []string{"AOWUGONG", "JOB", "ERROR"}, "任务失败", ""); err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if sender.text != "【AOWUGONG / JOB / ERROR】\n\n任务失败" {
		t.Errorf("text = %q", sender.text)
	}

	// 3. 核对通知日志已记录成功状态且不保存密钥。
	var title, status string
	if err := db.QueryRowContext(ctx, "SELECT title, status FROM notification_log ORDER BY id DESC LIMIT 1").Scan(&title, &status); err != nil {
		t.Fatalf("query notification_log: %v", err)
	}
	if title != "AOWUGONG / JOB / ERROR" || status != "success" {
		t.Errorf("log = title:%q status:%q", title, status)
	}
}

// TestServiceLogsSendFailureAndReturnsError 验证外部发送失败会入库并向调用方返回。
// 输入：返回网络错误的模拟发送器。
// 输出：返回错误并在 MySQL 写入 failed 日志和错误信息。
// 副作用：创建并写入隔离 MySQL 测试 schema。
func TestServiceLogsSendFailureAndReturnsError(t *testing.T) {
	// 1. 创建数据库和预设失败发送器。
	ctx := context.Background()
	db := testdatabase.Open(t)
	service := NewService(NewRepository(db), &fakeSender{error: errors.New("hub unavailable")})

	// 2. 发送并确认错误传播及失败日志。
	if err := service.Text(ctx, []string{"TEST"}, "失败", ""); err == nil {
		t.Fatal("Text() error = nil, want send error")
	}
	var status, errorMessage string
	if err := db.QueryRowContext(ctx, "SELECT status, error_message FROM notification_log ORDER BY id DESC LIMIT 1").Scan(&status, &errorMessage); err != nil {
		t.Fatalf("query notification_log: %v", err)
	}
	if status != "failed" || errorMessage == "" {
		t.Errorf("log = status:%q error:%q", status, errorMessage)
	}
}
