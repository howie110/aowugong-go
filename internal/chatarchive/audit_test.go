package chatarchive

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestAuditDatabasePagination(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT.*count").WillReturnRows(sqlmock.NewRows([]string{"c", "m", "f", "s"}).AddRow(2, 4, 1, 9))
	mock.ExpectQuery("WITH latest.*SELECT count").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(4))
	mock.ExpectQuery("WITH latest.*LIMIT ").WithArgs(int64(9), 2, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "key", "role", "body", "time", "source", "session", "device", "observed"}).AddRow(1, "id:a", "user", "first", "2026-01-01T09:00:00+08:00", "codex", "s", "d", "o").AddRow(2, "id:b", "assistant", "second", nil, "codex", "s", "d", "o"))
	page, err := NewRepository(db).ListAudit(context.Background(), AuditFilter{Limit: 2, Order: "asc"})
	if err != nil || len(page.Messages) != 2 || !page.HasMore || page.Total != 4 {
		t.Fatal(page, err)
	}
	if *page.Messages[0].SourceTime != "2026-01-01T01:00:00Z" || page.Messages[1].SourceTime != nil {
		t.Fatal(page.Messages)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRejectsInvalidFilters(t *testing.T) {
	for _, f := range []AuditFilter{{Limit: 0}, {Limit: 20, Order: "DROP"}, {Limit: 20, From: "2026-02-30"}, {Limit: 20, Snapshot: -1}, {Limit: 20, Offset: -1}} {
		if f.Validate() == nil {
			t.Fatalf("accepted %+v", f)
		}
	}
}
