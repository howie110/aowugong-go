package articleanalysis

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

type articleAnalysisRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 执行文章分析测试声明的内存 HTTP 响应。
// 输入：request 是微信读书客户端生成的请求。
// 输出：返回测试响应或传输错误。
// 副作用：调用测试闭包，不访问网络。
func (function articleAnalysisRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	// 1. 把请求交给当前测试定义的处理函数。
	return function(request)
}

// articleAnalysisHTTPResponse 创建绑定到原请求的内存 HTTP 响应。
// 输入：request 是原请求，status 是状态码，body 是响应正文。
// 输出：返回可由标准客户端关闭的响应。
// 副作用：分配内存正文读取器。
func articleAnalysisHTTPResponse(request *http.Request, status int, body string) *http.Response {
	// 1. 写入微信读书客户端解码所需的最小响应字段。
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

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
		VID: "123456", DeviceID: "device-test", InstallID: "install-test",
		AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	if err := source.saveCredentials(ctx, want, true); err != nil {
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

// TestWeReadCredentialHealthLifecycle 验证多次检查会累计绑定寿命和自动刷新次数。
// 输入：一份新扫码凭据、一次有效检查、一次自动刷新和一次认证失效。
// 输出：绑定时间不被刷新覆盖，检查与刷新次数准确，寿命停在首次失效时间。
// 副作用：写入隔离 SQLite 测试库。
func TestWeReadCredentialHealthLifecycle(t *testing.T) {
	// 1. 保存新绑定并以持久绑定时间为基准构造后续检查时间。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	source := NewWeReadSource(repository, client.NewWeReadArticleClient(nil), "test-encryption-secret")
	credentials := client.WeReadArticleCredentials{
		VID: "123456", DeviceID: "stable-device", InstallID: "stable-install",
		AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	if err := source.saveCredentials(ctx, credentials, true); err != nil {
		t.Fatalf("save new credentials: %v", err)
	}
	health, found, err := repository.weReadCredentialHealth(ctx)
	if err != nil || !found {
		t.Fatalf("initial health = %#v, %v, %v", health, found, err)
	}
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	boundAt, err := time.ParseInLocation("2006-01-02 15:04:05", health.BoundAt, location)
	if err != nil {
		t.Fatalf("parse bound time: %v", err)
	}

	// 2. 有效检查和 Token 刷新分别累计，不允许刷新重置绑定起点。
	if err := repository.recordWeReadCredentialCheck(ctx, "valid", "", boundAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("record valid check: %v", err)
	}
	credentials.AccessToken = "refreshed-access"
	if err := source.saveCredentials(ctx, credentials, false); err != nil {
		t.Fatalf("save refreshed credentials: %v", err)
	}
	if err := repository.recordWeReadCredentialCheck(ctx, "invalid", "认证失效", boundAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("record invalid check: %v", err)
	}

	// 3. 首次失效固定三小时寿命，累计两次检查和一次自动刷新。
	health, found, err = repository.weReadCredentialHealth(ctx)
	if err != nil || !found {
		t.Fatalf("final health = %#v, %v, %v", health, found, err)
	}
	result := buildWeReadCredentialCheckResult(health, boundAt.Add(5*time.Hour), 0)
	if result.Status != "invalid" || result.ValidFor != "3小时" || result.CheckCount != 2 || result.RefreshCount != 1 {
		t.Fatalf("credential result = %#v", result)
	}
}

// TestWeReadCredentialCheckUsesArticleEndpoint 验证凭据检查使用正式文章列表接口而不是较宽松的书架接口。
// 输入：一个已启用公众号和返回人工验证错误的内存微信读书服务。
// 输出：检查返回人工验证错误并把凭据健康状态记为警告。
// 副作用：执行一次内存 HTTP 请求并写入隔离测试数据库。
func TestWeReadCredentialCheckUsesArticleEndpoint(t *testing.T) {
	// 1. 保存完整凭据并准备一个参与正式抓取的公众号。
	ctx := context.Background()
	db := testdatabase.Open(t)
	repository := NewRepository(db)
	if err := repository.upsertWeReadAccounts(ctx, []WeReadAccount{{
		AccountID: "MP_WXS_100", Title: "测试公众号",
	}}); err != nil {
		t.Fatalf("upsertWeReadAccounts() error = %v", err)
	}
	if err := repository.setWeReadAccountEnabled(ctx, "MP_WXS_100", true); err != nil {
		t.Fatalf("setWeReadAccountEnabled() error = %v", err)
	}
	transport := articleAnalysisRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/mp/chapters" || request.URL.Query().Get("count") != "1" {
			t.Fatalf("credential check request = %s", request.URL.String())
		}
		return articleAnalysisHTTPResponse(request, http.StatusOK, `{"errCode":-2041,"errMsg":"verify required"}`), nil
	})
	source := NewWeReadSource(repository, client.NewWeReadArticleClient(&http.Client{Transport: transport}), "test-secret")
	credentials := client.WeReadArticleCredentials{
		VID: "123456", DeviceID: "device-test", AccessToken: "access-secret", RefreshToken: "refresh-secret",
	}
	if err := source.saveCredentials(ctx, credentials, true); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}

	// 2. 检查必须暴露人工验证错误，并持久化为页面可见的警告状态。
	if _, err := source.CheckCredential(ctx); !errors.Is(err, client.ErrWeReadArticleVerification) {
		t.Fatalf("CheckCredential() error = %v", err)
	}
	health, found, err := repository.weReadCredentialHealth(ctx)
	if err != nil || !found {
		t.Fatalf("weReadCredentialHealth() = %#v, %v, %v", health, found, err)
	}
	if health.LastStatus != "error" || !strings.Contains(health.LastError, "测试公众号") {
		t.Fatalf("credential health = %#v", health)
	}
}

