package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var notificationProjectPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var notificationTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{32,256}$`)

// parseNotificationTokens 读取项目到专用 Token 的映射；配置错误绝不回显密钥。
func parseNotificationTokens(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var tokens map[string]string
	if len(value) > 64*1024 || json.Unmarshal([]byte(value), &tokens) != nil || tokens == nil || len(tokens) > 100 {
		return nil, fmt.Errorf("AOWUGONG_NOTIFICATION_TOKENS 必须是最多 100 个项目的 JSON 对象")
	}
	seen := make(map[string]bool, len(tokens))
	for project, token := range tokens {
		if !notificationProjectPattern.MatchString(project) || !notificationTokenPattern.MatchString(token) || seen[token] {
			return nil, fmt.Errorf("通知项目标识无效、Token 不唯一或不符合 32–256 位字母数字下划线连字符要求")
		}
		seen[token] = true
	}
	return tokens, nil
}
