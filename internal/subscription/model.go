// Package subscription 提供订阅记录、费用摘要和到期提醒能力。
package subscription

import "errors"

var (
	ErrInvalidInput = errors.New("订阅参数无效")
	ErrConflict     = errors.New("订阅服务名已存在")
	ErrNotFound     = errors.New("订阅记录不存在")
)

// WriteRequest 描述新增和更新订阅时允许编辑的字段。
type WriteRequest struct {
	ServiceName string `json:"service_name"`
	Note        string `json:"note"`
	Category    string `json:"category"`
	AnnualFee   string `json:"annual_fee"`
	MonthlyFee  string `json:"monthly_fee"`
	StartsOn    string `json:"starts_on"`
	ExpiresOn   string `json:"expires_on"`
}

// Record 描述订阅原始字段及实时计算的状态。
type Record struct {
	ID              int64   `json:"id"`
	ServiceName     string  `json:"service_name"`
	Note            string  `json:"note"`
	Category        string  `json:"category"`
	AnnualFee       string  `json:"annual_fee"`
	MonthlyFee      string  `json:"monthly_fee"`
	StartsOn        *string `json:"starts_on"`
	ExpiresOn       string  `json:"expires_on"`
	CurrentStatus   string  `json:"current_status"`
	DaysUntilExpiry int     `json:"days_until_expiry"`
	CreatedBy       *string `json:"created_by"`
	CreatedAt       *string `json:"created_at"`
	UpdatedAt       *string `json:"updated_at"`
}

// SummaryMetric 描述订阅页面的一张摘要卡片。
type SummaryMetric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Status string `json:"status"`
}

// Summary 描述订阅页面摘要。
type Summary struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Metrics     []SummaryMetric `json:"metrics"`
}

type storedRecord struct {
	ID          int64
	ServiceName string
	Note        string
	Category    string
	AnnualFee   string
	MonthlyFee  string
	StartsOn    *string
	ExpiresOn   string
	CreatedBy   *string
	CreatedAt   *string
	UpdatedAt   *string
}
