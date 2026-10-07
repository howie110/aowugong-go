package notification

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid             = errors.New("通知参数无效，完整消息不得超过 2048 UTF-8 字节")
	ErrConflict            = errors.New("通知编号已用于不同内容")
	ErrRateLimited         = errors.New("通知接口发送过于频繁")
	ErrStorage             = errors.New("通知记录暂时不可用，请使用同一编号重试")
	notificationIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// DispatchInput 是统一通知输入；HTTP 层必须用 Token 对应的项目填充 Source。
type DispatchInput struct {
	Source    string `json:"source"`
	RequestID string `json:"request_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}

type DispatchResult struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	Duplicate bool   `json:"duplicate"`
}

// Dispatch 先持久化占位再发送；崩溃、网络失败或写回失败都不自动重发。
// 限流只覆盖通知接口（每分钟最多十条），不会改变原有定时任务通知行为。
func (s *Service) Dispatch(ctx context.Context, in DispatchInput) (DispatchResult, error) {
	in.Title, in.Content = strings.TrimSpace(in.Title), strings.TrimSpace(in.Content)
	title := buildTitle([]string{in.Source, in.Title})
	message := fmt.Sprintf("【%s】\n\n%s", title, in.Content)
	if !notificationIdentifier.MatchString(in.Source) || !notificationIdentifier.MatchString(in.RequestID) ||
		in.Title == "" || in.Content == "" || strings.ContainsAny(in.Title, "\r\n") || strings.ContainsRune(message, '\x00') || !utf8.ValidString(message) || len(message) > 2048 {
		return DispatchResult{}, ErrInvalid
	}
	result, err := s.reserveDispatch(ctx, in, title, message)
	if err != nil || result.Duplicate {
		return result, err
	}

	// 不受调用方断线影响地完成发送与结果写回，同时严格限制后台执行时间。
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	sendErr := s.sender.SendText(sendCtx, message)
	status, detail := "success", ""
	if sendErr != nil {
		status, detail = "unknown", "上游未确认发送结果；为避免重复，不自动重试"
	}
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	if err := s.repository.finishDispatch(saveCtx, result.ID, status, detail); err != nil {
		// 占位仍是 unknown；不把“发送成功但记账失败”当作可以重新发送。
		return result, ErrStorage
	}
	result.Status = status
	return result, nil
}

func (s *Service) reserveDispatch(ctx context.Context, in DispatchInput, title, message string) (DispatchResult, error) {
	// 同时限制排队和数据库等待，避免数据库锁阻塞拖住全部通知请求。
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	select {
	case s.dispatchGate <- struct{}{}:
		defer func() { <-s.dispatchGate }()
	case <-ctx.Done():
		return DispatchResult{}, ErrStorage
	}
	if result, found, err := s.repository.lookupDispatch(ctx, in.Source, in.RequestID, message); err != nil || found {
		return result, err
	}
	now := time.Now()
	if now.Sub(s.windowStart) >= time.Minute {
		s.windowStart = now
		s.windowCount = 0
	}
	if s.windowCount >= 10 {
		return DispatchResult{}, ErrRateLimited
	}
	result, err := s.repository.claimDispatch(ctx, in.Source, in.RequestID, title, message)
	if err == nil && !result.Duplicate {
		s.windowCount++
	}
	return result, err
}
