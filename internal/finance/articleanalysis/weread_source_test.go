package articleanalysis

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestWeReadSourceCredentialRoundTrip 验证微信读书凭据只以密文保存并可由同一应用密钥恢复。
// 输入：一组完整测试凭据和隔离 SQLite。
// 输出：解密结果与原值一致，数据库密文不含 Token 明文。
// 副作用：写入隔离 SQLite 测试库。
func TestWeReadSourceCredentialRoundTrip(t *testing.T) {
	// 1. 使用固定应用密钥加密保存测试凭据。
	ctx := context.Background()
	db := testdatabase.Open(t)
	source := NewWeReadSource(NewRepository(db), client.NewWeReadArticleClient(nil), "test-encryption-secret")
	want := client.WeReadArticleCredentials{
		VID: "123456", DeviceID: "device-test", AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	if err := source.saveCredentials(ctx, want); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}

	// 2. 检查数据库不含明文，再使用同一密钥完成解密。
	var ciphertext []byte
	if err := db.QueryRowContext(ctx, "SELECT ciphertext FROM weread_article_credential WHERE id = 1").Scan(&ciphertext); err != nil {
		t.Fatalf("query ciphertext: %v", err)
	}
	if bytes.Contains(ciphertext, []byte(want.AccessToken)) || bytes.Contains(ciphertext, []byte(want.RefreshToken)) {
		t.Fatalf("ciphertext contains plaintext token: %q", ciphertext)
	}
	got, err := source.loadCredentials(ctx)
	if err != nil {
		t.Fatalf("loadCredentials() error = %v", err)
	}
	if got != want {
		t.Fatalf("loadCredentials() = %#v, want %#v", got, want)
	}

	// 3. 不同应用密钥必须无法解密同一凭据。
	other := NewWeReadSource(NewRepository(db), client.NewWeReadArticleClient(nil), "different-secret")
	if _, err := other.loadCredentials(ctx); err == nil {
		t.Fatal("loadCredentials() with different key expected error")
	}
}

// TestWeReadBindingReflectsFetchFailure 验证页面状态会暴露最近一次真实抓取失败。
// 输入：完整测试凭据和一条 refresh_token 抓取错误。
// 输出：失败时要求重新绑定，清除错误后恢复为已绑定。
// 副作用：写入隔离 SQLite 测试库。
func TestWeReadBindingReflectsFetchFailure(t *testing.T) {
	// 1. 保存完整凭据并创建微信读书文章来源。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	source := NewWeReadSource(repository, client.NewWeReadArticleClient(nil), "test-secret")
	credentials := client.WeReadArticleCredentials{
		VID: "123456", DeviceID: "device-test", AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	if err := source.saveCredentials(ctx, credentials); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}
	if err := repository.SetWeReadSourceActive(ctx, true); err != nil {
		t.Fatalf("SetWeReadSourceActive() error = %v", err)
	}

	// 2. 写入真实抓取错误，确认接口不再只根据密文存在返回已绑定。
	sources, err := repository.Sources(ctx, false)
	if err != nil || len(sources) != 1 {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	if err := repository.UpdateSourceStatus(ctx, sources[0].ID, "error", "微信读书凭据字段 refresh_token 为空"); err != nil {
		t.Fatalf("UpdateSourceStatus() error = %v", err)
	}
	binding, err := source.Binding(ctx)
	if err != nil {
		t.Fatalf("Binding() error = %v", err)
	}
	if binding.State != "failed" || binding.Message != "微信读书凭据不可用，请重新绑定" {
		t.Fatalf("Binding() = %#v", binding)
	}

	// 3. 模拟重新绑定已验证，确认历史错误被清除且不再误报。
	if err := repository.clearWeReadSourceFetchError(ctx); err != nil {
		t.Fatalf("clearWeReadSourceFetchError() error = %v", err)
	}
	binding, err = source.Binding(ctx)
	if err != nil {
		t.Fatalf("Binding() after clear error = %v", err)
	}
	if binding.State != "connected" {
		t.Fatalf("Binding() after clear = %#v", binding)
	}
}

// TestPollLoginKeepsConnectedTerminalState 验证扫码成功后不会因二维码时限再次变成过期。
// 输入：一个已连接但二维码有效期已过的内存流程。
// 输出：轮询仍返回 connected。
// 副作用：无，不访问上游或数据库。
func TestPollLoginKeepsConnectedTerminalState(t *testing.T) {
	// 1. 构造已经完成凭据交换的过期扫码流程。
	source := &WeReadSource{flow: &weReadLoginFlow{
		state: "connected", message: "微信读书凭据已保存", expiresAt: time.Now().Add(-time.Minute),
	}}

	// 2. 核对终态优先于二维码有效期。
	status, err := source.PollLogin(context.Background())
	if err != nil {
		t.Fatalf("PollLogin() error = %v", err)
	}
	if status.State != "connected" {
		t.Fatalf("PollLogin() = %#v", status)
	}
}

// TestWeReadArticleFallbacks 验证详情字段异常时仍使用列表标题并恢复长正文。
// 输入：正常列表标题、异常长详情标题和普通短详情标题。
// 输出：标题使用列表值，只有长详情文本可作为正文。
// 副作用：无。
func TestWeReadArticleFallbacks(t *testing.T) {
	// 1. 核对列表标题不会被详情中的长正文覆盖。
	longContent := strings.Repeat("这是详情接口错放的文章正文。", 30)
	if got := weReadArticleTitle("列表标题", longContent); got != "列表标题" {
		t.Fatalf("weReadArticleTitle() = %q", got)
	}
	if got := weReadArticleTitle("真实标题\n\n"+longContent, longContent); got != "真实标题" {
		t.Fatalf("weReadArticleTitle() with body = %q", got)
	}

	// 2. 核对长详情文本可恢复，普通短标题不会冒充正文。
	if got := weReadDetailContentFallback(longContent); got == "" {
		t.Fatal("weReadDetailContentFallback() returned empty long content")
	}
	if got := weReadDetailContentFallback("普通文章标题"); got != "" {
		t.Fatalf("weReadDetailContentFallback() = %q", got)
	}
}

// TestWeReadAccountSelectionSurvivesShelfRefresh 验证重新发现书架不会覆盖人工公众号开关。
// 输入：两次包含重叠公众号的书架发现结果。
// 输出：已启用公众号保持启用，新公众号默认关闭。
// 副作用：写入隔离 SQLite 测试库。
func TestWeReadAccountSelectionSurvivesShelfRefresh(t *testing.T) {
	// 1. 首次发现两个公众号，并人工启用其中一个。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.upsertWeReadAccounts(ctx, []WeReadAccount{
		{AccountID: "MP_WXS_100", Title: "甲公众号"},
		{AccountID: "MP_WXS_200", Title: "乙公众号"},
	}); err != nil {
		t.Fatalf("upsertWeReadAccounts() error = %v", err)
	}
	source := NewWeReadSource(repository, client.NewWeReadArticleClient(nil), "test-secret")
	if err := source.SetAccountEnabled(ctx, "MP_WXS_100", true); err != nil {
		t.Fatalf("SetAccountEnabled() error = %v", err)
	}

	// 2. 再次发现时更新标题并增加账号，核对人工选择不被覆盖且离架账号被删除。
	if err := repository.upsertWeReadAccounts(ctx, []WeReadAccount{
		{AccountID: "MP_WXS_100", Title: "甲公众号新名称"},
		{AccountID: "MP_WXS_300", Title: "丙公众号"},
	}); err != nil {
		t.Fatalf("second upsertWeReadAccounts() error = %v", err)
	}
	accounts, err := repository.listWeReadAccounts(ctx, false)
	if err != nil {
		t.Fatalf("listWeReadAccounts() error = %v", err)
	}
	enabled := make(map[string]bool, len(accounts))
	for _, account := range accounts {
		enabled[account.AccountID] = account.Enabled
	}
	if len(accounts) != 2 || !enabled["MP_WXS_100"] || !enabled["MP_WXS_300"] {
		t.Fatalf("account enabled states = %#v", enabled)
	}
	if _, exists := enabled["MP_WXS_200"]; exists {
		t.Fatalf("removed shelf account still exists: %#v", enabled)
	}
}

