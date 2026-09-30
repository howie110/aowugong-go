package blog

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPublishKeepsCurrentOnInvalidContent(t *testing.T) {
	root := t.TempDir()
	first := t.TempDir()
	writeArticle(t, first, "hello.md", articleFixture)
	if err := Publish(first, root, 1); err != nil {
		t.Fatal(err)
	}
	bad := t.TempDir()
	writeArticle(t, bad, "bad.md", "broken")
	if err := Publish(bad, root, 2); err == nil {
		t.Fatal("bad content published")
	}
	a, err := NewArticleStore(filepath.Join(root, "current")).Get("hello")
	if err != nil || a.Title != "测试文章" {
		t.Fatalf("current lost: %v %v", a, err)
	}
}
func TestPublishRejectsOlderSequence(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	if err := Publish(source, root, 10); err != nil {
		t.Fatal(err)
	}
	if err := Publish(source, root, 9); err == nil {
		t.Fatal("accepted stale release")
	}
}
func TestPublishRemovesDeletedArticle(t *testing.T) {
	root := t.TempDir()
	first := t.TempDir()
	writeArticle(t, first, "hello.md", articleFixture)
	writeArticle(t, first, "removed.md", articleFixture)
	if err := Publish(first, root, 1); err != nil {
		t.Fatal(err)
	}
	second := t.TempDir()
	writeArticle(t, second, "hello.md", articleFixture)
	if err := Publish(second, root, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := NewArticleStore(filepath.Join(root, "current")).Get("removed"); err == nil {
		t.Fatal("deleted article survives")
	}
	if _, err := os.Stat(filepath.Join(root, "previous", "removed.md")); err != nil {
		t.Fatal(err)
	}
	if err := Publish(second, root, 3); err != nil {
		t.Fatal(err)
	}
	dirs, err := os.ReadDir(filepath.Join(root, "versions"))
	if err != nil || len(dirs) != 2 {
		t.Fatalf("versions %d %v", len(dirs), err)
	}
}
func TestPublishSerializesActivation(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	var wg sync.WaitGroup
	for _, seq := range []int64{5, 6} {
		wg.Add(1)
		go func(n int64) { defer wg.Done(); _ = Publish(source, root, n) }(seq)
	}
	wg.Wait()
	target, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil || filepath.Base(target) != "6" {
		t.Fatalf("current %s %v", target, err)
	}
}

func TestPublishWaitsForSnapshotReader(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	writeArticle(t, source, "hello.md", articleFixture)
	if err := Publish(source, root, 1); err != nil {
		t.Fatal(err)
	}
	store := NewArticleStore(filepath.Join(root, "current"))
	snapshot, release, err := store.root()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	finished := make(chan error, 1)
	go func() {
		if err := Publish(source, root, 2); err != nil {
			finished <- err
			return
		}
		finished <- Publish(source, root, 3)
	}()
	select {
	case err := <-finished:
		t.Fatalf("publisher passed held reader: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if _, err := readArticle(snapshot, "hello.md"); err != nil {
		t.Fatal(err)
	}
	release()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("publisher never resumed")
	}
}
