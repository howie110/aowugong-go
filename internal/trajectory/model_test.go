package trajectory

import (
	"net/url"
	"testing"
	"time"
)

func TestParseGPSLoggerForm(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	v := url.Values{"timestamp": {"1792540800"}, "lat": {"0"}, "lon": {"113.25"}, "acc": {"0"}, "alt": {""}, "spd": {"1.5"}, "dir": {"0"}, "prov": {"gps"}}
	p, err := Parse(v, now)
	if err != nil || p.Lat != 0 || p.Acc == nil || *p.Acc != 0 || p.Alt != nil || p.Speed == nil || *p.Speed != 1.5 {
		t.Fatalf("bad parse %+v %v", p, err)
	}
	for _, tc := range []struct{ k, v string }{{"lat", "91"}, {"lon", "NaN"}, {"timestamp", "0"}, {"timestamp", "9999999999"}, {"acc", "-1"}, {"spd", "Inf"}, {"dir", "361"}, {"secret", "password"}} {
		q := url.Values{}
		for k, x := range v {
			q[k] = append([]string{}, x...)
		}
		q.Set(tc.k, tc.v)
		if _, err := Parse(q, now); err == nil {
			t.Errorf("accepted %s", tc.k)
		}
	}
	v.Add("lat", "1")
	if _, err := Parse(v, now); err == nil {
		t.Fatal("accepted duplicate")
	}
}
func TestQueryBeijingBoundary(t *testing.T) {
	q := Query{From: "2026-10-22", To: "2026-10-23", Page: 1}
	from, to, err := q.Bounds()
	if err != nil || from.Format(time.RFC3339) != "2026-10-21T16:00:00Z" || to.Sub(from) != 24*time.Hour {
		t.Fatalf("bad boundary %v %v", from, err)
	}
	q.To = "2026-10-22"
	if _, _, err := q.Bounds(); err == nil {
		t.Fatal("empty range")
	}
}
