package articleanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/howiedata/aowugong-go/internal/client"
)

// ParsePending 解析已经抓取元数据但尚未成功获取正文的文章。
// 输入：ctx 控制请求，limit 限制本次解析数量。
// 输出：返回解析统计；失败文章保留待解析状态。
// 副作用：调用微信原文接口并写入 PostgreSQL。
func (s *Service) ParsePending(ctx context.Context, limit int) (ParseBatchResult, error) {
	// 1. 解析功能只对当前微信读书来源开放，避免误调用其他文章来源。
	if s.options.WeRead == nil {
		return ParseBatchResult{}, fmt.Errorf("微信读书解析器未配置")
	}
	items, err := s.repository.pendingParseArticles(ctx, limit)
	if err != nil {
		return ParseBatchResult{}, err
	}
	result := ParseBatchResult{Items: make([]map[string]any, 0, len(items))}
	credentials, credentialErr := s.options.WeRead.loadCredentials(ctx)
	if credentialErr != nil {
		return result, credentialErr
	}
	originalCredentials := credentials
	for _, item := range items {
		content, fetchErr := s.parseWeReadArticle(ctx, item, &credentials)
		if fetchErr != nil || strings.TrimSpace(content) == "" {
			result.ErrorCount++
			result.Items = append(result.Items, map[string]any{"id": item.ID, "title": item.Title, "status": "pending_parse", "error": errorText(fetchErr, "正文为空")})
			continue
		}
		if err := s.repository.UpdateArticleContent(ctx, item.ID, content, truncateRunes(content, 300), "parsed"); err != nil {
			return result, err
		}
		result.ParsedCount++
		result.Items = append(result.Items, map[string]any{"id": item.ID, "title": item.Title, "status": "parsed"})
	}
	if credentials != originalCredentials {
		if saveErr := s.options.WeRead.saveCredentials(ctx, credentials, false); saveErr != nil {
			return result, saveErr
		}
	}
	return result, nil
}

// parseWeReadArticle 优先读取微信读书详情正文，再访问微信公众号原文。
// 输入：ctx 控制请求，item 是待解析文章，credentials 是可自动刷新的凭据。
// 输出：返回正文或带上下文的失败原因。
// 副作用：调用微信读书和微信公众号接口，可能刷新凭据。
func (s *Service) parseWeReadArticle(ctx context.Context, item pendingParseArticle, credentials *client.WeReadArticleCredentials) (string, error) {
	// 1. 读取抓取时保存的 review_id，避免直接访问已经触发环境验证的微信原文。
	var raw struct {
		ReviewID string `json:"review_id"`
	}
	if strings.TrimSpace(item.RawEntryJSON) != "" {
		if err := json.Unmarshal([]byte(item.RawEntryJSON), &raw); err != nil {
			return "", fmt.Errorf("解析文章原始标识: %w", err)
		}
	}
	var detailErr error
	if raw.ReviewID != "" {
		detail, err := s.options.WeRead.client.FetchArticleDetail(ctx, credentials, raw.ReviewID)
		if err == nil && strings.TrimSpace(detail.Content) != "" {
			return detail.Content, nil
		}
		detailErr = err
		if detailErr == nil {
			detailErr = fmt.Errorf("正文为空")
		}
	}

	// 2. 详情接口没有正文时，再用 Readability 解析微信公众号原文。
	content, _, err := s.options.WeRead.client.FetchArticleContent(ctx, item.Link)
	if err != nil {
		return "", combineArticleParseErrors(detailErr, err)
	}
	return content, nil
}

// combineArticleParseErrors 合并详情接口和微信原文的失败原因。
func combineArticleParseErrors(detailErr, originalErr error) error {
	if detailErr == nil {
		return fmt.Errorf("微信原文: %w", originalErr)
	}
	if originalErr == nil {
		return fmt.Errorf("微信读书详情: %w", detailErr)
	}
	return errors.Join(
		fmt.Errorf("微信读书详情: %w", detailErr),
		fmt.Errorf("微信原文: %w", originalErr),
	)
}

func errorText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}
