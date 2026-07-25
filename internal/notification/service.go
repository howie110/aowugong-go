package notification

import (
	"context"
	"fmt"
	"strings"
)

// Sender 定义通知服务使用的 OpeniLink 文本、文件和图片能力。
type Sender interface {
	SendText(ctx context.Context, content, to string) (map[string]any, error)
	SendFile(ctx context.Context, path, to string) (map[string]any, error)
	SendImage(ctx context.Context, path, to string) (map[string]any, error)
}

// Service 统一格式化并发送业务通知，同时记录发送结果。
type Service struct {
	repository *Repository
	sender     Sender
}

// NewService 创建统一通知服务。
// 输入：repository 写入 SQLite 日志，sender 调用 OpeniLink Hub。
// 输出：返回可并发复用的通知服务。
// 副作用：无，不访问数据库和外部接口。
func NewService(repository *Repository, sender Sender) *Service {
	// 1. 保存显式依赖，业务和任务不直接持有 OpeniLink 客户端。
	return &Service{repository: repository, sender: sender}
}

// Text 发送带统一标题的微信文本并记录结果。
// 输入：ctx 控制执行，titleParts 是标题层级，body 是正文，to 是可选接收人。
// 输出：成功返回 nil，发送或日志写入失败时返回错误。
// 副作用：调用 OpeniLink Hub 并写入 SQLite notification_log。
func (s *Service) Text(ctx context.Context, titleParts []string, body, to string) error {
	// 1. 清理标题并组装与旧项目一致的微信文本。
	title := buildTitle(titleParts)
	message := fmt.Sprintf("【%s】\n\n%s", title, strings.TrimSpace(body))

	// 2. 发送文本并无论成败都写入统一日志。
	_, sendErr := s.sender.SendText(ctx, message, to)
	record := Record{Channel: "openilink", Title: title, Message: message, Status: "success"}
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

// File 发送本地文件并记录结果。
// 输入：ctx 控制执行，title 是日志标题，path 是文件路径，to 是可选接收人。
// 输出：成功返回 nil，发送或日志写入失败时返回错误。
// 副作用：读取文件、调用 OpeniLink Hub 并写入 SQLite。
func (s *Service) File(ctx context.Context, title, path, to string) error {
	// 1. 调用文件能力并使用统一媒体结果记录入口。
	_, err := s.sender.SendFile(ctx, path, to)
	return s.recordMedia(ctx, "file", title, path, err)
}

// Image 发送本地图片并记录结果。
// 输入：ctx 控制执行，title 是日志标题，path 是图片路径，to 是可选接收人。
// 输出：成功返回 nil，发送或日志写入失败时返回错误。
// 副作用：读取图片、调用 OpeniLink Hub 并写入 SQLite。
func (s *Service) Image(ctx context.Context, title, path, to string) error {
	// 1. 调用图片能力并使用统一媒体结果记录入口。
	_, err := s.sender.SendImage(ctx, path, to)
	return s.recordMedia(ctx, "image", title, path, err)
}

// recordMedia 统一写入文件或图片发送结果并传播错误。
// 输入：ctx 控制写入，channel、title、path 描述媒体，sendErr 是发送结果。
// 输出：成功返回 nil，发送或日志写入失败时返回错误。
// 副作用：写入 SQLite notification_log。
func (s *Service) recordMedia(ctx context.Context, channel, title, path string, sendErr error) error {
	// 1. 根据发送错误构造成功或失败记录。
	record := Record{Channel: "openilink_" + channel, Title: strings.TrimSpace(title), Message: path, Status: "success"}
	if sendErr != nil {
		record.Status = "failed"
		record.ErrorMessage = sendErr.Error()
	}

	// 2. 优先保证发送结果可追踪，再传播原始发送错误。
	if saveErr := s.repository.Save(ctx, record); saveErr != nil {
		if sendErr != nil {
			return fmt.Errorf("发送微信%s: %v; 记录通知结果: %w", channel, sendErr, saveErr)
		}
		return fmt.Errorf("记录微信%s结果: %w", channel, saveErr)
	}
	if sendErr != nil {
		return fmt.Errorf("发送微信%s: %w", channel, sendErr)
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
