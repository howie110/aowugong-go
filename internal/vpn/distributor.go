package vpn

import (
	"context"
	"strings"
)

// Distributor 定义用户订阅发布状态和公开根地址。
type Distributor interface {
	Configured() bool
	BaseURL() string
	Publish(context.Context, string, DistributionPayload) error
	Revoke(context.Context, string) error
}

// DirectDistributor 使用当前 Go 服务直接提供订阅，不复制节点到外部服务。
type DirectDistributor struct {
	baseURL string
}

// NewDirectDistributor 创建 Go 直连订阅分发器。
// 输入：baseURL 是用户设备可直接访问的 Aowugong 根地址。
// 输出：返回不会主动联网的分发器。
// 副作用：无。
func NewDirectDistributor(baseURL string) *DirectDistributor {
	// 1. 去掉末尾斜杠，确保生成地址稳定。
	return &DirectDistributor{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/")}
}

// Configured 判断公开订阅根地址是否已经配置。
// 输入：无。
// 输出：根地址非空时返回 true。
// 副作用：无。
func (d *DirectDistributor) Configured() bool {
	// 1. Go 直连分发只依赖公开根地址。
	return d.baseURL != ""
}

// BaseURL 返回用户设备访问的 Aowugong 根地址。
// 输入：无。
// 输出：返回不带末尾斜杠的根地址。
// 副作用：无。
func (d *DirectDistributor) BaseURL() string {
	// 1. 直接返回构造时已清理地址。
	return d.baseURL
}

// Publish 校验 Go 直连分发是否可用。
// 输入：ctx、tokenHash 和 payload 为统一分发接口参数。
// 输出：根地址已配置时返回 nil，否则返回 ErrDistributorNotConfigured。
// 副作用：无，不联网、不复制节点。
func (d *DirectDistributor) Publish(_ context.Context, _ string, _ DistributionPayload) error {
	// 1. 节点由公开请求实时读取，只需确认根地址存在。
	if !d.Configured() {
		return ErrDistributorNotConfigured
	}
	return nil
}

// Revoke 完成 Go 直连订阅的远端撤销步骤。
// 输入：ctx 和 tokenHash 为统一分发接口参数。
// 输出：始终返回 nil，数据库状态和 Token 校验会立即阻断旧地址。
// 副作用：无。
func (d *DirectDistributor) Revoke(_ context.Context, _ string) error {
	// 1. Go 直连模式没有远端副本需要删除。
	return nil
}
