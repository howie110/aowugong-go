package pictureproxy

import (
	"errors"
	"net/http"
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

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
	if _, err := NewOSSStore("oss-cn-hangzhou.aliyuncs.com", "", "id", "secret", nil); err == nil {
		t.Fatal("NewOSSStore() error = nil, want incomplete configuration error")
	}
}
