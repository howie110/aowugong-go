package blog

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Publish 在独占锁内校验、复制并激活不可变内容快照；失败不替换 current。
func Publish(source, root string, sequence int64) error {
	if sequence < 1 {
		return fmt.Errorf("发布序号必须大于零")
	}
	if err := os.MkdirAll(filepath.Join(root, "versions"), 0750); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(root, ".publish.lock"), os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	oldTarget, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if oldTarget != "" {
		old, err := strconv.ParseInt(filepath.Base(oldTarget), 10, 64)
		if err != nil || oldTarget != filepath.Join("versions", strconv.FormatInt(old, 10)) {
			return fmt.Errorf("current 不是合法发布目录")
		}
		if sequence <= old {
			return fmt.Errorf("拒绝旧发布序号 %d，当前为 %d", sequence, old)
		}
	}
	if err := NewArticleStore(source).Validate(); err != nil {
		return err
	}
	target := filepath.Join("versions", strconv.FormatInt(sequence, 10))
	destination := filepath.Join(root, target)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("发布目录已存在: %d", sequence)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(destination, 0750); err != nil {
		return err
	}
	activated := false
	defer func() {
		if !activated {
			_ = os.RemoveAll(destination)
		}
	}()
	if err := copyContent(source, destination); err != nil {
		return err
	}
	if err := NewArticleStore(destination).Validate(); err != nil {
		return err
	}
	if oldTarget != "" {
		if err := replaceLink(root, "previous", oldTarget); err != nil {
			return err
		}
	}
	if err := replaceLink(root, "current", target); err != nil {
		return err
	}
	activated = true
	// 仅清理本发布器创建的旧数字目录，保留当前与上一份快照。
	entries, err := os.ReadDir(filepath.Join(root, "versions"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		n, parseErr := strconv.ParseInt(entry.Name(), 10, 64)
		if parseErr != nil || n < 1 {
			continue
		}
		relative := filepath.Join("versions", entry.Name())
		if relative != target && relative != oldTarget {
			if err := os.RemoveAll(filepath.Join(root, relative)); err != nil {
				return fmt.Errorf("内容已生效，但清理旧快照失败: %w", err)
			}
		}
	}
	return nil
}
func replaceLink(root, name, target string) error {
	file, err := os.CreateTemp(root, ".link-")
	if err != nil {
		return err
	}
	temp := file.Name()
	file.Close()
	defer os.Remove(temp)
	if err := os.Remove(temp); err != nil {
		return err
	}
	if err := os.Symlink(target, temp); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(root, name))
}
func copyContent(source, destination string) error {
	return filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
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
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("不允许符号链接: %s", relative)
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return fmt.Errorf("不允许隐藏文件: %s", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		if relative == LegacyStatusFile {
			return nil
		}
		switch strings.ToLower(filepath.Ext(relative)) {
		case ".md", ".png", ".jpg", ".jpeg", ".gif", ".webp":
		default:
			return fmt.Errorf("不支持的内容文件: %s", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 10<<20 {
			return fmt.Errorf("文件过大或类型无效: %s", relative)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0640)
	})
}
