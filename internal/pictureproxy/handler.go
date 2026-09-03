// Package pictureproxy 提供私有 OSS 图片的受限读取入口。
package pictureproxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotFound 表示 OSS 中不存在请求的对象。
var ErrNotFound = errors.New("picture object not found")

// Metadata 描述图片对象的 HTTP 元数据。
type Metadata struct {
	ContentType   string
	ContentLength int64
	ETag          string
	LastModified  time.Time
}

// Object 是从对象存储读取到的对象流。
type Object struct {
	Metadata
	Body io.ReadCloser
}

// Store 是图片代理需要的最小对象存储接口。
type Store interface {
	Head(ctx context.Context, key string) (Metadata, error)
	Get(ctx context.Context, key string) (Object, error)
}

type handlerOptions struct {
	store             Store
	requestsPerWindow int
	window            time.Duration
	maxObjectBytes    int64
	maxConcurrent     int
}

// Option 配置图片代理。
type Option func(*handlerOptions)

// WithStore 设置对象存储实现。
func WithStore(store Store) Option {
	return func(options *handlerOptions) { options.store = store }
}

// WithRateLimit 设置单客户端固定窗口请求限制。
func WithRateLimit(requests int, window time.Duration) Option {
	return func(options *handlerOptions) {
		options.requestsPerWindow = requests
		options.window = window
	}
}

// WithMaxObjectBytes 设置单个对象允许读取的最大字节数。
func WithMaxObjectBytes(maxBytes int64) Option {
	return func(options *handlerOptions) { options.maxObjectBytes = maxBytes }
}

// WithMaxConcurrent 设置同时读取 OSS 对象的最大数量。
func WithMaxConcurrent(maxConcurrent int) Option {
	return func(options *handlerOptions) { options.maxConcurrent = maxConcurrent }
}

// Handler 是私有 OSS 图片 HTTP 处理器。
type Handler struct {
	store          Store
	limiter        *rateLimiter
	maxObjectBytes int64
	semaphore      chan struct{}
}

// NewHandler 创建图片代理并校验所有限制参数。
func NewHandler(options ...Option) (*Handler, error) {
	settings := handlerOptions{
		requestsPerWindow: 600,
		window:            10 * time.Minute,
		maxObjectBytes:    20 << 20,
		maxConcurrent:     8,
	}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	if settings.store == nil {
		return nil, fmt.Errorf("图片代理必须配置对象存储")
	}
	if settings.requestsPerWindow < 1 || settings.window <= 0 {
		return nil, fmt.Errorf("图片代理频率限制必须大于零")
	}
	if settings.maxObjectBytes < 1 {
		return nil, fmt.Errorf("图片代理对象大小限制必须大于零")
	}
	if settings.maxConcurrent < 1 {
		return nil, fmt.Errorf("图片代理并发限制必须大于零")
	}
	return &Handler{
		store:          settings.store,
		limiter:        newRateLimiter(settings.requestsPerWindow, settings.window),
		maxObjectBytes: settings.maxObjectBytes,
		semaphore:      make(chan struct{}, settings.maxConcurrent),
	}, nil
}

