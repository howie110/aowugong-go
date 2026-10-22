package chatarchive

import (
	"context"
	"time"

	"github.com/howiedata/aowugong-go/internal/timezone"
)

type RoleCounts struct {
	User      int64 `json:"user"`
	Assistant int64 `json:"assistant"`
}
type DailyCount struct {
	Date string `json:"date"`
	RoleCounts
}
type Statistics struct {
	Snapshot int64        `json:"snapshot"`
	Totals   RoleCounts   `json:"totals"`
	Unknown  RoleCounts   `json:"unknown"`
	Days     []DailyCount `json:"days"`
	Before   string       `json:"before"`
	HasMore  bool         `json:"has_more"`
}

// 统计只扫描角色、时间和身份，不读取或返回正文。每次最多七天。
func (r *Repository) Statistics(ctx context.Context, snapshot int64, before string) (Statistics, error) {
	out := Statistics{Days: []DailyCount{}}
	if snapshot < 0 {
		return out, ErrInvalidFilter
	}
	zone := timezone.Beijing
	end := time.Now().In(zone)
	end = time.Date(end.Year(), end.Month(), end.Day()+1, 0, 0, 0, 0, zone)
	if before != "" {
		var err error
		end, err = time.ParseInLocation("2006-01-02", before, zone)
		if err != nil {
			return out, ErrInvalidFilter
		}
	}
	start := end.AddDate(0, 0, -7)
	out.Before = start.Format("2006-01-02")
	if snapshot == 0 {
		if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(max(id),0) FROM chat_archive_message_versions`).Scan(&snapshot); err != nil {
			return out, err
		}
	}
	out.Snapshot = snapshot
	err := r.db.QueryRowContext(ctx, latestAudit+`SELECT
 count(*) FILTER(WHERE role='user'),count(*) FILTER(WHERE role='assistant'),
 count(*) FILTER(WHERE role='user' AND source_at IS NULL),count(*) FILTER(WHERE role='assistant' AND source_at IS NULL),
 COALESCE(bool_or(source_at<?::timestamptz),false) FROM latest`, snapshot, start).Scan(&out.Totals.User, &out.Totals.Assistant, &out.Unknown.User, &out.Unknown.Assistant, &out.HasMore)
	if err != nil {
		return out, err
	}
	rows, err := r.db.QueryContext(ctx, latestAudit+`SELECT to_char(source_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'),count(*) FILTER(WHERE role='user'),count(*) FILTER(WHERE role='assistant') FROM latest WHERE source_at>=?::timestamptz AND source_at<?::timestamptz GROUP BY 1`, snapshot, start, end)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	days := map[string]RoleCounts{}
	for rows.Next() {
		var date string
		var c RoleCounts
		if err := rows.Scan(&date, &c.User, &c.Assistant); err != nil {
			return out, err
		}
		days[date] = c
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i := 1; i <= 7; i++ {
		date := end.AddDate(0, 0, -i).Format("2006-01-02")
		out.Days = append(out.Days, DailyCount{date, days[date]})
	}
	return out, nil
}
