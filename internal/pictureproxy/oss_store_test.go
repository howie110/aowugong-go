package pictureproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type imageTransport func(*http.Request) (*http.Response, error)

func (f imageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOSSImageProcessingRequestAndMetadata(t *testing.T) {
	client := &http.Client{Transport: imageTransport(func(r *http.Request) (*http.Response, error) {
		header := http.Header{"Content-Length": []string{"3"}, "Etag": []string{`"original"`}}
		body := ""
		if r.Method == "HEAD" && !r.URL.Query().Has("objectMeta") {
			header.Set("Content-Type", "image/jpeg")
		}
		if r.Method == "GET" {
			if r.URL.Query().Get("x-oss-process") != "image/resize,w_640" {
				t.Error("missing signed process parameter")
			}
			header.Set("Content-Type", "image/webp")
			header.Set("Etag", `"processed"`)
			body = "new"
		}
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	store, err := NewOSSStore("https://oss.example.com", "pictures", "id", "secret", client)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Head(context.Background(), "pic/photo.jpg")
	if err != nil || meta.ContentType != "image/jpeg" {
		t.Fatalf("HEAD missing format required for processing: %#v %v", meta, err)
	}
	object, err := store.GetProcessed(context.Background(), "pic/photo.jpg", "image/resize,w_640")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	if object.ContentType != "image/webp" || object.ETag != `"processed"` {
		t.Fatalf("wrong processed metadata: %#v", object.Metadata)
	}
}

func TestMetadataFromHeaders(t *testing.T) {
	// 1. 构造 OSS 返回的基础对象头。
	headers := http.Header{
		"Content-Type":   []string{"image/webp"},
		"Content-Length": []string{"2048"},
		"ETag":           []string{`"etag-value"`},
		"Last-Modified":  []string{"Thu, 03 Sep 2026 12:00:00 GMT"},
	}

	// 2. 转换并验证代理使用的元数据。
	metadata := metadataFromHeaders(headers)
	if metadata.ContentType != "image/webp" || metadata.ContentLength != 2048 || metadata.ETag != `"etag-value"` {
		t.Errorf("metadata = %#v", metadata)
	}
	if metadata.LastModified.IsZero() {
		t.Error("LastModified is zero")
	}
}

func TestNormalizeOSSErrorMapsNotFoundOnly(t *testing.T) {
	// 1. OSS 404 应转换为代理层统一错误。
	notFound := normalizeOSSError(oss.ServiceError{StatusCode: http.StatusNotFound})
	if !errors.Is(notFound, ErrNotFound) {
		t.Errorf("not found error = %v, want ErrNotFound", notFound)
	}

	// 2. 其他供应商错误必须保留为上游错误，交由 HTTP 层返回 502。
	upstream := errors.New("storage unavailable")
	if got := normalizeOSSError(upstream); !errors.Is(got, upstream) {
		t.Errorf("upstream error = %v, want original error", got)
	}
}

func TestNewOSSStoreRejectsIncompleteConfiguration(t *testing.T) {
	// 1. 缺少任何一个必要参数时不应创建客户端。
	if _, err := NewOSSStore("oss-cn-guangzhou.aliyuncs.com", "", "id", "secret", nil); err == nil {
		t.Fatal("NewOSSStore() error = nil, want incomplete configuration error")
	}
}
