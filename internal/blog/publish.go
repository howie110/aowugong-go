package blog

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PublishDirectory 先解析完整输入，再以数据库事务发布；不保留文件快照。
func (r *ArticleRepository) PublishDirectory(ctx context.Context, source string, sequence int64) error {
	articles, err := NewArticleSource(source).List()
	if err != nil {
		return err
	}
	assets := []ArticleAsset{}
	var total int64
	count := 0
	err = filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".") {
			return fmt.Errorf("不允许符号链接或隐藏文件: %s", relative)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		count++
		if !info.Mode().IsRegular() || info.Size() > 10<<20 || total > 100<<20 || count > 2000 {
			return fmt.Errorf("内容文件类型或大小超限: %s", relative)
		}
		extension := strings.ToLower(filepath.Ext(relative))
		if extension == ".md" {
			if filepath.Ext(relative) != ".md" {
				return fmt.Errorf("Markdown 扩展名必须为 .md: %s", relative)
			}
			return nil
		}
		switch extension {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			return fmt.Errorf("不支持的附件: %s", relative)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		kind := http.DetectContentType(data)
		switch kind {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return fmt.Errorf("附件不是有效图片: %s", relative)
		}
		assets = append(assets, ArticleAsset{Path: filepath.ToSlash(relative), Content: data, ContentType: kind})
		return nil
	})
	if err != nil {
		return err
	}
	return r.replace(ctx, articles, assets, sequence)
}
