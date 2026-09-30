package blog

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ParseLegacyStatuses 只解析一次性旧动态格式，不参与日常状态渲染。
func ParseLegacyStatuses(source []byte) ([]StatusInput, error) {
	_, body, err := splitFrontmatter(source)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	result := []StatusInput{}
	buffer := []string{}
	seen := map[string]bool{}
	finish := func() error {
		raw := strings.TrimSpace(strings.Join(buffer, "\n"))
		if raw == "" {
			buffer = nil
			return nil
		}
		lines := strings.Split(raw, "\n")
		stamp := strings.TrimSpace(lines[len(lines)-1])
		published, err := time.ParseInLocation("2006-01-02 15:04:05", stamp, location)
		if err != nil {
			return fmt.Errorf("第 %d 条动态缺少明确的末尾时间", len(result)+1)
		}
		markdown := strings.TrimSpace(strings.Join(lines[:len(lines)-1], "\n"))
		plain, images, err := legacyPlainText([]byte(markdown))
		if err != nil {
			return fmt.Errorf("第 %d 条动态: %w", len(result)+1, err)
		}
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("aowugong-legacy-status\n"+stamp+"\n"+markdown)).String()
		if seen[id] {
			return fmt.Errorf("第 %d 条动态与前文完全重复，需人工核对", len(result)+1)
		}
		seen[id] = true
		result = append(result, StatusInput{ID: id, Body: plain, Images: images, Published: true, PublishedAt: &published, Create: true})
		buffer = nil
		return nil
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "---" {
			chunk := strings.TrimSpace(strings.Join(buffer, "\n"))
			if chunk == "" {
				buffer = nil
				continue
			}
			lines := strings.Split(chunk, "\n")
			if _, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(lines[len(lines)-1]), location); err == nil {
				if err := finish(); err != nil {
					return nil, err
				}
				continue
			}
		}
		buffer = append(buffer, line)
	}
	if err := finish(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可导入的历史动态")
	}
	return result, nil
}
func legacyPlainText(source []byte) (string, []Image, error) {
	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(source))
	var out bytes.Buffer
	images := []Image{}
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n := node.(type) {
		case *ast.Image:
			if !entering {
				return ast.WalkContinue, nil
			}
			u, err := url.Parse(string(n.Destination))
			if err != nil || u.Scheme != "https" || u.Host != "pic.aowugong.top" || u.RawQuery != "" || u.Fragment != "" {
				return ast.WalkStop, fmt.Errorf("旧图片不是受信任的图片地址")
			}
			image := Image{Key: strings.TrimPrefix(u.Path, "/"), URL: u.String()}
			if err := validateImages([]Image{image}, true); err != nil {
				return ast.WalkStop, err
			}
			images = append(images, image)
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			if entering {
				out.Write(n.Segment.Value(source))
				if n.SoftLineBreak() || n.HardLineBreak() {
					out.WriteByte('\n')
				}
			}
		case *ast.String:
			if entering {
				out.Write(n.Value)
			}
		case *ast.Link:
			if !entering {
				out.WriteString("（" + string(n.Destination) + "）")
			}
		case *ast.AutoLink:
			if entering {
				out.Write(n.URL(source))
				return ast.WalkSkipChildren, nil
			}
		case *ast.FencedCodeBlock:
			if entering {
				for i := 0; i < n.Lines().Len(); i++ {
					line := n.Lines().At(i)
					out.Write(line.Value(source))
				}
				out.WriteString("\n\n")
				return ast.WalkSkipChildren, nil
			}
		case *ast.CodeBlock:
			if entering {
				for i := 0; i < n.Lines().Len(); i++ {
					line := n.Lines().At(i)
					out.Write(line.Value(source))
				}
				out.WriteString("\n\n")
				return ast.WalkSkipChildren, nil
			}
		case *ast.Paragraph, *ast.Heading, *ast.ListItem:
			if !entering {
				out.WriteString("\n\n")
			}
		case *ast.ThematicBreak:
			if entering {
				out.WriteString("\n---\n")
			}
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkStop, fmt.Errorf("历史动态含 HTML，需要核对转换")
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(out.String()), images, err
}

// ImportLegacy 单事务导入并保持稳定 ID，任一条有冲突则整批回滚。
func (s *StatusService) ImportLegacy(ctx context.Context, actorID int64, rows []StatusInput) error {
	tx, err := s.repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := NewStatusService(&Repository{queries: tx})
	for _, row := range rows {
		row.Create = true
		if _, err := scoped.save(ctx, actorID, row, true); err != nil {
			return err
		}
	}
	return tx.Commit()
}
