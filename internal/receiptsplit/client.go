// Package receiptsplit 从同机小票分账服务读取匿名汇总，不访问其数据库。
package receiptsplit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrNotConfigured = errors.New("receipt statistics not configured")
var ErrInvalid = errors.New("invalid receipt statistics range")
var ErrUpstream = errors.New("receipt statistics unavailable")

type Query struct {
	From, To         string
	ExcludeOperators bool
}

func (q Query) Validate() error {
	from, e1 := time.Parse("2006-01-02", q.From)
	to, e2 := time.Parse("2006-01-02", q.To)
	if e1 != nil || e2 != nil || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return ErrInvalid
	}
	return nil
}

type Counts struct {
	AnalysisUsers int64 `json:"analysis_users"`
	Tasks         int64 `json:"tasks"`
	Succeeded     int64 `json:"succeeded"`
	Failed        int64 `json:"failed"`
	Running       int64 `json:"running"`
	Queued        int64 `json:"queued"`
	BillUsers     int64 `json:"bill_users"`
	Bills         int64 `json:"bills"`
}
type Day struct {
	Date string `json:"date"`
	Counts
}
type Report struct {
	Version             int       `json:"version"`
	From                string    `json:"from"`
	To                  string    `json:"to"`
	ExcludeOperators    bool      `json:"exclude_operators"`
	OperatorsConfigured bool      `json:"operators_configured"`
	GeneratedAt         time.Time `json:"generated_at"`
	LifetimeLoginUsers  int64     `json:"lifetime_login_users"`
	Totals              Counts    `json:"totals"`
	Days                []Day     `json:"days"`
}
type cached struct {
	report Report
	until  time.Time
}
type Client struct {
	base, key string
	http      *http.Client
	mu        sync.Mutex
	cache     map[Query]cached
}

// NewClient 只接受 HTTP 回环 IP 根地址，防止配置错误把专用 Key 发往外部。
func NewClient(address, key string) (*Client, error) {
	c := &Client{cache: map[Query]cached{}, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if address == "" && key == "" {
		return c, nil
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("小票统计地址必须为 HTTP 回环 IP 根地址")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("小票统计仅允许回环 IP")
	}
	if len(key) < 32 || len(key) > 256 || strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return nil, errors.New("小票统计 Key 必须为 32–256 位非空白 ASCII")
	}
	c.base = strings.TrimRight(address, "/")
	c.key = key
	return c, nil
}

// Statistics 缓存仅保存汇总，过期后拉取；上游异常不以旧数字伪装成功。
func (c *Client) Statistics(ctx context.Context, q Query) (Report, error) {
	if c == nil || c.base == "" {
		return Report{}, ErrNotConfigured
	}
	if err := q.Validate(); err != nil {
		return Report{}, err
	}
	c.mu.Lock()
	v, ok := c.cache[q]
	c.mu.Unlock()
	if ok && time.Now().Before(v.until) {
		return v.report, nil
	}
	values := url.Values{"from": {q.From}, "to": {q.To}, "exclude_operators": {"false"}}
	if q.ExcludeOperators {
		values.Set("exclude_operators", "true")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/internal/statistics?"+values.Encode(), nil)
	if err != nil {
		return Report{}, ErrUpstream
	}
	request.Header.Set("Authorization", "Bearer "+c.key)
	response, err := c.http.Do(request)
	if err != nil {
		return Report{}, ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Report{}, ErrUpstream
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return Report{}, ErrUpstream
	}
	var result Report
	if json.Unmarshal(body, &result) != nil || result.Version != 1 || result.From != q.From || result.To != q.To || result.ExcludeOperators != q.ExcludeOperators || result.GeneratedAt.IsZero() {
		return Report{}, ErrUpstream
	}
	from, _ := time.Parse("2006-01-02", q.From)
	to, _ := time.Parse("2006-01-02", q.To)
	if len(result.Days) != int(to.Sub(from)/(24*time.Hour)) || result.LifetimeLoginUsers < 0 || !validCounts(result.Totals) {
		return Report{}, ErrUpstream
	}
	for i, day := range result.Days {
		if day.Date != from.AddDate(0, 0, i).Format("2006-01-02") || !validCounts(day.Counts) {
			return Report{}, ErrUpstream
		}
	}
	c.mu.Lock()
	if len(c.cache) >= 32 {
		c.cache = map[Query]cached{}
	}
	c.cache[q] = cached{result, time.Now().Add(30 * time.Second)}
	c.mu.Unlock()
	return result, nil
}
func validCounts(c Counts) bool {
	return c.AnalysisUsers >= 0 && c.Tasks >= 0 && c.Succeeded >= 0 && c.Failed >= 0 && c.Running >= 0 && c.Queued >= 0 && c.BillUsers >= 0 && c.Bills >= 0 && c.AnalysisUsers <= c.Tasks && c.BillUsers <= c.Bills && c.Succeeded+c.Failed+c.Running+c.Queued == c.Tasks
}
