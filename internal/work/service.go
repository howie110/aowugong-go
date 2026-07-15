// Package work 提供私有工作导航文件读取和清洗。
package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Link 描述一个可展示的工作导航链接。
type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Host  string `json:"host"`
}

// Group 描述一组工作导航链接。
type Group struct {
	Title string `json:"title"`
	Links []Link `json:"links"`
}

// Navigation 描述工作导航页面完整数据。
type Navigation struct {
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Groups       []Group `json:"groups"`
	Total        int     `json:"total"`
	IsConfigured bool    `json:"is_configured"`
}

type rawNavigation struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Groups      []rawGroup `json:"groups"`
}

type rawGroup struct {
	Title string    `json:"title"`
	Links []rawLink `json:"links"`
}

type rawLink struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	WebTitle string `json:"web_title"`
	WebURL   string `json:"web_url"`
}

// Service 读取由部署环境维护的私有导航文件。
type Service struct {
	path string
}

// NewService 创建工作导航服务。
// 输入：path 是私有 navigation.json 路径。
// 输出：返回导航服务。
// 副作用：无。
func NewService(path string) *Service {
	// 1. 保存配置层提供的文件路径。
	return &Service{path: path}
}

// Navigation 读取并清洗私有工作导航。
// 输入：无。
// 输出：返回可供 React 页面使用的导航；文件不存在时返回空状态。
// 副作用：读取本地私有 JSON 文件。
func (s *Service) Navigation() (Navigation, error) {
	// 1. 文件未配置时返回稳定空页面数据。
	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyNavigation(), nil
	}
	if err != nil {
		return Navigation{}, fmt.Errorf("读取工作导航文件: %w", err)
	}

	// 2. 解码 JSON 并清洗有有效链接的分组。
	var raw rawNavigation
	if err := json.Unmarshal(content, &raw); err != nil {
		return Navigation{}, fmt.Errorf("解析工作导航文件: %w", err)
	}
	navigation := Navigation{
		Title:        strings.TrimSpace(raw.Title),
		Description:  strings.TrimSpace(raw.Description),
		Groups:       make([]Group, 0),
		IsConfigured: true,
	}
	if navigation.Title == "" {
		navigation.Title = "工作导航"
	}
	if navigation.Description == "" {
		navigation.Description = "常用系统、工具和资料入口。"
	}
	for _, sourceGroup := range raw.Groups {
		group := normalizeGroup(sourceGroup)
		if len(group.Links) == 0 {
			continue
		}
		navigation.Total += len(group.Links)
		navigation.Groups = append(navigation.Groups, group)
	}
	return navigation, nil
}

// emptyNavigation 构建未配置私有文件时的响应。
// 输入：无。
// 输出：返回空导航。
// 副作用：无。
func emptyNavigation() Navigation {
	// 1. 返回非 nil 空数组，方便前端直接遍历。
	return Navigation{
		Title:       "工作导航",
		Description: "常用系统、工具和资料入口。",
		Groups:      []Group{},
	}
}

// normalizeGroup 清洗单个导航分组。
// 输入：source 是原始 JSON 分组。
// 输出：返回仅包含有效链接的分组。
// 副作用：无。
func normalizeGroup(source rawGroup) Group {
	// 1. 清理标题并给空标题使用默认值。
	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = "未分组"
	}
	group := Group{Title: title, Links: make([]Link, 0)}

	// 2. 忽略缺少标题或地址的链接。
	for _, sourceLink := range source.Links {
		link, ok := normalizeLink(sourceLink)
		if ok {
			group.Links = append(group.Links, link)
		}
	}
	return group
}

// normalizeLink 清洗一个导航链接并提取 host。
// 输入：source 是原始 JSON 链接。
// 输出：有效时返回链接和 true。
// 副作用：无。
func normalizeLink(source rawLink) (Link, bool) {
	// 1. 兼容当前字段和旧 web_* 字段。
	title := strings.TrimSpace(source.Title)
	if title == "" {
		title = strings.TrimSpace(source.WebTitle)
	}
	address := strings.TrimSpace(source.URL)
	if address == "" {
		address = strings.TrimSpace(source.WebURL)
	}
	if title == "" || address == "" {
		return Link{}, false
	}

	// 2. 解析绝对 URL；无 scheme 的内部地址沿用路径作为 host。
	parsed, err := url.Parse(address)
	if err != nil {
		return Link{}, false
	}
	host := parsed.Host
	if host == "" {
		host = parsed.Path
	}
	return Link{Title: title, URL: address, Host: host}, true
}
