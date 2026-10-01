package blog

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// PublishArchive 校验并解压上传包，事务入库后删除临时文件。
func (r *ArticleRepository) PublishArchive(ctx context.Context, archive string, sequence int64) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 64<<20 {
		return fmt.Errorf("压缩包超过 64 MiB")
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	staged, err := os.MkdirTemp("", "blog-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	reader := tar.NewReader(io.LimitReader(gz, 128<<20))
	var total int64
	for count := 0; ; count++ {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if count > 2000 {
			return fmt.Errorf("文件数量超限")
		}
		name := strings.TrimPrefix(header.Name, "./")
		name = strings.TrimSuffix(name, "/")
		if (name == "" || name == ".") && header.Typeflag == tar.TypeDir {
			continue
		}
		if name == "" || path.Clean(name) != name || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, `\`) {
			return fmt.Errorf("压缩包路径无效: %s", header.Name)
		}
		for _, component := range strings.Split(name, "/") {
			if strings.HasPrefix(component, ".") {
				return fmt.Errorf("不允许隐藏路径: %s", name)
			}
		}
		destination := filepath.Join(staged, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, 0750); err != nil {
				return err
			}
		case tar.TypeReg:
			total += header.Size
			if header.Size < 0 || header.Size > 10<<20 || total > 100<<20 {
				return fmt.Errorf("内容文件大小超限")
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0750); err != nil {
				return err
			}
			output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(output, reader, header.Size)
			closeErr := output.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("压缩包只允许普通文件和目录: %s", name)
		}
	}
	return r.PublishDirectory(ctx, staged, sequence)
}
