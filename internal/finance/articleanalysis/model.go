// Package articleanalysis 提供投资文章抓取、结构化分析和统计报告。
package articleanalysis

const (
	// PromptVersion 标识当前投资文章结构化提示词版本。
	PromptVersion = "investment_article_single_verdict_v7"
	// DefaultTargetDays 是信号榜默认统计天数。
	DefaultTargetDays = 60
	// DefaultMarketDays 是短期市场判断默认统计天数。
	DefaultMarketDays = 3
)

// Source 描述投资文章外部信息源。
type Source struct {
	ID               int64  `json:"id"`
	SourceCode       string `json:"source_code"`
	SourceName       string `json:"source_name"`
	SourceType       string `json:"source_type"`
	FeedURL          string `json:"-"`
	IsActive         bool   `json:"is_active"`
	Description      string `json:"description,omitempty"`
	LastFetchAt      string `json:"last_fetch_at,omitempty"`
	LastFetchStatus  string `json:"last_fetch_status,omitempty"`
	LastFetchMessage string `json:"last_fetch_message,omitempty"`
}

// FeedEntry 描述从外部文章 API 规范化得到的一篇文章。
type FeedEntry struct {
	ArticleKey  string
	ExternalID  string
	Title       string
	Link        string
	Author      string
	PublishedAt string
	Summary     string
	Content     string
	RawEntry    map[string]any
}

// Signal 描述文章中的一个推荐或风险信号。
type Signal struct {
	Name   string `json:"name"`
	Type   string `json:"type,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// SignalGroup 描述信号榜使用的规范概念组及其原始名称。
type SignalGroup struct {
	ID      int64    `json:"id"`
	Name    string   `json:"canonical_name"`
	Type    string   `json:"type"`
	Aliases []string `json:"aliases"`
}

// MarketJudgment 描述文章中的短期市场判断。
type MarketJudgment struct {
	Mood             string `json:"mood"`
	MoodReason       string `json:"mood_reason"`
	Prediction       string `json:"prediction"`
	PredictionReason string `json:"prediction_reason"`
}

// AnalysisResult 描述模型输出和数据库写入使用的结构化结果。
type AnalysisResult struct {
	Summary         string         `json:"summary"`
	Recommendations []Signal       `json:"recommendations"`
	Risks           []Signal       `json:"risks"`
	Market          MarketJudgment `json:"market"`
}

// Analysis 描述文章详情抽屉使用的分析字段。
type Analysis struct {
	Summary                string   `json:"summary,omitempty"`
	MarketMood             string   `json:"market_mood,omitempty"`
	MarketMoodReason       string   `json:"market_mood_reason,omitempty"`
	MarketPrediction       string   `json:"market_prediction,omitempty"`
	MarketPredictionReason string   `json:"market_prediction_reason,omitempty"`
	Recommendations        []Signal `json:"recommendations"`
	Risks                  []Signal `json:"risks"`
	ErrorMessage           string   `json:"error_message,omitempty"`
}

// ArticleItem 描述投资文章列表中的一行。
type ArticleItem struct {
	ID                  int64    `json:"id"`
	SourceName          string   `json:"source_name"`
	Title               string   `json:"title"`
	Author              string   `json:"author,omitempty"`
	PublishedAt         string   `json:"published_at,omitempty"`
	MarketMood          string   `json:"market_mood,omitempty"`
	MarketPrediction    string   `json:"market_prediction,omitempty"`
	RecommendationNames []string `json:"recommendation_names"`
	RiskNames           []string `json:"risk_names"`
	CreatedAt           string   `json:"created_at,omitempty"`
}

// ArticleDetail 描述单篇文章和完整分析结果。
type ArticleDetail struct {
	ArticleItem
	Link           string    `json:"link"`
	PromptFeedback string    `json:"prompt_feedback,omitempty"`
	Analysis       *Analysis `json:"analysis,omitempty"`
}

// SignalStat 描述信号榜的一行合并统计。
type SignalStat struct {
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	Members             []string       `json:"members"`
	MemberNetCounts     map[string]int `json:"member_net_counts"`
	RecommendationCount int            `json:"recommendation_count"`
	RiskCount           int            `json:"risk_count"`
	Count               int            `json:"count"`
}

// DistributionItem 描述市场枚举的计数分布。
type DistributionItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Report 描述投资文章分析页面统计响应。
type Report struct {
	AnalysisModel          string             `json:"analysis_model"`
	AnalysisPrompt         string             `json:"analysis_prompt"`
	PromptVersion          string             `json:"prompt_version"`
	Signals                []SignalStat       `json:"signals"`
	MoodDistribution       []DistributionItem `json:"mood_distribution"`
	PredictionDistribution []DistributionItem `json:"prediction_distribution"`
}

// PageMetric 描述文章抓取或分析页面顶部指标。
type PageMetric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Status string `json:"status,omitempty"`
}

// PageSummary 描述文章抓取或分析页面摘要。
type PageSummary struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	Metrics         []PageMetric `json:"metrics"`
	LatestArticleAt string       `json:"latest_article_at,omitempty"`
}

// SyncResult 描述一次外部文章同步和可选分析的统计。
type SyncResult struct {
	SourceCount          int                 `json:"source_count"`
	FetchedCount         int                 `json:"fetched_count"`
	LatestFetchedAt      string              `json:"latest_fetched_at,omitempty"`
	InsertedCount        int                 `json:"inserted_count"`
	UpdatedCount         int                 `json:"updated_count"`
	FailedSources        []map[string]string `json:"failed_sources"`
	AnalyzedCount        int                 `json:"analyzed_count"`
	ClassifiedAliasCount int                 `json:"classified_alias_count"`
	SkippedCount         int                 `json:"skipped_count"`
	ErrorCount           int                 `json:"error_count"`
	PendingCount         int                 `json:"pending_count"`
}

// WeReadAccount 描述可参与投资文章抓取的微信读书书架公众号。
type WeReadAccount struct {
	AccountID            string `json:"account_id"`
	Title                string `json:"title"`
	CoverURL             string `json:"cover_url,omitempty"`
	Enabled              bool   `json:"enabled"`
	FetchIntervalMinutes int    `json:"fetch_interval_minutes"`
	FetchLimit           int    `json:"fetch_limit"`
	LastCheckedAt        string `json:"last_checked_at,omitempty"`
	ArticleCount         int    `json:"article_count"`
	TodayInsertedCount   int    `json:"today_inserted_count"`
	PendingCount         int    `json:"pending_count"`
	LatestFetchedAt      string `json:"latest_fetched_at,omitempty"`
}

// WeReadAccountSettings 描述单个公众号抓取节奏。
type WeReadAccountSettings struct {
	FetchIntervalMinutes int `json:"fetch_interval_minutes"`
	FetchLimit           int `json:"fetch_limit"`
}

// WeReadBinding 描述文章抓取页展示的微信读书连接和公众号状态。
type WeReadBinding struct {
	State    string          `json:"state"`
	Message  string          `json:"message"`
	Accounts []WeReadAccount `json:"accounts"`
}

// WeReadLoginStatus 描述单个内存扫码流程的公开状态。
type WeReadLoginStatus struct {
	State     string `json:"state"`
	Message   string `json:"message"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// AnalysisBatchResult 描述一批待分析文章的处理统计。
type AnalysisBatchResult struct {
	AnalyzedCount int              `json:"analyzed_count"`
	SkippedCount  int              `json:"skipped_count"`
	ErrorCount    int              `json:"error_count"`
	Items         []map[string]any `json:"items"`
}
