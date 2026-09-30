package blog

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"
)

type recordingMediaWriter struct {
	key, kind string
	data      []byte
	err       error
}

func (w *recordingMediaWriter) Put(ctx context.Context, key string, r io.Reader, kind string) error {
	w.key = key
	w.kind = kind
	w.data, _ = io.ReadAll(r)
	return w.err
}
func pngFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestImageUploadUsesBlogPrefix(t *testing.T) {
	writer := &recordingMediaWriter{}
	s := newMediaStore(writer)
	data := pngFixture(t)
	image, err := s.Upload(context.Background(), bytes.NewReader(data), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(image.Key, "blog/status/") || image.URL != ImageBaseURL+image.Key || writer.kind != "image/png" || !bytes.Equal(writer.data, data) {
		t.Fatalf("wrong upload: %+v %+v", image, writer)
	}
}
func TestImageUploadRejectsInvalidContent(t *testing.T) {
	writer := &recordingMediaWriter{}
	s := newMediaStore(writer)
	for _, tc := range []struct {
		data []byte
		kind string
	}{{[]byte("not a PNG"), "image/png"}, {pngFixture(t), "image/jpeg"}, {make([]byte, 10<<20+1), "image/png"}} {
		if _, err := s.Upload(context.Background(), bytes.NewReader(tc.data), tc.kind); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid accepted %v", err)
		}
	}
	if writer.key != "" {
		t.Fatal("invalid image reached storage")
	}
}
func TestImageUploadSurfacesStoreFailure(t *testing.T) {
	expected := errors.New("store unavailable")
	s := newMediaStore(&recordingMediaWriter{err: expected})
	if _, err := s.Upload(context.Background(), bytes.NewReader(pngFixture(t)), "image/png"); !errors.Is(err, expected) {
		t.Fatalf("error %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Upload(ctx, bytes.NewReader(pngFixture(t)), "image/png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
}
