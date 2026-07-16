// Package position 提供仓位截图、账户资产和持仓明细业务。
package position

// Snapshot 描述单个账户在一个日期的资产快照。
type Snapshot struct {
	ID                int64     `json:"id"`
	SnapshotDate      string    `json:"snapshot_date"`
	BrokerName        string    `json:"broker_name"`
	SourceApp         string    `json:"source_app"`
	AccountSuffix     string    `json:"account_suffix"`
	AccountAlias      string    `json:"account_alias,omitempty"`
	TotalAsset        float64   `json:"total_asset"`
	MarketValue       float64   `json:"market_value"`
	AvailableCash     float64   `json:"available_cash"`
	OtherAmount       float64   `json:"other_amount"`
	PositionPercent   *float64  `json:"position_percent,omitempty"`
	ImagePath         string    `json:"image_path,omitempty"`
	ImageSHA256       string    `json:"image_sha256,omitempty"`
	OCRProvider       string    `json:"ocr_provider,omitempty"`
	ProviderRequestID string    `json:"provider_request_id,omitempty"`
	Warnings          []string  `json:"warnings"`
	Holdings          []Holding `json:"holdings"`
	HoldingsParsed    bool      `json:"holdings_parsed"`
	CreatedBy         string    `json:"created_by,omitempty"`
	CreatedAt         string    `json:"created_at,omitempty"`
	UpdatedAt         string    `json:"updated_at,omitempty"`
}

// Holding 描述单个证券在账户快照中的持仓明细。
type Holding struct {
	ID                int64    `json:"id,omitempty"`
	SnapshotDate      string   `json:"snapshot_date,omitempty"`
	BrokerName        string   `json:"broker_name,omitempty"`
	SourceApp         string   `json:"source_app,omitempty"`
	AccountSuffix     string   `json:"account_suffix,omitempty"`
	AccountAlias      string   `json:"account_alias,omitempty"`
	SecurityName      string   `json:"security_name"`
	SecurityCode      string   `json:"security_code,omitempty"`
	MarketValue       float64  `json:"market_value"`
	Quantity          *float64 `json:"quantity,omitempty"`
	AvailableQuantity *float64 `json:"available_quantity,omitempty"`
	ProfitAmount      *float64 `json:"profit_amount,omitempty"`
	ProfitPercent     *float64 `json:"profit_percent,omitempty"`
	CostPrice         *float64 `json:"cost_price,omitempty"`
	CurrentPrice      *float64 `json:"current_price,omitempty"`
}

// UploadResult 描述单张仓位截图的处理结果。
type UploadResult struct {
	Filename string         `json:"filename"`
	Status   string         `json:"status"`
	Snapshot *Snapshot      `json:"snapshot,omitempty"`
	Error    string         `json:"error,omitempty"`
	OCRText  string         `json:"ocr_text,omitempty"`
	RawOCR   map[string]any `json:"raw_ocr,omitempty"`
}

// UploadResponse 描述一批仓位截图的处理结果。
type UploadResponse struct {
	SnapshotDate string         `json:"snapshot_date"`
	Results      []UploadResult `json:"results"`
}
