package pictureproxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSStore 使用阿里云 OSS 官方 SDK 读取私有对象。
type OSSStore struct {
	bucket *oss.Bucket
}

// NewOSSStore 创建只读图片对象存储适配器。
// 输入：endpoint、bucket 和只读访问密钥；httpClient 可为空。
// 输出：返回可并发复用的 OSS 存储；参数无效时返回错误。
// 副作用：只初始化 SDK，不发起 OSS 网络请求。
func NewOSSStore(endpoint, bucketName, accessKeyID, accessKeySecret string, httpClient *http.Client) (*OSSStore, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" || bucketName == "" || accessKeyID == "" || accessKeySecret == "" {
		return nil, fmt.Errorf("OSS 图片存储配置不完整")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	options := make([]oss.ClientOption, 0, 1)
	if httpClient != nil {
		options = append(options, oss.HTTPClient(httpClient))
	}
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret, options...)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS 客户端: %w", err)
	}
	objectBucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS Bucket: %w", err)
	}
	return &OSSStore{bucket: objectBucket}, nil
}

// Head 读取对象元数据，不读取对象正文。
func (s *OSSStore) Head(ctx context.Context, key string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	headers, err := s.bucket.GetObjectMeta(key)
	if err != nil {
		return Metadata{}, normalizeOSSError(err)
	}
	return metadataFromHeaders(headers), nil
}

// Get 打开对象正文流；调用方负责关闭 Body。
func (s *OSSStore) Get(ctx context.Context, key string) (Object, error) {
	if err := ctx.Err(); err != nil {
		return Object{}, err
	}
	body, err := s.bucket.GetObject(key)
	if err != nil {
		return Object{}, normalizeOSSError(err)
	}
	return Object{Body: body}, nil
}

// metadataFromHeaders 把 OSS 响应头转换为代理元数据。
func metadataFromHeaders(headers http.Header) Metadata {
	metadata := Metadata{
		ContentType:   headerValue(headers, "Content-Type"),
		ContentLength: -1,
		ETag:          headerValue(headers, "ETag"),
	}
	if value := headerValue(headers, "Content-Length"); value != "" {
		if length, err := strconv.ParseInt(value, 10, 64); err == nil && length >= 0 {
			metadata.ContentLength = length
		}
	}
	if value := headerValue(headers, "Last-Modified"); value != "" {
		if lastModified, err := http.ParseTime(value); err == nil {
			metadata.LastModified = lastModified
		}
	}
	return metadata
}

// headerValue 兼容标准库和第三方 SDK 对响应头规范化方式的差异。
func headerValue(headers http.Header, name string) string {
	if value := headers.Get(name); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// normalizeOSSError 将 OSS 404 统一为代理层的 not found。
func normalizeOSSError(err error) error {
	var serviceError oss.ServiceError
	if errors.As(err, &serviceError) && serviceError.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	var serviceErrorPointer *oss.ServiceError
	if errors.As(err, &serviceErrorPointer) && serviceErrorPointer != nil && serviceErrorPointer.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	return err
}
