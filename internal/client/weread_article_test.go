package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type weReadRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 执行测试定义的微信读书响应。
// 输入：request 是客户端生成的请求。
// 输出：返回测试响应或错误。
// 副作用：调用测试闭包。
func (function weReadRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	// 1. 把请求交给当前测试声明的处理函数。
	return function(request)
}

// TestWeReadArticleClientReadsShelfAndRecentPage 验证书架发现和最近文章使用固定认证协议。
// 输入：本地拦截传输层提供书架和文章页 JSON。
// 输出：返回一个公众号和一篇文章引用。
// 副作用：执行两次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientReadsShelfAndRecentPage(t *testing.T) {
	// 1. 拦截两个固定端点并检查账号认证头。
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("vid") != "123456" || request.Header.Get("accessToken") != "access-token" {
			t.Fatalf("authentication headers = vid:%q token:%q", request.Header.Get("vid"), request.Header.Get("accessToken"))
		}
		body := ""
		switch request.URL.Path {
		case "/shelf/sync":
			body = `{"books":[{"bookId":"MP_WXS_100","title":"测试公众号","cover":"https://example.com/cover"},{"bookId":"book-1","title":"普通书籍"}]}`
		case "/mp/chapters":
			if request.URL.Query().Get("bookId") != "MP_WXS_100" || request.URL.Query().Get("count") != "20" || request.URL.Query().Get("synckey") != "0" {
				t.Fatalf("chapter query = %q", request.URL.RawQuery)
			}
			body = `{"data":[{"reviewId":"review-1","title":"文章一","createTime":100}],"synckey":100}`
		default:
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
		return weReadTestResponse(request, http.StatusOK, "application/json", body), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0
	credentials := WeReadArticleCredentials{
		VID: "123456", DeviceID: "device-test", AccessToken: "access-token", RefreshToken: "refresh-token",
	}

	// 2. 发现时只保留公众号，再读取其最近一页文章。
	accounts, err := articleClient.DiscoverPublicAccounts(context.Background(), &credentials)
	if err != nil || len(accounts) != 1 || accounts[0].AccountID != "MP_WXS_100" {
		t.Fatalf("DiscoverPublicAccounts() = %#v, %v", accounts, err)
	}
	articles, err := articleClient.ListRecentArticles(context.Background(), &credentials, accounts[0].AccountID, 20)
	if err != nil || len(articles) != 1 || articles[0].ReviewID != "review-1" {
		t.Fatalf("ListRecentArticles() = %#v, %v", articles, err)
	}
}

// TestWeReadArticleClientReusesExistingDevice 验证重新扫码继续模拟同一台 BOOX 设备。
// 输入：旧凭据包含已经登记的设备 ID 和安装 ID，内存传输层接收登录请求。
// 输出：请求复用两个 ID、官方风格设备名和 User-Agent，并把安装 ID写回新凭据。
// 副作用：执行一次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientReusesExistingDevice(t *testing.T) {
	// 1. 截获登录请求并校验整套 BOOX 设备身份保持一致。
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode login payload: %v", err)
		}
		if payload["deviceId"] != "stable-device" || payload["installId"] != "stable-install" {
			t.Fatalf("device identity = %#v", payload)
		}
		if payload["deviceName"] != "BOOX" || payload["appFirstInstall"] != float64(0) {
			t.Fatalf("device presentation = %#v", payload)
		}
		if request.Header.Get("User-Agent") != weReadArticleUserAgent {
			t.Fatalf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		return weReadTestResponse(request, http.StatusOK, "application/json", `{"vid":"123456","accessToken":"new-access","refreshToken":"new-refresh","errCode":0}`), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0
	previous := WeReadArticleCredentials{DeviceID: "stable-device", InstallID: "stable-install"}

	// 2. 交换后的 Token 更新，但稳定设备身份必须继续保存。
	credentials, err := articleClient.ExchangeLoginCode(context.Background(), "scan-code", &previous)
	if err != nil {
		t.Fatalf("ExchangeLoginCode() error = %v", err)
	}
	if credentials.DeviceID != previous.DeviceID || credentials.InstallID != previous.InstallID {
		t.Fatalf("credentials = %#v", credentials)
	}
}

// TestWeReadArticleClientPreservesRefreshToken 验证刷新响应省略 RefreshToken 时沿用旧值。
// 输入：旧凭据包含完整 Token，内存传输层只返回新 AccessToken。
// 输出：新凭据更新 AccessToken 并保留原 RefreshToken。
// 副作用：执行一次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientPreservesRefreshToken(t *testing.T) {
	// 1. 返回微信读书实际可能出现的仅轮换 AccessToken 响应。
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/login" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		return weReadTestResponse(request, http.StatusOK, "application/json", `{"vid":"123456","accessToken":"new-access","errCode":0}`), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0
	credentials := WeReadArticleCredentials{
		VID: "123456", DeviceID: "stable-device", InstallID: "stable-install",
		AccessToken: "old-access", RefreshToken: "stable-refresh",
	}

	// 2. 刷新后核对新访问令牌和沿用的刷新令牌都可继续持久化。
	refreshed, err := articleClient.RefreshCredentials(context.Background(), credentials)
	if err != nil {
		t.Fatalf("RefreshCredentials() error = %v", err)
	}
	if refreshed.AccessToken != "new-access" || refreshed.RefreshToken != credentials.RefreshToken {
		t.Fatalf("refreshed credentials = %#v", refreshed)
	}
}

