// Package blog 提供文件文章与数据库状态，两类内容各有唯一来源。
package blog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("内容不存在")

const LegacyStatusFile = "blog-2000-01-05.md"

type Heading struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
}
type Article struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Date        string    `json:"date"`
	Tags        []string  `json:"tags"`
	HTML        string    `json:"html,omitempty"`
	Description string    `json:"description"`
	TOC         []Heading `json:"toc"`
}
type ArticleStore struct{ directory string }

func NewArticleStore(directory string) *ArticleStore { return &ArticleStore{directory: directory} }

// root 固定本次读取的快照；current 可以是发布器维护的符号链接。
func (s *ArticleStore) root() (string, error) {
	if s.directory == "" {
		return "", fmt.Errorf("未配置博客文章目录")
	}
	return filepath.EvalSymlinks(s.directory)
}
func safeFile(root, relative string) (string, error) {
	if relative == "" || strings.Contains(relative, `\`) || filepath.IsAbs(relative) {
		return "", ErrNotFound
	}
	cleaned := filepath.Clean(relative)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrNotFound
	}
	current := root
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("不允许符号链接: %s", relative)
		}
	}
	return current, nil
}
func (s *ArticleStore) Get(slug string) (Article, error) {
	if slug == strings.TrimSuffix(LegacyStatusFile, ".md") {
		return Article{}, ErrNotFound
	}
	root, err := s.root()
	if err != nil {
		return Article{}, err
	}
	return readArticle(root, slug+".md")
}
func readArticle(root, relative string) (Article, error) {
	filename, err := safeFile(root, relative)
	if err != nil {
		return Article{}, err
	}
	info, err := os.Stat(filename)
	if err != nil {
		return Article{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return Article{}, fmt.Errorf("%s: 文章必须为不超过 4 MiB 的文件", relative)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return Article{}, err
	}
	article, err := parseArticle(data, root, relative)
	if err != nil {
		return Article{}, fmt.Errorf("%s: %w", relative, err)
	}
	article.Slug = filepath.ToSlash(strings.TrimSuffix(relative, ".md"))
	return article, nil
}
func (s *ArticleStore) List() ([]Article, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	articles := []Article{}
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("不允许符号链接: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			return fmt.Errorf("不允许隐藏文件: %s", rel)
		}
		if filepath.Ext(rel) != ".md" || rel == LegacyStatusFile {
			return nil
		}
		article, err := readArticle(root, rel)
		if err != nil {
			return err
		}
		articles = append(articles, article)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(articles, func(i, j int) bool {
		if articles[i].Date == articles[j].Date {
			return articles[i].Slug < articles[j].Slug
		}
		return articles[i].Date > articles[j].Date
	})
	return articles, nil
}
func (s *ArticleStore) Validate() error {
	articles, err := s.List()
	if err != nil {
		return err
	}
	if len(articles) == 0 {
		return fmt.Errorf("博客内容包没有文章")
	}
	return nil
}

// Asset 只允许文章目录内的图片附件，阻止下载原始 Markdown 和隐藏配置。
func (s *ArticleStore) Asset(relative string) (string, error) {
	ext := strings.ToLower(filepath.Ext(relative))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
	default:
		return "", ErrNotFound
	}
	root, err := s.root()
	if err != nil {
		return "", err
	}
	return safeFile(root, relative)
}
