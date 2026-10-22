package receiptsplit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientValidatesDestinationAndDisabledState(t *testing.T) {
	for _, address := range []string{"https://example.com", "http://127.0.0.1:80/path", "http://user:pass@127.0.0.1", "http://localhost:8080"} {
		if _, err := NewClient(address, "01234567890123456789012345678901"); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
	c, err := NewClient("", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Statistics(context.Background(), Query{From: "2026-10-21", To: "2026-10-22"}); err != ErrNotConfigured {
		t.Fatal(err)
	}
}

func TestClientExpiredCacheDoesNotHideFailure(t *testing.T) {
	var fail atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"version":1,"from":"2026-10-21","to":"2026-10-22","generated_at":"2026-10-22T00:00:00Z","days":[{"date":"2026-10-21"}]}`)
	}))
	defer upstream.Close()
	c, err := NewClient(upstream.URL, "01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	q := Query{From: "2026-10-21", To: "2026-10-22"}
	if _, err := c.Statistics(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	v := c.cache[q]
	v.until = time.Now().Add(-time.Second)
	c.cache[q] = v
	c.mu.Unlock()
	fail.Store(true)
	if _, err := c.Statistics(context.Background(), q); err != ErrUpstream {
		t.Fatalf("expired result hid upstream failure: %v", err)
	}
}

func TestClientRejectsIncompleteDaySequence(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":1,"from":"2026-10-20","to":"2026-10-22","generated_at":"2026-10-22T00:00:00Z","days":[{"date":"2026-10-21"}]}`)
	}))
	defer upstream.Close()
	c, err := NewClient(upstream.URL, "01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Statistics(context.Background(), Query{From: "2026-10-20", To: "2026-10-22"}); err != ErrUpstream {
		t.Fatalf("accepted missing day: %v", err)
	}
}
func TestClientCachesAndRejectsRedirects(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer 01234567890123456789012345678901" {
			t.Error("missing secret")
		}
		fmt.Fprint(w, `{"version":1,"from":"2026-10-21","to":"2026-10-22","exclude_operators":true,"generated_at":"2026-10-22T00:00:00Z","days":[{"date":"2026-10-21"}]}`)
	}))
	defer upstream.Close()
	c, err := NewClient(upstream.URL, "01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	q := Query{From: "2026-10-21", To: "2026-10-22", ExcludeOperators: true}
	for i := 0; i < 2; i++ {
		if _, err = c.Statistics(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cache miss")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, upstream.URL, 302) }))
	defer redirect.Close()
	c, _ = NewClient(redirect.URL, "01234567890123456789012345678901")
	if _, err = c.Statistics(context.Background(), q); err == nil {
		t.Fatal("followed redirect")
	}
	if calls.Load() != 1 {
		t.Fatal("secret forwarded")
	}
}
