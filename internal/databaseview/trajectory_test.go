package databaseview

import (
	"context"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"testing"
)

func TestTrajectoryExcludedFromGenericBrowser(t *testing.T) {
	db := testdatabase.Open(t)
	_, e := db.Exec("CREATE TABLE trajectory_points(id INTEGER,lat REAL)")
	if e != nil {
		t.Fatal(e)
	}
	s := newService(db, "sqlite")
	if _, e = s.Rows(context.Background(), "trajectory_points", "", 1, 20); e == nil {
		t.Fatal("trajectory bypasses owner filtering")
	}
}
