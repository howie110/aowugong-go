package analytics

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mileusna/useragent"
	"github.com/oschwald/maxminddb-golang"
)

type Payload struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Referrer string `json:"referrer"`
	Language string `json:"language"`
}
type Service struct {
	lastPrune atomic.Int64
	db        *sql.DB
	secret    string
	geo       *maxminddb.Reader
	now       func() time.Time
}

// NewService 加载可选的本地 GeoLite2 City 数据；不会联网下载地理数据。
func NewService(db *sql.DB, secret, geoPath string) (*Service, error) {
	if len(secret) < 16 {
		return nil, errors.New("analytics secret must contain at least 16 characters")
	}
	s := &Service{db: db, secret: secret, now: time.Now}
	if geoPath != "" {
		b, err := os.ReadFile(geoPath)
		if err != nil {
			return nil, err
		}
		s.geo, err = maxminddb.FromBytes(b)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Collect 原子写入浏览事件和访问边界；UUID 重传不重复计数。
func (s *Service) Collect(ctx context.Context, p Payload, ip, ua, host string) error {
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return ErrInvalid
	}
	path, address, err := NormalizeURL(p.URL)
	if err != nil {
		return err
	}
	if len(p.Language) > 35 || len(p.Referrer) > 4096 || len(ua) > 2048 {
		return ErrInvalid
	}
	agent := useragent.Parse(ua)
	if agent.Bot {
		return nil
	}
	now := s.now().UTC()
	visitor := VisitorID(s.secret, ip, ua, now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 同一访客串行分组，防止多标签页同时创建两次访问。
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?,0))", visitor); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM analytics_events WHERE id=?)", id.String()).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	var visit string
	err = tx.QueryRowContext(ctx, "SELECT id FROM analytics_visits WHERE visitor_id=? AND last_at>? ORDER BY last_at DESC LIMIT 1", visitor, now.Add(-30*time.Minute)).Scan(&visit)
	if errors.Is(err, sql.ErrNoRows) {
		visit = uuid.NewString()
		_, err = tx.ExecContext(ctx, "INSERT INTO analytics_visits(id,visitor_id,started_at,last_at,entry_path,exit_path) VALUES(?,?,?,?,?,?)", visit, visitor, now, now, path, path)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE analytics_visits SET last_at=?,exit_path=? WHERE id=?", now, path, visit)
	}
	if err != nil {
		return err
	}
	device := "desktop"
	if agent.Mobile {
		device = "mobile"
	}
	if agent.Tablet {
		device = "tablet"
	}
	ref := ReferrerDomain(p.Referrer, host)
	u, _ := url.Parse(address)
	country, region, city := s.location(net.ParseIP(ip))
	_, err = tx.ExecContext(ctx, `INSERT INTO analytics_events(id,visit_id,visitor_id,created_at,path,url,referrer,channel,browser,os,device,language,country,region,city) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id.String(), visit, visitor, now, path, address, ref, Channel(ref, u.Query().Get("utm_medium")), agent.Name, agent.OS, device, strings.ToLower(p.Language), country, region, city)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.pruneExpired(ctx, now)
	return nil
}
func (s *Service) location(ip net.IP) (string, string, string) {
	if s.geo == nil || ip == nil {
		return "", "", ""
	}
	var record struct {
		Country struct {
			ISO string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
	}
	if s.geo.Lookup(ip, &record) != nil {
		return "", "", ""
	}
	name := func(m map[string]string) string {
		if m["zh-CN"] != "" {
			return m["zh-CN"]
		}
		return m["en"]
	}
	region := ""
	if len(record.Subdivisions) > 0 {
		region = name(record.Subdivisions[0].Names)
	}
	return record.Country.ISO, region, name(record.City.Names)
}
