package notification

import (
	"context"
	"fmt"
	"strings"
)

// Sender 定义通知服务使用的纯文本发送能力。
type Sender interface {
	SendText(ctx context.Context, content string) error
}

// Service 统一格式化并发送业务通知，同时记录发送结果。
type Service struct {
	repository *Repository
	sender     Sender
}

// NewService 创建统一通知服务。
// 输入：repository 写入 PostgreSQL 日志，sender 调用企业微信群机器人。
// 输出：返回可并发复用的通知服务。
// 副作用：无，不访问数据库和外部接口。
func NewService(repository *Repository, sender Sender) *Service {
	// 1. 保存显式依赖，业务和任务不直接持有企业微信客户端。
	return &Service{repository: repository, sender: sender}
}

// Text 发送带统一标题的微信文本并记录结果。
// 输入：ctx 控制执行，titleParts 是标题层级，body 是正文。
// 输出：成功返回 nil，发送或日志写入失败时返回错误。
// 副作用：调用企业微信群机器人并写入 PostgreSQL notification_log。
func (s *Service) Text(ctx context.Context, titleParts []string, body string) error {
	// 1. 清理标题并组装与旧项目一致的微信文本。
	title := buildTitle(titleParts)
	message := fmt.Sprintf("【%s】\n\n%s", title, strings.TrimSpace(body))

	// 2. 发送文本并无论成败都写入统一日志。
	sendErr := s.sender.SendText(ctx, message)
	record := Record{Channel: "wecom_bot", Title: title, Message: message, Status: "success"}
	if sendErr != nil {
		record.Status = "failed"
		record.ErrorMessage = sendErr.Error()
	}
	if saveErr := s.repository.Save(ctx, record); saveErr != nil {
		if sendErr != nil {
			return fmt.Errorf("发送微信通知: %v; 记录通知结果: %w", sendErr, saveErr)
		}
		return fmt.Errorf("记录微信通知结果: %w", saveErr)
	}
	if sendErr != nil {
		return fmt.Errorf("发送微信通知: %w", sendErr)
	}
	return nil
}

// buildTitle 清理标题层级并提供稳定默认标题。
// 输入：parts 是可能包含空值的标题层级。
// 输出：返回使用斜杠连接的非空标题。
// 副作用：无。
func buildTitle(parts []string) string {
	// 1. 丢弃空白层级并保留调用方顺序。
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return "AOWUGONG / INFO"
	}
	return strings.Join(cleaned, " / ")
}
