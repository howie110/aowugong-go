// Package monitoring 提供服务目标探测、结果持久化和页面摘要。
package monitoring

// Target 描述一个需要探测的服务。
type Target struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	ProbeURL    string  `json:"-"`
	Description *string `json:"description"`
}

// Result 描述一次标准服务探测结果。
type Result struct {
	TargetCode   string  `json:"target_code"`
	TargetName   string  `json:"target_name"`
	TargetURL    string  `json:"target_url"`
	Status       string  `json:"status"`
	HTTPStatus   *int    `json:"http_status"`
	LatencyMS    *int    `json:"latency_ms"`
	ErrorMessage *string `json:"error_message"`
	CheckedAt    *string `json:"checked_at"`
}

// CheckResult 描述一轮全部目标探测汇总。
type CheckResult struct {
	CheckedCount int      `json:"checked_count"`
	UpCount      int      `json:"up_count"`
	DownCount    int      `json:"down_count"`
	Results      []Result `json:"results"`
}

// Summary 描述服务监控页面数据。
type Summary struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Metrics     []map[string]string `json:"metrics"`
	Services    []Result            `json:"services"`
}
