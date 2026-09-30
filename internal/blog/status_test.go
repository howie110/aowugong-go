package blog

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

func statusDatabase(t *testing.T) *sql.DB {
	t.Helper()
	if dsn := os.Getenv("BLOG_TEST_DATABASE_URL"); dsn != "" {
		db, err := database.OpenPostgres(context.Background(), config.Database{URL: dsn, MaxOpenConns: 4, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		if _, err := db.Exec(`DROP TABLE IF EXISTS blog_statuses`); err != nil {
			t.Fatal(err)
		}
		schema, err := os.ReadFile("../../migrations/postgres/00010_blog_statuses.sql")
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(string(schema), "-- +goose Down")[0]
		if _, err := db.Exec(up); err != nil {
			t.Fatal(err)
		}
		return db
	}
	return testdatabase.Open(t)
}
func TestPublicStatusesExcludeDrafts(t *testing.T) {
	s := NewStatusService(NewRepository(statusDatabase(t)))
	ctx := context.Background()
	for _, in := range []StatusInput{{ID: "11111111-1111-4111-8111-111111111111", Body: "draft", Create: true}, {ID: "22222222-2222-4222-8222-222222222222", Body: "public", Published: true, Create: true}} {
		if _, err := s.Save(ctx, 1, in); err != nil {
			t.Fatal(err)
		}
	}
	public, err := s.List(ctx, true, 20, 0)
	if err != nil || len(public) != 1 || public[0].Body != "public" || public[0].PublishedAt == nil {
		t.Fatalf("public %v %v", public, err)
	}
	all, err := s.List(ctx, false, 20, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all %v %v", all, err)
	}
}
func TestStatusRequiresContentAndAtMostNineImages(t *testing.T) {
	s := NewStatusService(NewRepository(statusDatabase(t)))
	ctx := context.Background()
	for _, in := range []StatusInput{{ID: "bad", Body: "x", Create: true}, {ID: "11111111-1111-4111-8111-111111111111", Create: true}, {ID: "11111111-1111-4111-8111-111111111111", Body: "x", Images: make([]Image, 10), Create: true}, {ID: "11111111-1111-4111-8111-111111111111", Body: "x", Images: []Image{{Key: "pic/private.jpg", URL: "https://evil.test/x"}}, Create: true}} {
		if _, err := s.Save(ctx, 1, in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted invalid %+v: %v", in, err)
		}
	}
}
func TestStatusCreateIsIdempotent(t *testing.T) {
	s := NewStatusService(NewRepository(statusDatabase(t)))
	ctx := context.Background()
	in := StatusInput{ID: "11111111-1111-4111-8111-111111111111", Body: "hello", Published: true, Create: true}
	first, err := s.Save(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Save(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if !first.PublishedAt.Equal(*second.PublishedAt) {
		t.Fatal("retry changed publish time")
	}
	in.Body = "overwrite"
	if _, err := s.Save(ctx, 1, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("retry mutated status: %v", err)
	}
	rows, err := s.List(ctx, false, 20, 0)
	if err != nil || len(rows) != 1 || rows[0].Body != "hello" {
		t.Fatalf("rows %v %v", rows, err)
	}
	in.Create = false
	if _, err := s.Save(ctx, 1, in); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, in.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, in.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing delete %v", err)
	}
}
