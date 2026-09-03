package pictureproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	metadata  map[string]Metadata
	bodies    map[string][]byte
	headErr   error
	getErr    error
	headCalls int
	getCalls  int
}

func (s *fakeStore) Head(_ context.Context, key string) (Metadata, error) {
	s.headCalls++
	if s.headErr != nil {
		return Metadata{}, s.headErr
	}
	metadata, ok := s.metadata[key]
	if !ok {
		return Metadata{}, ErrNotFound
	}
	return metadata, nil
}

func (s *fakeStore) Get(_ context.Context, key string) (Object, error) {
	s.getCalls++
	if s.getErr != nil {
		return Object{}, s.getErr
	}
	body, ok := s.bodies[key]
	if !ok {
		return Object{}, ErrNotFound
	}
	return Object{Metadata: s.metadata[key], Body: io.NopCloser(strings.NewReader(string(body)))}, nil
}

const stringKey = "pic/photo.jpg"

func newFakeStore() *fakeStore {
	return &fakeStore{
		metadata: map[string]Metadata{stringKey: {
			ContentType:   "image/jpeg",
			ContentLength: 3,
			ETag:          `"photo-etag"`,
			LastModified:  time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
		}},
		bodies: map[string][]byte{stringKey: []byte("pic")},
	}
}

func newTestHandler(t *testing.T, store Store, options ...Option) *Handler {
	t.Helper()
	baseOptions := []Option{
		WithStore(store),
		WithRateLimit(2, 10*time.Minute),
		WithMaxObjectBytes(20),
	}
	baseOptions = append(baseOptions, options...)
	handler, err := NewHandler(baseOptions...)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func TestHandlerServesGetAndHead(t *testing.T) {
	store := newFakeStore()
	handler := newTestHandler(t, store)

	getRecorder := httptest.NewRecorder()
	handler.ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, "/pic/photo.jpg", nil))
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", getRecorder.Code, http.StatusOK)
	}
	if getRecorder.Body.String() != "pic" {
		t.Errorf("GET body = %q, want pic", getRecorder.Body.String())
	}
	if getRecorder.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", getRecorder.Header().Get("Content-Type"))
	}
	if getRecorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", getRecorder.Header().Get("Cache-Control"))
	}
	if getRecorder.Header().Get("ETag") != `"photo-etag"` {
		t.Errorf("ETag = %q", getRecorder.Header().Get("ETag"))
	}

	headRecorder := httptest.NewRecorder()
	headRequest := httptest.NewRequest(http.MethodHead, "/pic/photo.jpg", nil)
	handler.ServeHTTP(headRecorder, headRequest)
	if headRecorder.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", headRecorder.Code, http.StatusOK)
	}
	if headRecorder.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", headRecorder.Body.Len())
	}
	if store.getCalls != 1 {
		t.Errorf("GET object calls after GET+HEAD = %d, want 1", store.getCalls)
	}
}

func TestHandlerReturnsNotModifiedForMatchingETag(t *testing.T) {
	store := newFakeStore()
	handler := newTestHandler(t, store)
	request := httptest.NewRequest(http.MethodGet, "/pic/photo.jpg", nil)
	request.Header.Set("If-None-Match", `W/"photo-etag"`)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotModified)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body length = %d, want 0", recorder.Body.Len())
	}
	if store.getCalls != 0 {
		t.Errorf("GET object calls = %d, want 0", store.getCalls)
	}
}

func TestHandlerRejectsUnsupportedMethodsAndUnsafePaths(t *testing.T) {
	handler := newTestHandler(t, newFakeStore())
	cases := []struct {
		name string
		path string
		want int
	}{
		{name: "post", path: "/pic/photo.jpg", want: http.StatusMethodNotAllowed},
		{name: "empty", path: "/", want: http.StatusNotFound},
		{name: "traversal", path: "/pic/../photo.jpg", want: http.StatusNotFound},
		{name: "wrong prefix", path: "/legacy/photo.jpg", want: http.StatusNotFound},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, testCase.path, nil))
			if testCase.name != "post" {
				recorder = httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.path, nil))
			}
			if recorder.Code != testCase.want {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.want)
			}
		})
	}
}

func TestHandlerRateLimitsEachForwardedIP(t *testing.T) {
	store := newFakeStore()
	handler := newTestHandler(t, store)

	for index := 0; index < 2; index++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodHead, "/pic/photo.jpg", nil)
		request.Header.Set("X-Forwarded-For", "203.0.113.10")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d", index+1, recorder.Code, http.StatusOK)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodHead, "/pic/photo.jpg", nil)
	request.Header.Set("X-Forwarded-For", "203.0.113.10")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Error("Retry-After is empty")
	}

	otherRecorder := httptest.NewRecorder()
	otherRequest := httptest.NewRequest(http.MethodHead, "/pic/photo.jpg", nil)
	otherRequest.Header.Set("X-Forwarded-For", "203.0.113.11")
	handler.ServeHTTP(otherRecorder, otherRequest)
	if otherRecorder.Code != http.StatusOK {
		t.Errorf("other IP status = %d, want %d", otherRecorder.Code, http.StatusOK)
	}
}

func TestHandlerMapsObjectErrorsAndSizeLimit(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		store := newFakeStore()
		store.headErr = ErrNotFound
		handler := newTestHandler(t, store)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pic/photo.jpg", nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})

	t.Run("upstream error", func(t *testing.T) {
		store := newFakeStore()
		store.headErr = errors.New("oss unavailable")
		handler := newTestHandler(t, store)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pic/photo.jpg", nil))
		if recorder.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
		}
	})

	t.Run("too large", func(t *testing.T) {
		store := newFakeStore()
		store.metadata[stringKey] = Metadata{ContentType: "image/jpeg", ContentLength: 21}
		handler := newTestHandler(t, store)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pic/photo.jpg", nil))
		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
		}
		if store.getCalls != 0 {
			t.Errorf("GET object calls = %d, want 0", store.getCalls)
		}
	})
}
