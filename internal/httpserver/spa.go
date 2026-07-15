package httpserver

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type spaHandler struct {
	staticDir string
}

// newSPAHandler 创建静态资源与 SPA 回退处理器。
func newSPAHandler(staticDir string) spaHandler {
	// 1. 清理静态文件根目录。
	return spaHandler{staticDir: filepath.Clean(staticDir)}
}

// ServeHTTP 返回静态资源或 SPA 入口文件。
func (s spaHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	// 1. 拒绝未配置静态目录的前端请求。
	if s.staticDir == "." || s.staticDir == "" {
		writeError(w, http.StatusNotFound, "not_found", "static resource not found")
		return
	}

	// 2. 清理请求路径并确定目标文件。
	requestPath := path.Clean("/" + request.URL.Path)
	filePath := filepath.Join(s.staticDir, filepath.FromSlash(strings.TrimPrefix(requestPath, "/")))
	if path.Ext(requestPath) == "" {
		filePath = filepath.Join(s.staticDir, "index.html")
	}

	// 3. 仅服务存在的普通文件，缺失资源使用 JSON 错误信封。
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not_found", "static resource not found")
		return
	}
	http.ServeFile(w, request, filePath)
}
