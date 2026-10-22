package pictureproxy

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// 固定且带版本的规格限制处理费用；改变参数需升级版本，避免长期缓存混用。
func imageProcess(query url.Values) (string, bool) {
	if query.Has("x-oss-process") {
		return "", false
	}
	values, exists := query["variant"]
	if !exists {
		return "", true
	}
	if len(values) != 1 {
		return "", false
	}
	switch values[0] {
	case "thumb-v1":
		return "image/auto-orient,1/resize,m_lfit,w_640,h_640,limit_1/quality,q_80/format,webp", true
	case "large-v1":
		return "image/auto-orient,1/resize,m_lfit,w_2048,h_2048,limit_1/quality,q_85/format,webp", true
	default:
		return "", false
	}
}

type processedStore interface {
	GetProcessed(context.Context, string, string) (Object, error)
}

// serveProcessed 使用处理结果的长度和 ETag，不复用原图元数据。
func (h *Handler) serveProcessed(w http.ResponseWriter, r *http.Request, key, process string) {
	store, ok := h.store.(processedStore)
	if !ok {
		writeError(w, http.StatusBadGateway, "image processing unavailable")
		return
	}
	object, err := store.GetProcessed(r.Context(), key, process)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	if object.Body == nil {
		writeError(w, http.StatusBadGateway, "empty processed image")
		return
	}
	defer object.Body.Close()
	data, err := io.ReadAll(io.LimitReader(object.Body, h.maxObjectBytes+1))
	if err != nil {
		writeError(w, http.StatusBadGateway, "read processed image failed")
		return
	}
	if int64(len(data)) > h.maxObjectBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "image is too large")
		return
	}
	object.ContentLength = int64(len(data))
	if object.ContentType == "" {
		object.ContentType = http.DetectContentType(data)
	}
	writeCacheHeaders(w, object.Metadata)
	if etagMatches(r.Header.Get("If-None-Match"), object.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}