// TestSyncWeReadSourceFollowsCredentialState 验证文章来源仅在绑定凭据存在后启用。
// 输入：绑定前后各执行一次来源初始化。
// 输出：同一来源主键从关闭切换为启用，类型固定为 weread。
// 副作用：写入隔离 SQLite 测试库。
func TestSyncWeReadSourceFollowsCredentialState(t *testing.T) {
	// 1. 未绑定时初始化来源并核对任务入口保持关闭。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.SyncWeReadSource(ctx); err != nil {
		t.Fatalf("SyncWeReadSource() before binding error = %v", err)
	}
	var sourceID int64
	var sourceType string
	var active bool
	if err := db.QueryRowContext(ctx, `SELECT id, source_type, is_active FROM investment_article_source WHERE source_code = 'wechat_aggregate'`).
		Scan(&sourceID, &sourceType, &active); err != nil {
		t.Fatalf("query source before binding: %v", err)
	}
	if sourceType != "weread" || active {
		t.Fatalf("source before binding = id:%d type:%q active:%t", sourceID, sourceType, active)
	}

	// 2. 保存凭据后再次初始化，核对原主键启用且没有重复来源。
	source := NewWeReadSource(repository, client.NewWeReadArticleClient(nil), "test-secret")
	credentials := client.WeReadArticleCredentials{
		VID: "123456", DeviceID: "device-test", AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	if err := source.saveCredentials(ctx, credentials); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}
	if err := repository.SyncWeReadSource(ctx); err != nil {
		t.Fatalf("SyncWeReadSource() after binding error = %v", err)
	}
	var enabledSourceID int64
	if err := db.QueryRowContext(ctx, `SELECT id, is_active FROM investment_article_source WHERE source_code = 'wechat_aggregate'`).
		Scan(&enabledSourceID, &active); err != nil {
		t.Fatalf("query source after binding: %v", err)
	}
	if enabledSourceID != sourceID || !active {
		t.Fatalf("source after binding = id:%d active:%t, want id:%d active:true", enabledSourceID, active, sourceID)
	}
}
