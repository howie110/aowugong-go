package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/howiedata/aowugong-go/internal/auth"
	"github.com/howiedata/aowugong-go/internal/finance/articleanalysis"
	"github.com/howiedata/aowugong-go/internal/rbac"
)

type articleAnalysisHandlers struct {
	service *articleanalysis.Service
	rbac    *rbac.Service
}

type promptFeedbackPayload struct {
	PromptFeedback string `json:"prompt_feedback"`
}

type weReadAccountPayload struct {
	FetchIntervalMinutes int `json:"fetch_interval_minutes"`
	FetchLimit           int `json:"fetch_limit"`
}

// registerArticleAnalysisRoutes 注册文章抓取、分析、报告和反馈接口。
// 输入：router 是 API 路由器，authService 和 rbacService 提供访问控制，service 提供文章业务。
// 输出：无。
// 副作用：修改路由注册表。
func registerArticleAnalysisRoutes(router chi.Router, authService *auth.Service, rbacService *rbac.Service, service *articleanalysis.Service) {
	// 1. 分析查看接口使用 investor 可拥有的文章分析权限。
	handlers := articleAnalysisHandlers{service: service, rbac: rbacService}
	analysis := router.With(authenticate(authService), requirePermission(rbacService, rbac.PermissionFinanceArticleAnalysis))
	analysis.Get("/api/v1/finance/article-analysis/summary", handlers.summary)
	analysis.Get("/api/v1/finance/article-analysis/articles", handlers.articles)
	analysis.Get("/api/v1/finance/article-analysis/articles/{articleID}", handlers.detail)
	analysis.Get("/api/v1/finance/article-analysis/report", handlers.report)

	// 2. 抓取与模型执行接口使用管理员默认拥有的文章抓取权限。
	fetch := router.With(authenticate(authService), requirePermission(rbacService, rbac.PermissionFinanceArticleFetch))
	fetch.Get("/api/v1/finance/article-analysis/fetch-summary", handlers.fetchSummary)
	fetch.Get("/api/v1/finance/article-analysis/sources", handlers.sources)
	fetch.Post("/api/v1/finance/article-analysis/sync", handlers.sync)
	fetch.Post("/api/v1/finance/article-analysis/analyze", handlers.analyze)
	fetch.Post("/api/v1/finance/article-analysis/parse", handlers.parse)
	fetch.Get("/api/v1/finance/article-analysis/weread", handlers.weReadBinding)
	fetch.Post("/api/v1/finance/article-analysis/weread/login", handlers.startWeReadLogin)
	fetch.Get("/api/v1/finance/article-analysis/weread/login", handlers.pollWeReadLogin)
	fetch.Get("/api/v1/finance/article-analysis/weread/login/qr.png", handlers.weReadLoginQR)
	fetch.Post("/api/v1/finance/article-analysis/weread/accounts/refresh", handlers.refreshWeReadAccounts)
	fetch.Patch("/api/v1/finance/article-analysis/weread/accounts/{accountID}", handlers.updateWeReadAccount)

	// 3. 人工反馈只要求登录，处理器内额外校验 admin 角色。
	feedback := router.With(authenticate(authService))
	feedback.Post("/api/v1/finance/article-analysis/articles/{articleID}/prompt-feedback", handlers.feedback)
	feedback.Patch("/api/v1/finance/article-analysis/articles/{articleID}/prompt-feedback", handlers.feedback)
}

