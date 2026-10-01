package blog

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const MaxImageBytes int64 = 10 << 20

type mediaWriter interface {
	Put(context.Context, string, io.Reader, string) error
}
type MediaStore struct {
	writer mediaWriter
	slots  chan struct{}
}

func newMediaStore(writer mediaWriter) *MediaStore {
	return &MediaStore{writer: writer, slots: make(chan struct{}, 2)}
}

// NewMediaStore 初始化专用 OSS 上传身份，不发起网络请求。
func NewMediaStore(endpoint, bucketName, keyID, keySecret string) (*MediaStore, error) {
	if endpoint == "" || bucketName == "" || keyID == "" || keySecret == "" {
		return nil, fmt.Errorf("博客图片 OSS 配置不完整")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	client, err := oss.New(endpoint, keyID, keySecret, oss.Timeout(10, 60))
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}
	return newMediaStore(ossMediaWriter{bucket}), nil
}

type ossMediaWriter struct{ bucket *oss.Bucket }

func (w ossMediaWriter) Put(ctx context.Context, key string, reader io.Reader, kind string) error {
	return w.bucket.PutObject(key, reader, oss.ContentType(kind), oss.WithContext(ctx))
}

// Upload 限流读取并验证图片，仅写入 blog/status/；调用方无需信任原文件名。
func (s *MediaStore) Upload(ctx context.Context, reader io.Reader, _ string) (Image, error) {
	if err := ctx.Err(); err != nil {
		return Image{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return Image{}, ctx.Err()
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxImageBytes+1))
	if err != nil {
		return Image{}, err
	}
	if int64(len(data)) > MaxImageBytes {
		return Image{}, fmt.Errorf("%w: 单张图片最大 10 MiB", ErrInvalidInput)
	}
	// 微信等客户端的文件名或 MIME 可能不准确，以图片实际内容决定格式。
	detected := http.DetectContentType(data)
	extension := ""
	switch detected {
	case "image/jpeg":
		extension = "jpg"
	case "image/png":
		extension = "png"
	case "image/webp":
		extension = "webp"
	case "image/gif":
		extension = "gif"
	default:
		return Image{}, fmt.Errorf("%w: 仅支持 JPEG、PNG、WebP、GIF", ErrInvalidInput)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return Image{}, fmt.Errorf("%w: 图片损坏或超过四千万像素", ErrInvalidInput)
	}
	key := "blog/status/" + time.Now().UTC().Format("2006/01/02/") + uuid.NewString() + "." + extension
	if err := s.writer.Put(ctx, key, bytes.NewReader(data), detected); err != nil {
		return Image{}, err
	}
	return Image{Key: key, URL: ImageBaseURL + key}, nil
}
