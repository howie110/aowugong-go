package blog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

func TestDatabaseArticlesSurviveSourceRemoval(t *testing.T) {
	ctx := context.Background()
	db := testdatabase.Open(t)
	store := NewArticleRepository(db)
	source := t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	if err := store.PublishDirectory(ctx, source, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	a, err := store.Get(ctx, "hello")
	if err != nil || a.Title != "测试文章" || !strings.Contains(a.HTML, "section-") {
		t.Fatalf("article %+v %v", a, err)
	}
	var raw string
	if err := db.QueryRow("SELECT markdown FROM blog_articles WHERE slug = ?", "hello").Scan(&raw); err != nil || raw != articleFixture {
		t.Fatalf("raw markdown %q %v", raw, err)
	}
}
func TestArticlePublishReplacesSnapshotAtomically(t *testing.T) {
	ctx := context.Background()
	db := testdatabase.Open(t)
	store := NewArticleRepository(db)
	source := t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	writeArticle(t, source, "removed.md", articleFixture)
	if err := store.PublishDirectory(ctx, source, 2); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(source, "removed.md"))
	writeArticle(t, source, "hello.md", strings.Replace(articleFixture, "测试文章", "新标题", 1))
	if err := store.PublishDirectory(ctx, source, 3); err != nil {
		t.Fatal(err)
	}
	rows, err := store.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].Title != "新标题" {
		t.Fatalf("snapshot %+v %v", rows, err)
	}
	if err := store.PublishDirectory(ctx, source, 2); err == nil {
		t.Fatal("accepted stale publish")
	}
	writeArticle(t, source, "bad.md", "invalid")
	if err := store.PublishDirectory(ctx, source, 4); err == nil {
		t.Fatal("accepted malformed article")
	}
	rows, err = store.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].Title != "新标题" {
		t.Fatal("failed publish changed current rows")
	}
	var sequence int
	if err := db.QueryRow("SELECT sequence FROM blog_article_publication WHERE id = 1").Scan(&sequence); err != nil || sequence != 3 {
		t.Fatalf("sequence %d %v", sequence, err)
	}
}
func TestArticlePublishRollsBackDatabaseFailure(t *testing.T) {
	ctx := context.Background()
	db := testdatabase.Open(t)
	store := NewArticleRepository(db)
	source := t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	if err := store.PublishDirectory(ctx, source, 1); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`CREATE TRIGGER reject_article BEFORE INSERT ON blog_articles BEGIN SELECT RAISE(ABORT, 'test database failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PublishDirectory(ctx, source, 2); err == nil {
		t.Fatal("database error hidden")
	}
	rows, err := store.List(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal("transaction did not restore articles")
	}
	var sequence int
	db.QueryRow("SELECT sequence FROM blog_article_publication WHERE id=1").Scan(&sequence)
	if sequence != 1 {
		t.Fatal("failed transaction advanced sequence")
	}
}

func TestArticleAssetsStoredAndReplaced(t *testing.T) {
	ctx := context.Background()
	repo := NewArticleRepository(testdatabase.Open(t))
	source := t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	picture := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00")
	if err := os.WriteFile(filepath.Join(source, "picture.gif"), picture, 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishDirectory(ctx, source, 1); err != nil {
		t.Fatal(err)
	}
	asset, err := repo.Asset(ctx, "picture.gif")
	if err != nil || string(asset.Content) != string(picture) || asset.ContentType != "image/gif" {
		t.Fatalf("asset = %+v, %v", asset, err)
	}
	if err := os.Remove(filepath.Join(source, "picture.gif")); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishDirectory(ctx, source, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Asset(ctx, "picture.gif"); err != ErrNotFound {
		t.Fatalf("deleted asset: %v", err)
	}
}
