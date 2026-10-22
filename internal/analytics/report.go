package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

var dimensions = map[string]string{"path": "e.path", "url": "e.url", "entry": "v.entry_path", "exit": "v.exit_path", "referrer": "e.referrer", "channel": "e.channel", "browser": "e.browser", "os": "e.os", "device": "e.device", "language": "e.language", "country": "e.country", "region": "e.region", "city": "e.city"}

type Filter struct {
	From, To time.Time
	Values   map[string]string
}
type Metrics struct {
	Visitors   int     `json:"visitors"`
	Visits     int     `json:"visits"`
	Views      int     `json:"views"`
	BounceRate float64 `json:"bounce_rate"`
	Duration   float64 `json:"duration"`
}
type Point struct {
	Time     time.Time `json:"time"`
	Visitors int       `json:"visitors"`
	Views    int       `json:"views"`
}
type Rank struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type Heat struct {
	Day   int `json:"day"`
	Hour  int `json:"hour"`
	Count int `json:"count"`
}
type Report struct {
	Current    Metrics           `json:"current"`
	Previous   Metrics           `json:"previous"`
	Trend      []Point           `json:"trend"`
	Ranks      map[string][]Rank `json:"ranks"`
	Heat       []Heat            `json:"heat"`
	GeoEnabled bool              `json:"geo_enabled"`
	From       time.Time         `json:"from"`
	To         time.Time         `json:"to"`
}

func (f Filter) Validate() error {
	if f.From.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return ErrInvalid
	}
	for k, v := range f.Values {
		if _, ok := dimensions[k]; !ok || len(v) > 1000 {
			return ErrInvalid
		}
	}
	return nil
}
func (f Filter) where() (string, []any) {
	conditions := []string{"e.created_at >= ?", "e.created_at < ?"}
	args := []any{f.From, f.To}
	keys := make([]string, 0, len(f.Values))
	for k := range f.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		conditions = append(conditions, dimensions[k]+" = ?")
		args = append(args, f.Values[k])
	}
	return strings.Join(conditions, " AND "), args
}

const eventJoin = " FROM analytics_events e JOIN analytics_visits v ON v.id=e.visit_id WHERE "

// Report 在同一只读快照内计算各模块，所有模块共用过滤条件。
func (s *Service) Report(ctx context.Context, f Filter) (Report, error) {
	out := Report{Trend: []Point{}, Ranks: map[string][]Rank{}, Heat: []Heat{}, GeoEnabled: s.geo != nil, From: f.From, To: f.To}
	if err := f.Validate(); err != nil {
		return out, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL statement_timeout = '8s'"); err != nil {
		return out, err
	}
	out.Current, err = metrics(ctx, tx, f)
	if err != nil {
		return out, err
	}
	previous := f
	previous.To = f.From
	previous.From = f.From.Add(-f.To.Sub(f.From))
	out.Previous, err = metrics(ctx, tx, previous)
	if err != nil {
		return out, err
	}
	where, args := f.where()
	unit := "hour"
	if f.To.Sub(f.From) > 48*time.Hour {
		unit = "day"
	}
	bucket := "date_trunc('" + unit + "', e.created_at AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai'"
	rows, err := tx.QueryContext(ctx, "SELECT "+bucket+",count(DISTINCT e.visitor_id),count(*)"+eventJoin+where+" GROUP BY 1 ORDER BY 1", args...)
	if err != nil {
		return out, err
	}
	points := map[int64]Point{}
	for rows.Next() {
		var p Point
		if err = rows.Scan(&p.Time, &p.Visitors, &p.Views); err != nil {
			rows.Close()
			return out, err
		}
		points[p.Time.Unix()] = p
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	start := f.From.In(Shanghai)
	step := time.Hour
	if unit == "day" {
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, Shanghai)
		step = 24 * time.Hour
	} else {
		start = start.Truncate(time.Hour)
	}
	for t := start; t.Before(f.To); t = t.Add(step) {
		p, ok := points[t.Unix()]
		if !ok {
			p.Time = t
		}
		out.Trend = append(out.Trend, p)
	}
	for key, column := range dimensions {
		count := "count(*)"
		if key == "entry" || key == "exit" {
			count = "count(DISTINCT e.visit_id)"
		}
		limit := "100"
		if key == "country" {
			limit = "300"
		}
		rows, err = tx.QueryContext(ctx, "SELECT "+column+","+count+eventJoin+where+" GROUP BY 1 ORDER BY 2 DESC,1 LIMIT "+limit, args...)
		if err != nil {
			return out, err
		}
		out.Ranks[key] = []Rank{}
		for rows.Next() {
			var rank Rank
			if err = rows.Scan(&rank.Name, &rank.Count); err != nil {
				rows.Close()
				return out, err
			}
			out.Ranks[key] = append(out.Ranks[key], rank)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT extract(isodow FROM e.created_at AT TIME ZONE 'Asia/Shanghai')::int, extract(hour FROM e.created_at AT TIME ZONE 'Asia/Shanghai')::int,count(*)`+eventJoin+where+" GROUP BY 1,2 ORDER BY 1,2", args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var h Heat
		if err = rows.Scan(&h.Day, &h.Hour, &h.Count); err != nil {
			rows.Close()
			return out, err
		}
		out.Heat = append(out.Heat, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func metrics(ctx context.Context, tx *sql.Tx, f Filter) (Metrics, error) {
	where, args := f.where()
	var m Metrics
	query := fmt.Sprintf(`WITH selected AS (SELECT e.visitor_id,e.visit_id,e.created_at %s %s), visits AS (SELECT visit_id,count(*) n,extract(epoch FROM max(created_at)-min(created_at)) seconds FROM selected GROUP BY visit_id) SELECT (SELECT count(DISTINCT visitor_id) FROM selected),(SELECT count(*) FROM visits),(SELECT count(*) FROM selected),COALESCE((SELECT 100.0*count(*) FILTER(WHERE n=1)/NULLIF(count(*),0) FROM visits),0),COALESCE((SELECT avg(seconds) FROM visits),0)`, eventJoin, where)
	err := tx.QueryRowContext(ctx, query, args...).Scan(&m.Visitors, &m.Visits, &m.Views, &m.BounceRate, &m.Duration)
	return m, err
}
