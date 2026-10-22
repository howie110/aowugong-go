package chatarchive

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListConversationsUsesBoundFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT c.id").WithArgs("codex", "2026-01-01", "2026-01-02", "%title%", "%'; DROP TABLE x;--%", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "source", "source_id", "title", "created_at", "updated_at", "account_scope"}).AddRow(1, "codex", "s", "title", "2026-01-01", "2026-01-02", "account"))
	rows, err := NewRepository(db).ListConversations(context.Background(), Filter{Source: "codex", From: "2026-01-01", To: "2026-01-02", Title: "title", Query: "'; DROP TABLE x;--", Limit: 20})
	if err != nil || len(rows) != 1 || rows[0].Title != "title" {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidFiltersRejected(t *testing.T) {
	for _, f := range []Filter{{Limit: 0}, {Limit: 201}, {Limit: 20, From: "bad"}, {Limit: 20, From: "2026-02-01", To: "2026-01-01"}, {Limit: 20, Source: "unknown"}} {
		if f.Validate() == nil {
			t.Fatalf("accepted invalid filter: %+v", f)
		}
	}
}
