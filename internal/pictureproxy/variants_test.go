package pictureproxy

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type variantStore struct {
	*fakeStore
	process string
	err     error
	body    string
}

func (s *variantStore) GetProcessed(_ context.Context, key, process string) (Object, error) {
	s.process = process
	if s.err != nil {
		return Object{}, s.err
	}
	body := s.body
	if body == "" {
		body = "small"
	}
	return Object{Metadata: Metadata{ContentType: "image/webp", ContentLength: -1, ETag: `"small"`}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestProcessedErrorsAndSizeLimitAreNotCached(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		body   string
		status int
	}{
		{"upstream", errors.New("private upstream detail"), "", 502},
		{"not found", ErrNotFound, "", 404},
		{"oversize", nil, strings.Repeat("a", 21), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &variantStore{fakeStore: newFakeStore(), err: tc.err, body: tc.body}
			r := httptest.NewRecorder()
			newTestHandler(t, s).ServeHTTP(r, httptest.NewRequest("GET", "/pic/photo.jpg?variant=thumb-v1", nil))
			if r.Code != tc.status || r.Header().Get("Cache-Control") != "no-store" || strings.Contains(r.Body.String(), "private upstream detail") {
				t.Fatalf("unexpected error response %d %v %s", r.Code, r.Header(), r.Body.String())
			}
		})
	}
}

func TestProcessedConditionalRequestAndBlogPrefix(t *testing.T) {
	s := &variantStore{fakeStore: newFakeStore()}
	key := "blog/status/photo.jpg"
	s.metadata[key] = s.metadata[stringKey]
	r := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/"+key+"?variant=thumb-v1", nil)
	req.Header.Set("If-None-Match", `"small"`)
	newTestHandler(t, s, WithBlogStatusPrefix()).ServeHTTP(r, req)
	if r.Code != 304 || r.Body.Len() != 0 {
		t.Fatalf("variant conditional response: %d %s", r.Code, r.Body.String())
	}
}

func TestJPEGAndPNGUseFixedProcessingProfiles(t *testing.T) {
	for _, kind := range []string{"image/jpeg", "image/png"} {
		for _, tc := range []struct{ variant, process string }{
			{"thumb-v1", "image/auto-orient,1/resize,m_lfit,w_640,h_640,limit_1/quality,q_80/format,webp"},
			{"large-v1", "image/auto-orient,1/resize,m_lfit,w_2048,h_2048,limit_1/quality,q_85/format,webp"},
		} {
			s := &variantStore{fakeStore: newFakeStore()}
			m := s.metadata[stringKey]
			m.ContentType = kind
			s.metadata[stringKey] = m
			r := httptest.NewRecorder()
			newTestHandler(t, s).ServeHTTP(r, httptest.NewRequest("GET", "/pic/photo.jpg?variant="+tc.variant, nil))
			if r.Code != 200 || s.process != tc.process {
				t.Fatalf("%s %s: %d %s", kind, tc.variant, r.Code, s.process)
			}
		}
	}
}

func TestImageVariantUsesProcessedMetadata(t *testing.T) {
	s := &variantStore{fakeStore: newFakeStore()}
	h := newTestHandler(t, s)
	r := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/pic/photo.jpg?variant=thumb-v1", nil)
	req.Header.Set("If-None-Match", `"photo-etag"`)
	h.ServeHTTP(r, req)
	if r.Code != 200 || r.Body.String() != "small" || r.Header().Get("Content-Type") != "image/webp" || r.Header().Get("Content-Length") != "5" || r.Header().Get("ETag") != `"small"` {
		t.Fatalf("wrong variant: %d %v %s", r.Code, r.Header(), r.Body.String())
	}
	if !strings.Contains(s.process, "resize") || s.getCalls != 0 {
		t.Fatal("original used instead of processing")
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest("HEAD", "/pic/photo.jpg?variant=large-v1", nil)
	h.ServeHTTP(r, req)
	if r.Code != 200 || r.Body.Len() != 0 || r.Header().Get("Content-Length") != "5" {
		t.Fatal("incorrect variant HEAD")
	}
}

func TestImageVariantRejectsUnboundedParameters(t *testing.T) {
	for _, query := range []string{"variant=huge", "variant=thumb-v1&variant=large-v1", "x-oss-process=image/resize,w_99999"} {
		s := newFakeStore()
		r := httptest.NewRecorder()
		newTestHandler(t, s).ServeHTTP(r, httptest.NewRequest("GET", "/pic/photo.jpg?"+query, nil))
		if r.Code != 400 || s.getCalls != 0 {
			t.Fatalf("unsafe query accepted: %s (%d)", query, r.Code)
		}
	}
}

func TestAnimatedFormatsRemainOriginal(t *testing.T) {
	for _, kind := range []string{"image/gif", "image/webp"} {
		s := &variantStore{fakeStore: newFakeStore()}
		m := s.metadata[stringKey]
		m.ContentType = kind
		s.metadata[stringKey] = m
		r := httptest.NewRecorder()
		newTestHandler(t, s).ServeHTTP(r, httptest.NewRequest("GET", "/pic/photo.jpg?variant=thumb-v1", nil))
		if r.Body.String() != "pic" || s.process != "" {
			t.Fatal("potential animation transformed")
		}
	}
}