// TestWeReadBindingReflectsFetchFailure 验证页面状态会暴露监控或抓取发现的凭据失败。
// 输入：完整测试凭据、一条 refresh_token 监控错误和一条抓取错误。
// 输出：两类失败都要求重新绑定，恢复有效后显示可用。
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
	if err := source.saveCredentials(ctx, credentials, true); err != nil {
		t.Fatalf("saveCredentials() error = %v", err)
	}
	if err := repository.SetWeReadSourceActive(ctx, true); err != nil {
		t.Fatalf("SetWeReadSourceActive() error = %v", err)
	}

	// 2. 写入监控发现的凭据错误，确认接口不再只根据密文存在返回可用。
	if err := repository.recordWeReadCredentialCheck(ctx, "error", "读取微信读书书架: 微信读书凭据字段 refresh_token 为空", time.Now()); err != nil {
		t.Fatalf("recordWeReadCredentialCheck() error = %v", err)
	}
	binding, err := source.Binding(ctx)
	if err != nil {
		t.Fatalf("Binding() with health error = %v", err)
	}
	if binding.State != "failed" || binding.Message != "微信读书凭据已失效，请重新绑定" {
		t.Fatalf("Binding() with health error = %#v", binding)
	}
	if err := repository.recordWeReadCredentialCheck(ctx, "valid", "", time.Now()); err != nil {
		t.Fatalf("record valid check: %v", err)
	}

	// 3. 正文节点缺失只表示部分文章解析失败，不应要求重新绑定。
	sources, err := repository.Sources(ctx, false)
	if err != nil || len(sources) != 1 {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	if err := repository.UpdateSourceStatus(ctx, sources[0].ID, "error", "微信读书文章读取部分失败: 测试文章: 微信原文缺少正文节点"); err != nil {
		t.Fatalf("UpdateSourceStatus() content error = %v", err)
	}
	binding, err = source.Binding(ctx)
	if err != nil {
		t.Fatalf("Binding() content error = %v", err)
	}
	if binding.State != "degraded" || binding.Message != "部分文章正文解析失败，成功文章已入库" {
		t.Fatalf("Binding() content error = %#v", binding)
	}

	// 4. 人工验证只显示警告，不应要求重新绑定。
	if err := repository.UpdateSourceStatus(ctx, sources[0].ID, "error", "读取公众号 测试文章: 微信读书要求人工验证"); err != nil {
		t.Fatalf("UpdateSourceStatus() verification error = %v", err)
	}
	binding, err = source.Binding(ctx)
	if err != nil {
		t.Fatalf("Binding() verification error = %v", err)
	}
	if binding.State != "degraded" || binding.Message != "微信读书暂时拒绝服务器请求，请稍后重试" {
		t.Fatalf("Binding() verification error = %#v", binding)
	}

	// 5. 明确凭据错误仍然要求重新绑定。
	if err := repository.UpdateSourceStatus(ctx, sources[0].ID, "error", "微信读书凭据字段 refresh_token 为空"); err != nil {
		t.Fatalf("UpdateSourceStatus() credential error = %v", err)
	}
	binding, err = source.Binding(ctx)
	if err != nil {
		t.Fatalf("Binding() credential error = %v", err)
	}
	if binding.State != "failed" || binding.Message != "微信读书凭据不可用，请重新绑定" {
		t.Fatalf("Binding() = %#v", binding)
	}

	// 6. 模拟重新绑定已验证，确认历史错误被清除且不再误报。
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
	if err := source.saveCredentials(ctx, credentials, true); err != nil {
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
