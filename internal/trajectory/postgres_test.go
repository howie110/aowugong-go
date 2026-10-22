package trajectory

import (
	"context"
	"github.com/google/uuid"
	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresTrajectoryLifecycle(t *testing.T) {
	dsn := os.Getenv("TRAJECTORY_TEST_DSN")
	if dsn == "" {
		t.Skip("TRAJECTORY_TEST_DSN not supplied")
	}
	schema := "trajectory_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ctx := context.Background()
	db, e := database.OpenPostgres(ctx, config.Database{URL: dsn + "&search_path=" + schema, MaxOpenConns: 6, MaxIdleConns: 4, ConnMaxLifetime: time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var databaseName string
	if e = db.QueryRow("SELECT current_database()").Scan(&databaseName); e != nil || databaseName != "analytics_test" {
		t.Fatal("refusing non-test database")
	}
	if _, e = db.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	_, e = db.Exec(`CREATE TABLE aowugong_fastapi_users(id BIGINT PRIMARY KEY,is_active INTEGER,is_superuser INTEGER);INSERT INTO aowugong_fastapi_users VALUES(1,1,1),(2,1,1);CREATE TABLE aowugong_roles(id BIGINT,code TEXT,is_active INTEGER);CREATE TABLE aowugong_user_roles(user_id BIGINT,role_id BIGINT);`)
	if e != nil {
		t.Fatal(e)
	}
	m, e := os.ReadFile("../../migrations/postgres/00015_trajectory.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(strings.Split(string(m), "-- +goose Down")[0]); e != nil {
		t.Fatal(e)
	}
	s := NewService(db)
	cred, e := s.Create(ctx, 1, "我的手机")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Second)
	p := Point{RecordedAt: now.Add(-24 * time.Hour), Lat: 23, Lon: 113, Raw: map[string]string{"lat": "23"}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Collect(ctx, cred.Token, p); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	day := now.In(Shanghai)
	q := Query{From: day.AddDate(0, 0, -2).Format("2006-01-02"), To: day.AddDate(0, 0, 1).Format("2006-01-02"), Page: 1}
	r, e := s.Report(ctx, 1, q)
	if e != nil || r.Total != 1 || r.TodayCount != 0 || r.LastReceivedAt == nil {
		t.Fatalf("report %+v %v", r, e)
	}
	r2, e := s.Report(ctx, 2, q)
	if e != nil || r2.Total != 0 {
		t.Fatalf("owner leak %+v %v", r2, e)
	}
	if _, e = s.Rotate(ctx, 2, cred.Device.ID); e != ErrNotFound {
		t.Fatalf("rotate owner %v", e)
	}
	for i := 0; i < 55; i++ {
		p.RecordedAt = now.Add(time.Duration(-i) * time.Second)
		if _, e = s.Collect(ctx, cred.Token, p); e != nil {
			t.Fatal(e)
		}
	}
	r, e = s.Report(ctx, 1, q)
	if e != nil || len(r.Points) != 50 || r.Total != 56 {
		t.Fatalf("page %+v %v", r, e)
	}
	q.Snapshot = r.Snapshot
	q.Page = 2
	p.RecordedAt = now.Add(time.Second)
	if _, e = s.Collect(ctx, cred.Token, p); e != nil {
		t.Fatal(e)
	}
	r, e = s.Report(ctx, 1, q)
	if e != nil || len(r.Points) != 6 || r.Total != 56 {
		t.Fatalf("snapshot %+v %v", r, e)
	}
	newer, e := s.Rotate(ctx, 1, cred.Device.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Collect(ctx, cred.Token, p); e != ErrUnauthorized {
		t.Fatalf("old token %v", e)
	}
	if _, e = s.Collect(ctx, newer.Token, p); e != nil {
		t.Fatal(e)
	}
	if e = s.Revoke(ctx, 2, cred.Device.ID); e != ErrNotFound {
		t.Fatal(e)
	}
	if e = s.Revoke(ctx, 1, cred.Device.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Collect(ctx, newer.Token, p); e != ErrUnauthorized {
		t.Fatal(e)
	}
	devices, e := s.Devices(ctx, 1)
	if e != nil || len(devices) != 1 || devices[0].RevokedAt == nil {
		t.Fatal(devices, e)
	}
}
