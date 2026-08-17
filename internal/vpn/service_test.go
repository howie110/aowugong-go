package vpn

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type memoryDistributor struct {
	baseURL string
	values  map[string]DistributionPayload
	revoked []string
}

// Configured 返回测试分发器是否有根地址。
// 输入：无。
// 输出：有地址时返回 true。
// 副作用：无。
func (d *memoryDistributor) Configured() bool {
	// 1. 使用根地址模拟配置完整状态。
	return d.baseURL != ""
}

// BaseURL 返回测试订阅根地址。
// 输入：无。
// 输出：返回固定 HTTPS 地址。
// 副作用：无。
func (d *memoryDistributor) BaseURL() string {
	// 1. 返回构造时地址。
	return d.baseURL
}

// Publish 在内存中保存一台设备配置。
// 输入：ctx 未使用，tokenHash 是 KV 键，payload 是配置。
// 输出：总是成功。
// 副作用：写入测试 Map。
func (d *memoryDistributor) Publish(ctx context.Context, tokenHash string, payload DistributionPayload) error {
	// 1. 覆盖相同哈希，模拟 Worker KV PUT。
	d.values[tokenHash] = payload
	return nil
}

// Revoke 从内存删除一台设备配置。
// 输入：ctx 未使用，tokenHash 是 KV 键。
// 输出：总是成功。
// 副作用：删除测试 Map 并记录哈希。
func (d *memoryDistributor) Revoke(ctx context.Context, tokenHash string) error {
	// 1. 幂等删除并保留撤销审计。
	delete(d.values, tokenHash)
	d.revoked = append(d.revoked, tokenHash)
	return nil
}

// TestServiceCreatesRotatesAndRevokesIndependentDevice 验证设备订阅完整生命周期和 Token 轮换。
// 输入：隔离 SQLite、临时 Clash 配置和内存分发器。
// 输出：创建可订阅，轮换更换地址，撤销清空远端配置。
// 副作用：创建临时文件，读写 SQLite 和测试 Map。
func TestServiceCreatesRotatesAndRevokesIndependentDevice(t *testing.T) {
	// 1. 准备隔离仓储、资源和远端分发器。
	db := testdatabase.Open(t)
	directory := t.TempDir()
	content := `proxies:
  - name: Test
    type: vmess
    server: test.example.com
    port: 443
    uuid: 00000000-0000-0000-0000-000000000001
    alterId: 0
    cipher: auto
`
	if err := os.WriteFile(filepath.Join(directory, "clash_demo.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	distributor := &memoryDistributor{baseURL: "https://vpn.example.test", values: make(map[string]DistributionPayload)}
	service := NewService(NewRepository(db), NewSourceCatalog(directory), distributor, "test-secret")

	// 2. 创建后取得 HTTPS 地址，数据库结构不包含明文 Token 字段。
	created, err := service.Create(context.Background(), CreateRequest{Name: "Android", ProfileCode: "demo"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	oldURL := created.Subscriptions["v2ray"]
	if !strings.HasPrefix(oldURL, "https://vpn.example.test/api/v1/vpn/subscriptions/") || created.Status != StatusActive || len(distributor.values) != 1 {
		t.Fatalf("created device = %#v, remote count = %d", created, len(distributor.values))
	}
	rows, err := db.QueryContext(context.Background(), `PRAGMA table_info(vpn_subscription_device)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info error = %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&index, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if name == "token" || name == "token_hash" {
			t.Fatalf("database unexpectedly stores %s", name)
		}
	}

	// 3. 轮换后 URL 和远端哈希改变，旧哈希已撤销。
	rotated, err := service.Rotate(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	newURL := rotated.Subscriptions["v2ray"]
	if newURL == oldURL || rotated.TokenVersion != 2 || len(distributor.values) != 1 || len(distributor.revoked) != 1 {
		t.Fatalf("rotated device = %#v", rotated)
	}
	oldToken := subscriptionToken(t, oldURL)
	if distributor.revoked[0] != hashToken(oldToken) {
		t.Errorf("revoked hash does not match old token")
	}

	// 4. 撤销当前设备后远端配置清空且页面不再返回订阅地址。
	revoked, err := service.Revoke(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != StatusRevoked || len(revoked.Subscriptions) != 0 || len(distributor.values) != 0 {
		t.Fatalf("revoked device = %#v, remote count = %d", revoked, len(distributor.values))
	}
}

// TestServiceCreatesDraftWithoutDistributor 验证未配置公网分发时仍可先创建设备草稿。
// 输入：隔离 SQLite、临时 Clash 配置和未配置内存分发器。
// 输出：返回草稿设备且不生成订阅地址。
// 副作用：创建临时文件并写入 SQLite。
func TestServiceCreatesDraftWithoutDistributor(t *testing.T) {
	// 1. 准备可发现资源和没有根地址的分发器。
	db := testdatabase.Open(t)
	directory := t.TempDir()
	content := "proxies:\n  - name: Test\n    type: trojan\n    server: test.example.com\n    port: 443\n    password: test-password\n"
	if err := os.WriteFile(filepath.Join(directory, "clash_demo.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	distributor := &memoryDistributor{values: make(map[string]DistributionPayload)}
	service := NewService(NewRepository(db), NewSourceCatalog(directory), distributor, "test-secret")

	// 2. 创建成功后保持草稿状态，不发布远端配置或返回不可用地址。
	created, err := service.Create(context.Background(), CreateRequest{Name: "Android", ProfileCode: "demo"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusDraft || len(created.Subscriptions) != 0 || len(distributor.values) != 0 {
		t.Fatalf("created device = %#v, remote count = %d", created, len(distributor.values))
	}

	// 3. 草稿无需分发器即可撤销，远端撤销调用保持为空。
	revoked, err := service.Revoke(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != StatusRevoked || len(distributor.revoked) != 0 {
		t.Fatalf("revoked device = %#v, remote revoke count = %d", revoked, len(distributor.revoked))
	}
}

// subscriptionToken 从测试订阅 URL 提取原始 Token。
// 输入：t 接收解析失败，rawURL 是设备订阅地址。
// 输出：返回设备编号后的 Token。
// 副作用：无。
func subscriptionToken(t *testing.T, rawURL string) string {
	// 1. 解析固定 URL 路径并断言结构。
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 7 || parts[0] != "api" || parts[3] != "subscriptions" {
		t.Fatalf("subscription path = %q", parsed.Path)
	}
	return parts[5]
}
