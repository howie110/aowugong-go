// Package analytics 提供本站匿名页面访问采集和聚合查询。
package analytics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid analytics input")
var Shanghai = time.FixedZone("Asia/Shanghai", 8*3600)

// NormalizeURL 仅保存页面路径及允许的营销参数，不保留片段与任意查询数据。
func NormalizeURL(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || u.Host != "" || u.Scheme != "" || strings.ContainsAny(u.Path, "\\\x00\r\n") {
		return "", "", ErrInvalid
	}
	p := path.Clean(u.Path)
	if len(p) > 500 || strings.HasPrefix(p, "/api/") || p == "/api" || strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/feeds/") {
		return "", "", ErrInvalid
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".js", ".css", ".png", ".jpg", ".jpeg", ".webp", ".svg", ".ico", ".xml", ".json", ".woff", ".woff2":
		return "", "", ErrInvalid
	}
	q := url.Values{}
	for _, k := range []string{"utm_source", "utm_medium", "utm_campaign"} {
		if v := u.Query().Get(k); v != "" && len(v) <= 100 {
			q.Set(k, v)
		}
	}
	address := p
	if len(q) > 0 {
		address += "?" + q.Encode()
	}
	return p, address, nil
}

// VisitorID 使用独立用途标签与月度轮换生成不可直接回推 IP 的匿名标识。
func VisitorID(secret, ip, ua string, now time.Time) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("aowugong-analytics-v1\x00" + now.In(Shanghai).Format("2006-01") + "\x00" + ip + "\x00" + ua))
	return hex.EncodeToString(h.Sum(nil))
}
func ReferrerDomain(raw, host string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	d := strings.ToLower(u.Hostname())
	if d == strings.ToLower(host) || len(d) > 253 {
		return ""
	}
	return d
}
func Channel(referrer, medium string) string {
	switch strings.ToLower(medium) {
	case "email":
		return "email"
	case "cpc", "ppc", "paid":
		return "paid"
	case "social":
		return "social"
	}
	for _, d := range []string{"google.com", "google.com.hk", "bing.com", "baidu.com", "duckduckgo.com", "yahoo.com", "sogou.com"} {
		if referrer == d || strings.HasSuffix(referrer, "."+d) {
			return "search"
		}
	}
	for _, d := range []string{"t.co", "twitter.com", "x.com", "facebook.com", "weibo.com", "zhihu.com", "reddit.com"} {
		if referrer == d || strings.HasSuffix(referrer, "."+d) {
			return "social"
		}
	}
	if referrer == "" {
		if medium != "" {
			return "unknown"
		}
		return "direct"
	}
	return "referral"
}