// TestWeReadArticleClientReadsDetailHTMLContent 验证文章详情保留微信读书返回的备用正文。
// 输入：包含 mpInfo 和顶层 htmlContent 的单篇文章详情响应。
// 输出：返回规范元数据和清理标签后的备用正文。
// 副作用：执行一次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientReadsDetailHTMLContent(t *testing.T) {
	// 1. 返回原文元数据与微信读书详情中已经携带的富文本正文。
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/review/single" || request.URL.Query().Get("reviewId") != "review-1" {
			t.Fatalf("detail request = %s", request.URL.String())
		}
		body := `{"htmlContent":"<p>第一段备用正文</p><p>第二段备用正文</p>","review":{"mpInfo":{"doc_url":"https://mp.weixin.qq.com/s/test","title":"测试文章","mp_name":"测试公众号","time":100}}}`
		return weReadTestResponse(request, http.StatusOK, "application/json", body), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0
	credentials := WeReadArticleCredentials{
		VID: "123456", DeviceID: "device-test", AccessToken: "access-token", RefreshToken: "refresh-token",
	}

	// 2. 读取详情并核对富文本已经转换为分析可用的纯文本。
	detail, err := articleClient.FetchArticleDetail(context.Background(), &credentials, "review-1")
	if err != nil {
		t.Fatalf("FetchArticleDetail() error = %v", err)
	}
	if detail.Content != "第一段备用正文 第二段备用正文" || detail.Title != "测试文章" {
		t.Fatalf("detail = %#v", detail)
	}
}

// TestWeReadArticleClientExtractsWeChatText 验证微信公众号正文只读取 js_content 可见文本。
// 输入：包含旁路内容、脚本和正文节点的 HTML。
// 输出：返回压缩后的正文文本和规范原文地址。
// 副作用：执行一次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientExtractsWeChatText(t *testing.T) {
	// 1. 返回带正确媒体类型的微信公众号测试页面。
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "mp.weixin.qq.com" || request.URL.Path != "/s/test" {
			t.Fatalf("article URL = %s", request.URL.String())
		}
		body := `<html><body><p>旁路内容</p><div id="js_content"><p>第一段</p><script>bad()</script><p>第二段</p></div></body></html>`
		return weReadTestResponse(request, http.StatusOK, "text/html; charset=utf-8", body), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0

	// 2. 核对正文不包含旁路文本和脚本。
	content, finalURL, err := articleClient.FetchArticleContent(context.Background(), "https://mp.weixin.qq.com/s/test#rd")
	if err != nil || content != "第一段 第二段" || finalURL != "https://mp.weixin.qq.com/s/test" {
		t.Fatalf("FetchArticleContent() = %q, %q, %v", content, finalURL, err)
	}
}

// TestWeReadArticleClientExtractsNewWeChatContent 验证新版正文类名可以作为兼容入口。
// 输入：不含 js_content、只含 rich_media_content 的 HTML。
// 输出：返回正文文本，不包含页面旁路内容。
// 副作用：执行一次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientExtractsNewWeChatContent(t *testing.T) {
	// 1. 返回新版正文容器，保留额外类名以验证词元匹配。
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `<html><body><header>页面标题</header><div class="rich_media_content custom"><p>新版第一段</p><p>新版第二段</p></div></body></html>`
		return weReadTestResponse(request, http.StatusOK, "text/html; charset=utf-8", body), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0

	// 2. 核对只读取新版正文容器。
	content, _, err := articleClient.FetchArticleContent(context.Background(), "https://mp.weixin.qq.com/s/new")
	if err != nil || content != "新版第一段 新版第二段" {
		t.Fatalf("FetchArticleContent() = %q, %v", content, err)
	}
}

// TestWeReadArticleClientUsesReadabilityFallback 验证正文节点变化时使用通用正文提取兜底。
// 输入：页面不包含微信固定正文节点，但主体区域包含足量段落文本。
// 输出：返回主体文章正文，不返回导航和页脚内容。
// 副作用：执行一次内存 HTTP 往返，不访问网络。
func TestWeReadArticleClientUsesReadabilityFallback(t *testing.T) {
	// 1. 构造没有 js_content 和 rich_media_content 的新版页面结构。
	body := `<html><body><nav>首页 关注 分享</nav><main><h1>长期投资观察</h1><p>这是第一段正文，用于验证通用正文提取算法能够识别页面主体并保留文章内容。</p><p>这是第二段正文，包含足够的连续文本，避免短页面被误判为导航、错误页或验证页。</p><p>这是第三段正文，说明页面结构发生变化时仍然可以进入解析流程。</p></main><footer>推荐阅读</footer></body></html>`
	transport := weReadRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return weReadTestResponse(request, http.StatusOK, "text/html; charset=utf-8", body), nil
	})
	articleClient := NewWeReadArticleClient(&http.Client{Transport: transport})
	articleClient.requestGap = 0

	// 2. 核对通用算法提取主体而不是旁路文本。
	content, _, err := articleClient.FetchArticleContent(context.Background(), "https://mp.weixin.qq.com/s/readability")
	if err != nil || !strings.Contains(content, "长期投资观察") || strings.Contains(content, "推荐阅读") {
		t.Fatalf("FetchArticleContent() = %q, %v", content, err)
	}
}

// weReadTestResponse 创建绑定到原请求的内存 HTTP 响应。
// 输入：request、状态、媒体类型和正文。
// 输出：返回可由标准客户端关闭的响应。
// 副作用：分配内存正文读取器。
func weReadTestResponse(request *http.Request, status int, contentType, body string) *http.Response {
	// 1. 写入客户端读取所需的最小响应字段。
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
