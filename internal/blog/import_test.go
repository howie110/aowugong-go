package blog

import (
	"context"
	"strings"
	"testing"
)

const legacyFixture = "---\ntitle: 动态\ndate: 2000-01-05\n---\n---\n\n第一条 [链接](https://example.com)\n\n![图](https://pic.aowugong.top/pic/a.jpg)\n\n2026-07-24 22:18:53\n\n---\n第二条\n\n---\n正文中的分隔线\n\n2026-06-09 09:10:00\n---\n"

func TestLegacyImportPreservesTimeOrderAndImages(t *testing.T) {
	rows, err := ParseLegacyStatuses([]byte(legacyFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].PublishedAt.Format("2006-01-02T15:04:05Z07:00") != "2026-07-24T22:18:53+08:00" || len(rows[0].Images) != 1 || rows[0].Images[0].Key != "pic/a.jpg" || !strings.Contains(rows[1].Body, "正文中的分隔线") {
		t.Fatalf("rows %+v", rows)
	}
	if !strings.Contains(rows[0].Body, "https://example.com") {
		t.Fatal("link lost")
	}
}
func TestLegacyImportRejectsAmbiguousBlock(t *testing.T) {
	if _, err := ParseLegacyStatuses([]byte(strings.Replace(legacyFixture, "2026-06-09 09:10:00", "unknown date", 1))); err == nil {
		t.Fatal("guessed date")
	}
}
func TestLegacyImportIsIdempotent(t *testing.T) {
	db := statusDatabase(t)
	s := NewStatusService(NewRepository(db))
	ctx := context.Background()
	rows, err := ParseLegacyStatuses([]byte(legacyFixture))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.ImportLegacy(ctx, 1, rows); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := s.List(ctx, true, 20, 0)
	if err != nil || len(stored) != 2 {
		t.Fatalf("count %d err %v", len(stored), err)
	}
	input := StatusInput{ID: stored[0].ID, Body: stored[0].Body + " edited", Images: stored[0].Images, Published: true}
	if _, err := s.Save(ctx, 1, input); err != nil {
		t.Fatalf("editing imported image failed: %v", err)
	}
}
