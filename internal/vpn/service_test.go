package vpn

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestV2rayQRCodePayloadPrefersSingleNode 验证手机二维码直接携带单节点分享链接。
// 输入：包含一个 Trojan 节点的 Base64 订阅和 HTTPS 回退地址。
// 输出：返回 Trojan 分享链接，不创建空的 v2rayNG 订阅组。
// 副作用：无。
func TestV2rayQRCodePayloadPrefersSingleNode(t *testing.T) {
	// 1. 编码不含真实凭据的测试节点并选择二维码内容。
	link := "trojan://test-password@test.example.com:443?security=tls#Test"
	body := base64.StdEncoding.EncodeToString([]byte(link))
	payload := v2rayQRCodePayload(body, "https://vpn.example.test/subscription")

	// 2. 单节点应原样进入二维码。
	if payload != link {
		t.Fatalf("v2rayQRCodePayload() = %q", payload)
	}
}

// TestV2rayQRCodePayloadKeepsSubscriptionForMultipleNodes 验证多节点二维码保留订阅地址。
// 输入：包含两个节点的 Base64 订阅和 HTTPS 回退地址。
// 输出：返回订阅地址，避免二维码静默遗漏节点。
// 副作用：无。
func TestV2rayQRCodePayloadKeepsSubscriptionForMultipleNodes(t *testing.T) {
	// 1. 编码两个测试节点并选择二维码内容。
	fallback := "https://vpn.example.test/subscription"
	body := base64.StdEncoding.EncodeToString([]byte("trojan://first@example.com:443\nvless://second@example.com:443"))

	// 2. 多节点无法由单个分享二维码完整表达，必须保留订阅地址。
	if payload := v2rayQRCodePayload(body, fallback); payload != fallback {
		t.Fatalf("v2rayQRCodePayload() = %q", payload)
	}
}

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

// TestServiceCreatesRotatesAndRevokesUserSubscription 验证用户订阅完整生命周期和 Token 轮换。
// 输入：隔离 SQLite、临时 Clash 配置和内存分发器。
// 输出：创建可订阅，轮换更换地址，撤销清空远端配置。
// 副作用：创建临时文件，读写 SQLite 和测试 Map。
func TestServiceCreatesRotatesAndRevokesUserSubscription(t *testing.T) {
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
	userID := createVPNTestUser(t, db, "android-user")

	// 2. 创建后取得 HTTPS 地址，数据库结构不包含明文 Token 字段。
	created, err := service.Create(context.Background(), CreateRequest{UserID: userID, ProfileCode: "demo"})
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
	rotated, err := service.Rotate(context.Background(), created.ID, userID, true)
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
	revoked, err := service.Revoke(context.Background(), created.ID, userID, true)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != StatusRevoked || len(revoked.Subscriptions) != 0 || len(distributor.values) != 0 {
		t.Fatalf("revoked device = %#v, remote count = %d", revoked, len(distributor.values))
	}
}

// TestServiceScopesSubscriptionsByLoginUser 验证普通用户只能读取自己的订阅。
// 输入：两个登录用户、同一测试资源和内存分发器。
// 输出：管理员看到全部记录，普通用户只看到自己且不能读取他人二维码。
// 副作用：创建临时文件，读写隔离 SQLite 和测试 Map。
func TestServiceScopesSubscriptionsByLoginUser(t *testing.T) {
	// 1. 准备两个用户和可发布的单节点资源。
	db := testdatabase.Open(t)
	directory := t.TempDir()
	content := "proxies:\n  - name: Test\n    type: vmess\n    server: test.example.com\n    port: 443\n    uuid: 00000000-0000-0000-0000-000000000001\n    alterId: 0\n    cipher: auto\n"
	if err := os.WriteFile(filepath.Join(directory, "clash_demo.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	distributor := &memoryDistributor{baseURL: "https://vpn.example.test", values: make(map[string]DistributionPayload)}
	service := NewService(NewRepository(db), NewSourceCatalog(directory), distributor, "test-secret")
	firstUserID := createVPNTestUser(t, db, "first-user")
	secondUserID := createVPNTestUser(t, db, "second-user")
	first, err := service.Create(context.Background(), CreateRequest{UserID: firstUserID, ProfileCode: "demo"})
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := service.Create(context.Background(), CreateRequest{UserID: secondUserID, ProfileCode: "demo"})
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	// 2. 普通用户摘要只包含自己的记录，管理员摘要包含全部记录和用户清单。
	viewerSummary, err := service.Summary(context.Background(), firstUserID, false)
	if err != nil {
		t.Fatalf("Summary(viewer) error = %v", err)
	}
	if len(viewerSummary.Subscriptions) != 1 || viewerSummary.Subscriptions[0].ID != first.ID || len(viewerSummary.Users) != 0 || viewerSummary.CanManage {
		t.Fatalf("viewer summary = %#v", viewerSummary)
	}
	adminSummary, err := service.Summary(context.Background(), firstUserID, true)
	if err != nil {
		t.Fatalf("Summary(admin) error = %v", err)
	}
	if len(adminSummary.Subscriptions) != 2 || len(adminSummary.Users) != 2 || !adminSummary.CanManage {
		t.Fatalf("admin summary = %#v", adminSummary)
	}

	// 3. 第一名用户不能读取第二名用户的二维码。
	if _, err := service.QRCode(context.Background(), second.ID, firstUserID, false, "v2ray"); err != ErrNotFound {
		t.Fatalf("QRCode(other user) error = %v, want ErrNotFound", err)
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
	userID := createVPNTestUser(t, db, "draft-user")

	// 2. 创建成功后保持草稿状态，不发布远端配置或返回不可用地址。
	created, err := service.Create(context.Background(), CreateRequest{UserID: userID, ProfileCode: "demo"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusDraft || len(created.Subscriptions) != 0 || len(distributor.values) != 0 {
		t.Fatalf("created device = %#v, remote count = %d", created, len(distributor.values))
	}

	// 3. 草稿无需分发器即可撤销，远端撤销调用保持为空。
	revoked, err := service.Revoke(context.Background(), created.ID, userID, true)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != StatusRevoked || len(distributor.revoked) != 0 {
		t.Fatalf("revoked device = %#v, remote revoke count = %d", revoked, len(distributor.revoked))
	}
}

// createVPNTestUser 创建 VPN 服务测试使用的活动登录用户。
// 输入：t 管理失败，db 是隔离 SQLite，username 是唯一用户名。
// 输出：返回新用户主键。
// 副作用：写入隔离 SQLite。
func createVPNTestUser(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, username string) int64 {
	// 1. 写入最小用户字段并返回主键。
	t.Helper()
	var userID int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO aowugong_fastapi_users (username, email, password, is_active)
		VALUES (?, ?, 'test-hash', 1)
		RETURNING id
	`, username, username+"@example.com").Scan(&userID)
	if err != nil {
		t.Fatalf("create VPN test user: %v", err)
	}
	return userID
}

// subscriptionToken 从测试订阅 URL 提取原始 Token。
// 输入：t 接收解析失败，rawURL 是用户订阅地址。
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