// weReadBinding 返回微信读书连接和书架公众号状态。
// 输入：request 已通过文章抓取权限校验。
// 输出：写入 WeReadBinding JSON。
// 副作用：读取 PostgreSQL 和内存扫码状态。
func (h articleAnalysisHandlers) weReadBinding(w http.ResponseWriter, request *http.Request) {
	// 1. 调用统一文章服务并转换读取错误。
	result, err := h.service.WeReadBinding(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取微信读书绑定状态失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// startWeReadLogin 创建或复用微信读书扫码流程。
// 输入：request 已通过文章抓取权限校验。
// 输出：写入公开扫码状态。
// 副作用：调用微信上游并修改进程内扫码状态。
func (h articleAnalysisHandlers) startWeReadLogin(w http.ResponseWriter, request *http.Request) {
	// 1. 创建二维码并返回等待状态。
	result, err := h.service.StartWeReadLogin(request.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "创建微信读书二维码失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// pollWeReadLogin 推进一次微信读书扫码流程。
// 输入：request 已通过文章抓取权限校验。
// 输出：写入最新扫码状态。
// 副作用：调用微信上游，成功时加密写 PostgreSQL。
func (h articleAnalysisHandlers) pollWeReadLogin(w http.ResponseWriter, request *http.Request) {
	// 1. 执行一次轮询并返回明确状态。
	result, err := h.service.PollWeReadLogin(request.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "确认微信读书扫码状态失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// weReadLoginQR 返回当前微信读书二维码 PNG。
// 输入：request 已通过文章抓取权限校验。
// 输出：写入 image/png。
// 副作用：读取内存扫码状态并写 HTTP 响应。
func (h articleAnalysisHandlers) weReadLoginQR(w http.ResponseWriter, request *http.Request) {
	// 1. 生成二维码并禁止浏览器缓存一次性内容。
	content, err := h.service.WeReadLoginQRPNG()
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "微信读书二维码不存在或已过期")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// refreshWeReadAccounts 重新发现微信读书书架公众号。
// 输入：request 已通过文章抓取权限校验。
// 输出：写入最新 WeReadAccount 数组。
// 副作用：调用微信读书并写 PostgreSQL。
func (h articleAnalysisHandlers) refreshWeReadAccounts(w http.ResponseWriter, request *http.Request) {
	// 1. 刷新书架并返回完整账号列表。
	result, err := h.service.RefreshWeReadAccounts(request.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "刷新微信读书公众号失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// updateWeReadAccount 更新一个公众号的投资文章抓取策略。
// 输入：路径 accountID 和 JSON 抓取频率、单次数量。
// 输出：成功时写入 status=ok。
// 副作用：写 PostgreSQL。
func (h articleAnalysisHandlers) updateWeReadAccount(w http.ResponseWriter, request *http.Request) {
	// 1. 解析策略并调用统一服务入口。
	var payload weReadAccountPayload
	if err := decodeJSON(w, request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "公众号抓取策略请求无效")
		return
	}
	settings := articleanalysis.WeReadAccountSettings{
		FetchIntervalMinutes: payload.FetchIntervalMinutes,
		FetchLimit:           payload.FetchLimit,
	}
	if err := h.service.SetWeReadAccountSettings(request.Context(), chi.URLParam(request, "accountID"), settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "更新公众号抓取策略失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// summary 返回投资文章分析页面摘要。
// 输入：request 已通过认证和文章分析权限校验。
// 输出：写入 PageSummary JSON。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h articleAnalysisHandlers) summary(w http.ResponseWriter, request *http.Request) {
	// 1. 读取统一摘要并转换服务错误。
	result, err := h.service.AnalysisSummary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取投资文章分析摘要失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// fetchSummary 返回投资文章抓取页面摘要。
// 输入：request 已通过认证和文章抓取权限校验。
// 输出：写入 PageSummary JSON。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h articleAnalysisHandlers) fetchSummary(w http.ResponseWriter, request *http.Request) {
	// 1. 读取统一摘要并转换服务错误。
	result, err := h.service.FetchSummary(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取投资文章抓取摘要失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// sources 返回当前文章信息源及抓取状态。
// 输入：request 已通过认证和文章抓取权限校验。
// 输出：写入 Source 数组。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h articleAnalysisHandlers) sources(w http.ResponseWriter, request *http.Request) {
	// 1. 返回包含未配置来源的完整列表。
	result, err := h.service.Sources(request.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取投资文章来源失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// articles 返回指定天数内的已分析文章。
// 输入：查询参数 days 范围 1 到 365，limit 范围 1 到 5000。
// 输出：写入 ArticleItem 数组。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h articleAnalysisHandlers) articles(w http.ResponseWriter, request *http.Request) {
	// 1. 解析并约束查询范围。
	days, ok := boundedQueryInt(w, request, "days", 60, 1, 365)
	if !ok {
		return
	}
	limit, ok := boundedQueryInt(w, request, "limit", 50, 1, 5000)
	if !ok {
		return
	}

	// 2. 查询文章并返回。
	result, err := h.service.Articles(request.Context(), days, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取投资文章列表失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// detail 返回单篇文章和完整分析。
// 输入：路径参数 articleID 是正整数。
// 输出：写入 ArticleDetail，不存在时返回 404。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h articleAnalysisHandlers) detail(w http.ResponseWriter, request *http.Request) {
	// 1. 解析文章主键并查询详情。
	articleID, ok := positivePathID(w, chi.URLParam(request, "articleID"))
	if !ok {
		return
	}
	result, err := h.service.Detail(request.Context(), articleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取投资文章详情失败")
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "not_found", "文章不存在")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// feedback 保存管理员的提示词修正意见。
// 输入：路径参数 articleID 和 JSON prompt_feedback。
// 输出：写入更新后的 ArticleDetail。
// 副作用：读取角色、写入 PostgreSQL 和 HTTP 响应。
func (h articleAnalysisHandlers) feedback(w http.ResponseWriter, request *http.Request) {
	// 1. 读取当前用户并明确校验 admin 角色。
	user, ok := currentUser(request)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "缺少当前用户")
		return
	}
	isAdmin, err := h.rbac.HasRole(request.Context(), user.ID, rbac.AdminRoleCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "检查管理员角色失败")
		return
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "仅管理员可以保存提示词反馈")
		return
	}

	// 2. 解析文章主键和 JSON 请求。
	articleID, valid := positivePathID(w, chi.URLParam(request, "articleID"))
	if !valid {
		return
	}
	var payload promptFeedbackPayload
	if err := decodeJSON(w, request, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提示词反馈请求无效")
		return
	}

	// 3. 更新反馈并返回最新详情。
	result, err := h.service.UpdatePromptFeedback(request.Context(), articleID, payload.PromptFeedback)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "保存提示词反馈失败")
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "not_found", "文章不存在")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// report 返回 60 天信号榜和 3 天市场判断分布。
// 输入：target_days 范围 1 到 365，market_days 范围 1 到 30。
// 输出：写入 Report JSON。
// 副作用：读取 PostgreSQL 并写入 HTTP 响应。
func (h articleAnalysisHandlers) report(w http.ResponseWriter, request *http.Request) {
	// 1. 独立解析两个统计范围。
	targetDays, ok := boundedQueryInt(w, request, "target_days", 60, 1, 365)
	if !ok {
		return
	}
	marketDays, ok := boundedQueryInt(w, request, "market_days", 3, 1, 30)
	if !ok {
		return
	}

	// 2. 生成并返回报告。
	result, err := h.service.Report(request.Context(), targetDays, marketDays)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "读取投资文章报告失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// sync 手动读取微信读书公众号并可继续执行模型分析。
// 输入：fetch_limit、analyze 和 analysis_limit 是可选查询参数。
// 输出：写入 SyncResult。
// 副作用：调用微信读书、微信公众号原文和 DeepSeek，写入 PostgreSQL 和 HTTP 响应。
func (h articleAnalysisHandlers) sync(w http.ResponseWriter, request *http.Request) {
	// 1. 解析批量上限和布尔开关。
	fetchLimit, ok := boundedQueryInt(w, request, "fetch_limit", 30, 1, 100)
	if !ok {
		return
	}
	analysisLimit, ok := boundedQueryInt(w, request, "analysis_limit", 10, 1, 50)
	if !ok {
		return
	}
	analyze := false
	if value := request.URL.Query().Get("analyze"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "analyze 必须是布尔值")
			return
		}
		analyze = parsed
	}

	// 2. 调用统一同步入口并返回统计。
	result, err := h.service.SyncNow(request.Context(), fetchLimit, analyze, analysisLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "同步投资文章失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// analyze 手动分析一批待处理文章。
// 输入：limit 范围 1 到 50。
// 输出：写入 AnalysisBatchResult。
// 副作用：调用 DeepSeek、写入 PostgreSQL 和 HTTP 响应。
func (h articleAnalysisHandlers) analyze(w http.ResponseWriter, request *http.Request) {
	// 1. 解析批量上限并调用统一分析入口。
	limit, ok := boundedQueryInt(w, request, "limit", 10, 1, 50)
	if !ok {
		return
	}
	all := request.URL.Query().Get("all") == "true"
	var result articleanalysis.AnalysisBatchResult
	var err error
	if all {
		result, err = h.service.AnalyzeAllPending(request.Context())
	} else {
		result, err = h.service.AnalyzePending(request.Context(), limit)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "分析投资文章失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// parse 手动解析已抓取但缺少正文的文章。
// 输入：limit 是本次解析数量上限。
// 输出：写入 ParseBatchResult。
// 副作用：调用微信读书原文接口并写入 PostgreSQL。
func (h articleAnalysisHandlers) parse(w http.ResponseWriter, request *http.Request) {
	limit, ok := boundedQueryInt(w, request, "limit", 30, 1, 100)
	if !ok {
		return
	}
	result, err := h.service.ParsePending(request.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "解析投资文章失败")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// boundedQueryInt 解析有闭区间约束的整数查询参数。
// 输入：w/request 提供 HTTP 上下文，name 是参数名，defaultValue/min/max 描述范围。
// 输出：成功返回数值和 true，失败写入 400 并返回 false。
// 副作用：参数无效时写入 HTTP 响应。
func boundedQueryInt(w http.ResponseWriter, request *http.Request, name string, defaultValue, min, max int) (int, bool) {
	// 1. 缺失时使用调用方默认值。
	value := request.URL.Query().Get(name)
	if value == "" {
		return defaultValue, true
	}

	// 2. 解析并检查闭区间。
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || parsed > max {
		writeError(w, http.StatusBadRequest, "invalid_request", name+" 必须在 "+strconv.Itoa(min)+" 到 "+strconv.Itoa(max)+" 之间")
		return 0, false
	}
	return parsed, true
}

// positivePathID 解析正整数路径主键。
// 输入：w 是响应，value 是路径参数。
// 输出：成功返回 int64 和 true，失败写入 400 并返回 false。
// 副作用：参数无效时写入 HTTP 响应。
func positivePathID(w http.ResponseWriter, value string) (int64, bool) {
	// 1. 解析十进制正整数。
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "文章 ID 无效")
		return 0, false
	}
	return parsed, true
}
