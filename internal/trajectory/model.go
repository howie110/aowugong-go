// Package trajectory 接收 GPSLogger 轨迹，按设备所属账号隔离原始数据。
package trajectory

import (
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/howiedata/aowugong-go/internal/timezone"
)

var (
	ErrInvalid      = errors.New("轨迹参数无效")
	ErrUnauthorized = errors.New("设备密钥无效或已撤销")
	ErrNotFound     = errors.New("设备不存在")
	ErrLimit        = errors.New("每个账号最多创建 20 台设备")
)
var Shanghai = timezone.Beijing

const PageSize = 50

type Point struct {
	ID         int64             `json:"id"`
	DeviceID   string            `json:"device_id"`
	DeviceName string            `json:"device_name"`
	RecordedAt time.Time         `json:"recorded_at"`
	ReceivedAt time.Time         `json:"received_at"`
	Lat        float64           `json:"lat"`
	Lon        float64           `json:"lon"`
	Acc        *float64          `json:"accuracy"`
	Alt        *float64          `json:"altitude"`
	Speed      *float64          `json:"speed"`
	Bearing    *float64          `json:"bearing"`
	Provider   string            `json:"provider"`
	Raw        map[string]string `json:"-"`
}
type Device struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
	LastReceivedAt *time.Time `json:"last_received_at"`
}
type Credential struct {
	Device Device `json:"device"`
	Token  string `json:"token"`
}
type Query struct {
	From, To, DeviceID string
	Page               int
	Snapshot           time.Time
}
type Report struct {
	Points         []Point    `json:"points"`
	Total          int64      `json:"total"`
	TodayCount     int64      `json:"today_count"`
	LastRecordedAt *time.Time `json:"last_recorded_at"`
	LastReceivedAt *time.Time `json:"last_received_at"`
	Page           int        `json:"page"`
	PageSize       int        `json:"page_size"`
	Snapshot       time.Time  `json:"snapshot"`
}

func (q Query) Bounds() (time.Time, time.Time, error) {
	a, e1 := time.ParseInLocation("2006-01-02", q.From, Shanghai)
	b, e2 := time.ParseInLocation("2006-01-02", q.To, Shanghai)
	if e1 != nil || e2 != nil || !b.After(a) || b.Sub(a) > 366*24*time.Hour || q.Page < 1 || q.Page > 20000 {
		return a, b, ErrInvalid
	}
	return a.UTC(), b.UTC(), nil
}
func validName(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 60 && !strings.ContainsAny(s, "\r\n\x00")
}

// Parse 接受 GPSLogger 的 Unix 秒与白名单字段，未知/重复参数拒绝保存。
func Parse(v url.Values, now time.Time) (Point, error) {
	p := Point{Raw: map[string]string{}}
	for k, x := range v {
		switch k {
		case "timestamp", "lat", "lon", "acc", "alt", "spd", "dir", "prov":
		default:
			return p, ErrInvalid
		}
		if len(x) != 1 || len(x[0]) > 80 {
			return p, ErrInvalid
		}
		p.Raw[k] = x[0]
	}
	stamp, err := strconv.ParseInt(v.Get("timestamp"), 10, 64)
	if err != nil || stamp < 946684800 || stamp > now.Add(10*time.Minute).Unix() {
		return p, ErrInvalid
	}
	p.RecordedAt = time.Unix(stamp, 0).UTC()
	number := func(k string, required bool, min, max float64) (*float64, error) {
		s := v.Get(k)
		if s == "" && !required {
			return nil, nil
		}
		f, e := strconv.ParseFloat(s, 64)
		if e != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < min || f > max {
			return nil, ErrInvalid
		}
		return &f, nil
	}
	lat, e := number("lat", true, -90, 90)
	if e != nil {
		return p, e
	}
	lon, e := number("lon", true, -180, 180)
	if e != nil {
		return p, e
	}
	p.Lat = *lat
	p.Lon = *lon
	if p.Acc, e = number("acc", false, 0, 1e7); e != nil {
		return p, e
	}
	if p.Alt, e = number("alt", false, -1e5, 1e7); e != nil {
		return p, e
	}
	if p.Speed, e = number("spd", false, 0, 1e5); e != nil {
		return p, e
	}
	if p.Bearing, e = number("dir", false, 0, 360); e != nil {
		return p, e
	}
	p.Provider = v.Get("prov")
	if len(p.Provider) > 32 || strings.ContainsAny(p.Provider, "\r\n\x00") {
		return p, ErrInvalid
	}
	return p, nil
}
