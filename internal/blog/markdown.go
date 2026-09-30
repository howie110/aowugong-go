package blog

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

var articlePolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("id").Matching(regexp.MustCompile(`^section-[0-9]+$`)).OnElements("h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^language-[a-zA-Z0-9_-]+$`)).OnElements("code")
	return p
}()

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, nil, fmt.Errorf("缺少 YAML 元信息")
	}
	end := bytes.Index(data[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, nil, fmt.Errorf("元信息未结束")
	}
	return data[4 : 4+end], data[4+end+5:], nil
}
func parseArticle(data []byte, root, filename string) (Article, error) {
	meta, body, err := splitFrontmatter(data)
	if err != nil {
		return Article{}, err
	}
	var fm struct {
		Title string    `yaml:"title"`
		Date  time.Time `yaml:"date"`
		Tags  []string  `yaml:"tags"`
		Image string    `yaml:"image"`
	}
	if err := yaml.Unmarshal(meta, &fm); err != nil {
		return Article{}, fmt.Errorf("无效元信息: %w", err)
	}
	if strings.TrimSpace(fm.Title) == "" {
		return Article{}, fmt.Errorf("缺少标题")
	}
	date := fm.Date
	if date.IsZero() {
		return Article{}, fmt.Errorf("日期应为 YYYY-MM-DD")
	}
	article := Article{Title: fm.Title, Date: date.Format("2006-01-02"), Tags: []string{}, TOC: []Heading{}}
	for _, tag := range fm.Tags {
		if tag != "博客" && strings.TrimSpace(tag) != "" {
			article.Tags = append(article.Tags, tag)
		}
	}
	md := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithHardWraps()))
	doc := md.Parser().Parse(text.NewReader(body))
	err = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Heading:
			id := fmt.Sprintf("section-%d", len(article.TOC)+1)
			n.SetAttributeString("id", []byte(id))
			article.TOC = append(article.TOC, Heading{ID: id, Text: string(n.Text(body)), Level: n.Level})
		case *ast.Image:
			dest, err := resolveReference(string(n.Destination), root, filename, true)
			if err != nil {
				return ast.WalkStop, err
			}
			n.Destination = []byte(dest)
		case *ast.Link:
			dest, err := resolveReference(string(n.Destination), root, filename, false)
			if err != nil {
				return ast.WalkStop, err
			}
			n.Destination = []byte(dest)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Article{}, err
	}
	var out bytes.Buffer
	if err := md.Renderer().Render(&out, body, doc); err != nil {
		return Article{}, err
	}
	article.HTML = articlePolicy.Sanitize(out.String())
	plain := bluemonday.StrictPolicy().Sanitize(article.HTML)
	runes := []rune(strings.Join(strings.Fields(plain), " "))
	if len(runes) > 160 {
		runes = runes[:160]
	}
	article.Description = string(runes)
	return article, nil
}
func resolveReference(raw, root, filename string, image bool) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("链接无效: %s", raw)
	}
	if u.IsAbs() || u.Host != "" || strings.HasPrefix(raw, "#") || raw == "" {
		return raw, nil
	}
	if strings.HasPrefix(u.Path, "/") {
		if strings.HasPrefix(u.Path, "/posts/") {
			u.Path = "/blog" + u.Path
			return u.String(), nil
		}
		if image {
			return "", fmt.Errorf("图片需要 OSS URL 或博客目录内相对路径: %s", raw)
		}
		return raw, nil
	}
	rel := path.Clean(path.Join(path.Dir(filepath.ToSlash(filename)), u.Path))
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, `\`) {
		return "", fmt.Errorf("链接超出博客目录: %s", raw)
	}
	if _, err := safeFile(root, filepath.FromSlash(rel)); err != nil {
		return "", fmt.Errorf("附件不存在或不安全 %s: %w", raw, err)
	}
	if strings.HasSuffix(rel, ".md") && !image {
		u.Path = "/blog/posts/" + strings.TrimSuffix(rel, ".md")
	} else {
		ext := strings.ToLower(path.Ext(rel))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		default:
			return "", fmt.Errorf("不支持的本地附件: %s", raw)
		}
		u.Path = "/blog/assets/" + rel
	}
	return u.String(), nil
}
