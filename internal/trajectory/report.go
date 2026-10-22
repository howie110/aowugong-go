package trajectory

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// Report 所有查询绑定 owner；分页使用首次查询时刻，防止补传插入导致翻页重复。
func (s *Service) Report(ctx context.Context, owner int64, q Query) (Report, error) {
	out := Report{Points: []Point{}, Page: q.Page, PageSize: PageSize}
	from, to, e := q.Bounds()
	if e != nil {
		return out, e
	}
	if q.DeviceID != "" {
		if _, e = uuid.Parse(q.DeviceID); e != nil {
			return out, ErrInvalid
		}
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(8672,?)", owner); e != nil {
		return out, e
	}
	now := time.Now().UTC()
	snapshot := q.Snapshot
	if snapshot.IsZero() {
		snapshot = now
	}
	if snapshot.After(now) || snapshot.Before(now.Add(-24*time.Hour)) {
		return out, ErrInvalid
	}
	out.Snapshot = snapshot
	scope := " FROM trajectory_points p JOIN trajectory_devices d ON d.id=p.device_id WHERE d.owner_id=? AND p.received_at<=?"
	args := []any{owner, snapshot}
	if q.DeviceID != "" {
		scope += " AND d.id=?"
		args = append(args, q.DeviceID)
	}
	rangeScope := scope + " AND p.recorded_at>=? AND p.recorded_at<?"
	rangeArgs := append(append([]any{}, args...), from, to)
	if e = tx.QueryRowContext(ctx, "SELECT count(*)"+rangeScope, rangeArgs...).Scan(&out.Total); e != nil {
		return out, e
	}
	day := now.In(Shanghai)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, Shanghai)
	todayArgs := append(append([]any{}, args...), start.UTC(), start.AddDate(0, 0, 1).UTC())
	if e = tx.QueryRowContext(ctx, "SELECT count(*)"+scope+" AND p.recorded_at>=? AND p.recorded_at<?", todayArgs...).Scan(&out.TodayCount); e != nil {
		return out, e
	}
	if e = tx.QueryRowContext(ctx, "SELECT max(p.recorded_at)"+scope, args...).Scan(&out.LastRecordedAt); e != nil {
		return out, e
	}
	deviceScope := "SELECT max(last_received_at) FROM trajectory_devices WHERE owner_id=?"
	deviceArgs := []any{owner}
	if q.DeviceID != "" {
		deviceScope += " AND id=?"
		deviceArgs = append(deviceArgs, q.DeviceID)
	}
	if e = tx.QueryRowContext(ctx, deviceScope, deviceArgs...).Scan(&out.LastReceivedAt); e != nil {
		return out, e
	}
	pageArgs := append(rangeArgs, PageSize, (q.Page-1)*PageSize)
	rows, e := tx.QueryContext(ctx, `SELECT p.id,p.device_id,d.name,p.recorded_at,p.received_at,p.lat,p.lon,p.accuracy,p.altitude,p.speed,p.bearing,p.provider`+rangeScope+" ORDER BY p.recorded_at DESC,p.id DESC LIMIT ? OFFSET ?", pageArgs...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p Point
		if e = rows.Scan(&p.ID, &p.DeviceID, &p.DeviceName, &p.RecordedAt, &p.ReceivedAt, &p.Lat, &p.Lon, &p.Acc, &p.Alt, &p.Speed, &p.Bearing, &p.Provider); e != nil {
			rows.Close()
			return out, e
		}
		out.Points = append(out.Points, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
