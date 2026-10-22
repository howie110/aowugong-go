package analytics

import (
	"context"
	"log/slog"
	"time"
)

// pruneExpired 采集后最多每分钟清理一批过期访问，级联删除对应事件。
// 保留最近 365 天；每批最多 100 次已结束访问，不触及其他业务表。
func (s *Service) pruneExpired(ctx context.Context, now time.Time) {
	last := s.lastPrune.Load()
	if now.Unix()-last < 60 || !s.lastPrune.CompareAndSwap(last, now.Unix()) {
		return
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM analytics_visits WHERE id IN (SELECT id FROM analytics_visits WHERE last_at < ? ORDER BY last_at LIMIT 100)`, now.AddDate(0, 0, -365))
	if err != nil {
		slog.Warn("analytics_retention_failed")
	}
}
