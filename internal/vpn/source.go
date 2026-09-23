package vpn

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxSourceFileBytes = 2 * 1024 * 1024

const commonRoutingFilename = "common-routing.json"

var formatNames = map[string]string{
	"clash":        "Clash / FlClash",
	"v2ray":        "v2rayN / v2rayNG",
	"shadowrocket": "Shadowrocket",
	"surge":        "Surge",
}

var requiredFormats = []string{"clash", "shadowrocket", "surge", "v2ray"}

// SourceCatalog 从私有目录发现并转换 VPN 客户端配置。
type SourceCatalog struct {
	directory string
}

// NewSourceCatalog 创建 VPN 私有资源目录读取器。
// 输入：directory 是只包含私有 VPN 文件的目录。
// 输出：返回资源目录读取器。
// 副作用：无，不读取目录。
func NewSourceCatalog(directory string) *SourceCatalog {
	// 1. 保存清理后的目录路径，后续只读取其直接子文件。
	return &SourceCatalog{directory: filepath.Clean(directory)}
}

// CommonRouting 返回分配页和资源页共用的只读规则原文。
// 输入：无。
// 输出：返回固定文件名和原文；文件尚未配置时正文为空。
// 副作用：读取 VPN 私有规则文件。
func (c *SourceCatalog) CommonRouting() (ConfigContent, error) {
	// 1. 规则文件是目录中的固定文件，不参与资源归组。
	path := filepath.Join(c.directory, commonRoutingFilename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ConfigContent{
			ContentType: "application/json; charset=utf-8",
			Filename:    commonRoutingFilename,
		}, nil
	} else if err != nil {
		return ConfigContent{}, fmt.Errorf("检查公共 VPN 分流规则: %w", err)
	}
	content, err := readPrivateSource(path)
	if err != nil {
		return ConfigContent{}, err
	}
	return ConfigContent{
		ContentType: "application/json; charset=utf-8",
		Filename:    commonRoutingFilename,
		Body:        string(content),
	}, nil
}

func (c *SourceCatalog) loadCommonRouting() (commonRouting, error) {
	content, err := c.CommonRouting()
	if err != nil {
		return commonRouting{}, err
	}
	routing, err := parseCommonRouting([]byte(content.Body))
	if err != nil {
		return commonRouting{}, err
	}
	return routing, nil
}

// Profiles 返回当前目录可用的资源和客户端格式。
// 输入：无。
// 输出：返回按资源编码排序的列表；目录缺失时返回空列表。
// 副作用：读取私有目录文件名，不读取节点内容。
func (c *SourceCatalog) Profiles() ([]Profile, error) {
	// 1. 收集受支持文件，并把同一资源的多种客户端格式归组。
	files, err := c.sourceFiles()
	if err != nil {
		return nil, err
	}
	grouped := make(map[string]struct{})
	for _, file := range files {
		_, profileCode, ok := parseSourceFilename(file.Name())
		if !ok {
			continue
		}
		grouped[profileCode] = struct{}{}
	}

	// 2. 稳定排序资源和格式，保证页面不会因目录遍历顺序跳动。
	profiles := make([]Profile, 0, len(grouped))
	for code := range grouped {
		formatList := make([]Format, 0, len(requiredFormats))
		for _, formatCode := range requiredFormats {
			formatList = append(formatList, Format{Code: formatCode, Name: formatNames[formatCode]})
		}
		profiles = append(profiles, Profile{Code: code, Name: profileDisplayName(code), Formats: formatList})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Code < profiles[j].Code })
	return profiles, nil
}

// sourceFiles 返回私有目录中的普通文件。
// 输入：无。
// 输出：目录缺失时返回空列表，其他读取错误带上下文返回。
// 副作用：读取私有目录元数据。
func (c *SourceCatalog) sourceFiles() ([]os.DirEntry, error) {
	// 1. 将尚未配置私有目录视为没有资源，而不是应用启动错误。
	files, err := os.ReadDir(c.directory)
	if os.IsNotExist(err) {
		return []os.DirEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 VPN 私有资源目录: %w", err)
	}
	result := make([]os.DirEntry, 0, len(files))
	for _, file := range files {
		if file.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, infoErr := file.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("读取 VPN 私有文件信息: %w", infoErr)
		}
		if info.Mode().IsRegular() {
			result = append(result, file)
		}
	}
	return result, nil
}

// readPrivateSource 读取大小受限的单个私有配置文件。
// 输入：path 是 SourceCatalog 已匹配的直接子文件。
// 输出：返回完整内容；文件过大或读取失败时返回错误。
// 副作用：读取私有 VPN 文件。
func readPrivateSource(path string) ([]byte, error) {
	// 1. 先检查大小，避免误把大文件推送到远端 KV。
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("检查 VPN 私有配置: %w", err)
	}
	if info.Size() > maxSourceFileBytes {
		return nil, fmt.Errorf("VPN 私有配置超过 %d 字节", maxSourceFileBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 VPN 私有配置: %w", err)
	}
	return content, nil
}

// parseSourceFilename 解析受支持的私有配置文件名。
// 输入：name 是不含目录的文件名。
// 输出：返回来源类型、资源编码和匹配标记。
// 副作用：无。
func parseSourceFilename(name string) (string, string, bool) {
	// 1. 按第一个下划线分开客户端类型和资源编码。
	lowerName := strings.ToLower(strings.TrimSpace(name))
	extension := filepath.Ext(lowerName)
	base := strings.TrimSuffix(lowerName, extension)
	separator := strings.IndexByte(base, '_')
	if separator <= 0 || separator == len(base)-1 {
		return "", "", false
	}
	kind, profileCode := base[:separator], base[separator+1:]
	validKind := kind == "clash" || kind == "flclash" || kind == "v2rayn" || kind == "shadowrocket" || kind == "surge"
	if !validKind || !validProfileCode(profileCode) {
		return "", "", false
	}
	return kind, profileCode, true
}

// formatsForSourceKind 返回一个源文件可以提供的订阅格式。
// 输入：kind 是文件名前缀。
// 输出：返回稳定格式编码列表。
// 副作用：无。
func formatsForSourceKind(kind string) []string {
	// 1. Clash 和 Xray 可以转换为通用 v2ray 链接，其余保持原生格式。
	switch kind {
	case "clash", "flclash":
		return []string{"clash", "v2ray"}
	case "v2rayn":
		return []string{"v2ray"}
	case "shadowrocket":
		return []string{"shadowrocket"}
	case "surge":
		return []string{"surge"}
	default:
		return nil
	}
}

// validProfileCode 判断资源编码能否安全用于文件匹配和 URL 展示。
// 输入：value 是待校验编码。
// 输出：仅小写字母、数字、短横线和下划线组成时返回 true。
// 副作用：无。
func validProfileCode(value string) bool {
	// 1. 拒绝空值、过长值和任何路径字符。
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

// profileDisplayName 把资源编码转换为页面显示名。
// 输入：code 是资源编码。
// 输出：返回用户可识别的资源显示名。
// 副作用：无。
func profileDisplayName(code string) string {
	// 1. 对已约定的中文供应方保留用户熟悉的名称，其余简称统一大写便于识别。
	if code == "mojie" {
		return "魔戒"
	}
	return strings.ToUpper(strings.ReplaceAll(code, "_", " "))
}
