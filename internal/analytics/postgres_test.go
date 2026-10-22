package analytics

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

func TestPostgresCollectionAndReport(t *testing.T) {
	dsn := os.Getenv("ANALYTICS_TEST_DSN")
	if dsn == "" {
		t.Skip("ANALYTICS_TEST_DSN not supplied")
	}
	ctx := context.Background()
	db, err := database.OpenPostgres(ctx, config.Database{URL: dsn + "&search_path=analytics_unit", MaxOpenConns: 4, MaxIdleConns: 4, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow("SELECT current_database()").Scan(&name); err != nil || name != "analytics_test" {
		t.Fatal("refusing non-test database")
	}
	if _, err = db.Exec("CREATE SCHEMA analytics_unit"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA analytics_unit CASCADE")
	migration, err := os.ReadFile("../../migrations/postgres/00016_analytics.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
		t.Fatal(err)
	}

	s, err := NewService(db, "analytics-test-secret-only", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, Shanghai)
	s.now = func() time.Time { return now }
	p := Payload{ID: uuid.NewString(), URL: "/blog?token=secret", Referrer: "https://www.google.com/search?q=secret", Language: "zh-CN"}
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36"
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := s.Collect(ctx, p, "192.0.2.1", ua, "aowugong.top"); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	now = now.Add(2 * time.Minute)
	p.ID = uuid.NewString()
	p.URL = "/work"
	if err = s.Collect(ctx, p, "192.0.2.1", ua, "aowugong.top"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)
	p.ID = uuid.NewString()
	if err = s.Collect(ctx, p, "192.0.2.1", ua, "aowugong.top"); err != nil {
		t.Fatal(err)
	}
	f := Filter{From: now.Add(-time.Hour), To: now.Add(time.Second)}
	r, err := s.Report(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if r.Current.Visitors != 1 || r.Current.Visits != 2 || r.Current.Views != 3 || r.Current.BounceRate != 50 || r.Current.Duration != 60 {
		t.Fatalf("bad metrics %+v", r.Current)
	}
	if len(r.Ranks["entry"]) != 2 || r.Ranks["entry"][0].Count != 1 {
		t.Fatalf("entry overcount %+v", r.Ranks["entry"])
	}
	if r.Previous.Views != 0 || len(r.Heat) != 1 || r.Heat[0].Count != 3 {
		t.Fatalf("bad report %+v", r)
	}
	f.Values = map[string]string{"path": "/blog"}
	r, err = s.Report(ctx, f)
	if err != nil || r.Current.Views != 1 || r.Current.BounceRate != 100 {
		t.Fatalf("filter %+v %v", r, err)
	}
	f.Values = map[string]string{"path": "' OR 1=1 --"}
	r, err = s.Report(ctx, f)
	if err != nil || r.Current.Views != 0 {
		t.Fatalf("injection %+v %v", r, err)
	}
	f.Values = nil
	f.To = now
	r, err = s.Report(ctx, f)
	if err != nil || r.Current.Views != 2 {
		t.Fatalf("exclusive end %+v %v", r, err)
	}
	var url string
	if err = db.QueryRow("SELECT url FROM analytics_events WHERE path='/blog'").Scan(&url); err != nil || url != "/blog" {
		t.Fatal("secret retained", url, err)
	}
	old := uuid.NewString()
	if _, err = db.Exec("INSERT INTO analytics_visits(id,visitor_id,started_at,last_at,entry_path,exit_path) VALUES(?,?,?,?,?,?)", old, "expired", now.AddDate(0, 0, -366), now.AddDate(0, 0, -366), "/old", "/old"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO analytics_events(id,visit_id,visitor_id,created_at,path,url) VALUES(?,?,?,?,?,?)", uuid.NewString(), old, "expired", now.AddDate(0, 0, -366), "/old", "/old"); err != nil {
		t.Fatal(err)
	}
	s.lastPrune.Store(0)
	s.pruneExpired(ctx, now)
	var remaining int
	if err = db.QueryRow("SELECT count(*) FROM analytics_events").Scan(&remaining); err != nil || remaining != 3 {
		t.Fatal("retention", remaining, err)
	}

}