// ServeHTTP 读取允许前缀下的图片并返回带长期缓存的响应。
func (h *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	// 1. 图片入口只提供读取方法，避免被误用为通用对象存储代理。
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 2. 将 URL 限制到图片前缀，拒绝路径穿越和未知对象类型。
	key, ok := objectKey(request.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}

	// 3. 先按客户端 IP 限流，再访问 OSS，避免无效请求消耗对象存储额度。
	allowed, retryAfter := h.limiter.Allow(clientIP(request), time.Now())
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(maxInt(1, int(retryAfter.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	if !h.acquire(request.Context()) {
		writeError(w, http.StatusServiceUnavailable, "image service busy")
		return
	}
	defer h.release()

	// 4. 先取元数据，既支持 HEAD/ETag，也能在下载前拦截过大对象。
	metadata, err := h.store.Head(request.Context(), key)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	if metadata.ContentLength > h.maxObjectBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "image is too large")
		return
	}
	if etagMatches(request.Header.Get("If-None-Match"), metadata.ETag) {
		writeCacheHeaders(w, metadata)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if request.Method == http.MethodHead {
		writeCacheHeaders(w, metadata)
		if metadata.ContentLength >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(metadata.ContentLength, 10))
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// 5. 下载对象时继续执行大小保护；正常 OSS 元数据都会提供长度。
	object, err := h.store.Get(request.Context(), key)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	if object.Body == nil {
		writeError(w, http.StatusBadGateway, "image storage returned an empty body")
		return
	}
	defer object.Body.Close()
	if metadata.ContentLength < 0 {
		h.serveUnknownLength(w, object.Body, metadata)
		return
	}
	reader := bufio.NewReader(object.Body)
	if metadata.ContentType == "" {
		metadata.ContentType = detectContentType(reader)
	}
	writeCacheHeaders(w, metadata)
	w.Header().Set("Content-Length", strconv.FormatInt(metadata.ContentLength, 10))
	w.WriteHeader(http.StatusOK)
	if _, err := io.CopyN(w, reader, metadata.ContentLength); err != nil {
		// 响应头已经发出，无法再改成 502；连接会由 net/http 正常结束。
		return
	}
}

// acquire 在服务关闭或并发已满时停止等待。
func (h *Handler) acquire(ctx context.Context) bool {
	select {
	case h.semaphore <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// release 释放一个 OSS 读取槽位。
func (h *Handler) release() { <-h.semaphore }

// serveUnknownLength 为没有 Content-Length 的测试存储或兼容存储保留大小保护。
func (h *Handler) serveUnknownLength(w http.ResponseWriter, body io.Reader, metadata Metadata) {
	data, err := io.ReadAll(io.LimitReader(body, h.maxObjectBytes+1))
	if err != nil {
		writeError(w, http.StatusBadGateway, "read image failed")
		return
	}
	if int64(len(data)) > h.maxObjectBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "image is too large")
		return
	}
	metadata.ContentLength = int64(len(data))
	if metadata.ContentType == "" {
		metadata.ContentType = http.DetectContentType(data)
	}
	writeCacheHeaders(w, metadata)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// objectKey 校验并返回允许代理的对象键。
func objectKey(requestPath string) (string, bool) {
	key := strings.TrimPrefix(requestPath, "/")
	if key == "" || strings.ContainsRune(key, '\x00') || strings.ContainsRune(key, '\\') {
		return "", false
	}
	if !strings.HasPrefix(key, "images/") && !strings.HasPrefix(key, "legacy/") {
		return "", false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	return key, true
}

// clientIP 读取 Caddy 覆盖后的转发地址，回退到 TCP 对端地址。
func clientIP(request *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		for _, value := range strings.Split(request.Header.Get(header), ",") {
			if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
				return ip.String()
			}
		}
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr)); err == nil {
		return host
	}
	if request.RemoteAddr != "" {
		return request.RemoteAddr
	}
	return "unknown"
}

// writeCacheHeaders 写入成功对象共用的安全与缓存头。
func writeCacheHeaders(w http.ResponseWriter, metadata Metadata) {
	if metadata.ContentType != "" {
		w.Header().Set("Content-Type", metadata.ContentType)
	}
	if metadata.ETag != "" {
		w.Header().Set("ETag", metadata.ETag)
	}
	if !metadata.LastModified.IsZero() {
		w.Header().Set("Last-Modified", metadata.LastModified.UTC().Format(http.TimeFormat))
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// etagMatches 支持 If-None-Match 的弱标签和多值形式。
func etagMatches(header, etag string) bool {
	if strings.TrimSpace(header) == "*" || strings.TrimSpace(etag) == "" {
		return strings.TrimSpace(header) == "*" && strings.TrimSpace(etag) != ""
	}
	normalize := func(value string) string { return strings.TrimPrefix(strings.TrimSpace(value), "W/") }
	want := normalize(etag)
	for _, candidate := range strings.Split(header, ",") {
		if normalize(candidate) == want {
			return true
		}
	}
	return false
}

// detectContentType 从对象头部推断缺失的 MIME 类型。

func detectContentType(reader *bufio.Reader) string {
	if sample, err := reader.Peek(512); len(sample) > 0 || err != nil {
		return http.DetectContentType(sample)
	}
	return "application/octet-stream"
}

// handleStoreError 把对象存储错误转换成不泄露供应商信息的 HTTP 错误。
func handleStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	writeError(w, http.StatusBadGateway, "image storage unavailable")
}

// writeError 写入不缓存的简短错误响应。
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, message+"\n")
}

// maxInt 返回两个整数中的较大值。
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]clientWindow
}

type clientWindow struct {
	started time.Time
	count   int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, clients: make(map[string]clientWindow)}
}

// Allow 判断客户端是否还能在当前固定窗口内请求。
func (l *rateLimiter) Allow(client string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, state := range l.clients {
		if now.Sub(state.started) >= l.window || now.Before(state.started) {
			delete(l.clients, key)
		}
	}
	state, exists := l.clients[client]
	if !exists {
		l.clients[client] = clientWindow{started: now, count: 1}
		return true, 0
	}
	if state.count >= l.limit {
		return false, l.window - now.Sub(state.started)
	}
	state.count++
	l.clients[client] = state
	return true, 0
}
