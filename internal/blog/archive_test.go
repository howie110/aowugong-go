package blog

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
	"os"
	"path/filepath"
	"testing"
)

func makeArchive(t *testing.T, filename string, headers []*tar.Header) {
	t.Helper()
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(articleFixture)); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestArchiveRejectsTraversalAndLinks(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../escape.md", Typeflag: tar.TypeReg, Size: int64(len(articleFixture))}, {Name: "/absolute.md", Typeflag: tar.TypeReg, Size: int64(len(articleFixture))}, {Name: "linked.md", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}} {
		archive := filepath.Join(t.TempDir(), "content.tar.gz")
		makeArchive(t, archive, []*tar.Header{h})
		if err := NewArticleRepository(testdatabase.Open(t)).PublishArchive(context.Background(), archive, 1); err == nil {
			t.Fatalf("accepted %+v", h)
		}
	}
}
func TestArchivePublishesOnlyValidatedContent(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "content.tar.gz")
	makeArchive(t, archive, []*tar.Header{{Name: "hello.md", Typeflag: tar.TypeReg, Size: int64(len(articleFixture))}})
	repo := NewArticleRepository(testdatabase.Open(t))
	if err := repo.PublishArchive(context.Background(), archive, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
}
