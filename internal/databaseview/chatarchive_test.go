package databaseview

import (
	"context"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"testing"
)

func TestArchiveTablesExcludedFromGenericBrowser(t *testing.T) {
	db := testdatabase.Open(t)
	if _, err := db.Exec("CREATE TABLE chat_archive_raw_batches(id INTEGER PRIMARY KEY, raw TEXT)"); err != nil {
		t.Fatal(err)
	}
	service := newService(db, "sqlite")
	names, err := service.tableNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == "chat_archive_raw_batches" {
			t.Fatal("chat archive exposed through generic database browser")
		}
	}
	if _, err := service.Rows(context.Background(), "chat_archive_raw_batches", "", 1, 20); err == nil {
		t.Fatal("archive table readable")
	}
}
