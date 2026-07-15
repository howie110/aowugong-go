package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/config"
)

const dailyFields = "ts_code,trade_date,open,high,low,close,pre_close,change,pct_chg,vol,amount"

// TushareClient 使用官方 HTTP 协议访问 Tushare Pro，不依赖 Python SDK。
type TushareClient struct {
	config     config.Tushare
	httpClient *http.Client
}

// DailyRow 描述股票或基金的一条日线行情。
type DailyRow struct {
	TSCode    string  `json:"ts_code"`
	TradeDate string  `json:"trade_date"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	PreClose  float64 `json:"pre_close"`
	Change    float64 `json:"change"`
	PctChange float64 `json:"pct_chg"`
	Volume    float64 `json:"vol"`
	Amount    float64 `json:"amount"`
}

// TushareTable 描述 Tushare 通用二维表响应。
type TushareTable struct {
	Fields []string
	Items  [][]json.RawMessage
}

type tushareRequest struct {
	APIName string         `json:"api_name"`
	Token   string         `json:"token"`
	Params  map[string]any `json:"params"`
	Fields  string         `json:"fields,omitempty"`
}

type tushareResponse struct {
	Code int           `json:"code"`
	Msg  any           `json:"msg"`
	Data *TushareTable `json:"data"`
}

// NewTushareClient 创建原生 HTTP Tushare 客户端。
// 输入：cfg 提供基础地址和 Token，httpClient 提供超时与连接复用。
// 输出：返回可并发复用的客户端。
// 副作用：无，不发起网络请求。
func NewTushareClient(cfg config.Tushare, httpClient *http.Client) *TushareClient {
	// 1. 应用默认超时并规范化基础地址。
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &TushareClient{config: cfg, httpClient: httpClient}
}

// Configured 返回 Tushare Token 和基础地址是否均已配置。
// 输入：无。
// 输出：配置完整时返回 true。
// 副作用：无。
func (c *TushareClient) Configured() bool {
	// 1. 仅返回配置状态，不暴露 Token。
	return c != nil && c.config.BaseURL != "" && strings.TrimSpace(c.config.Token) != ""
}

// Daily 拉取指定日期范围的股票日线行情。
// 输入：ctx 控制请求，startDate 和 endDate 接受 YYYY-MM-DD 或 YYYYMMDD。
// 输出：返回结构化日线记录；上游或数据格式错误时返回错误。
// 副作用：调用 Tushare 外部接口。
func (c *TushareClient) Daily(ctx context.Context, startDate, endDate string) ([]DailyRow, error) {
	// 1. 使用官方 daily 接口请求固定字段。
	table, err := c.FetchTable(ctx, "daily", map[string]any{
		"start_date": compactDate(startDate),
		"end_date":   compactDate(endDate),
	}, dailyFields)
	if err != nil {
		return nil, fmt.Errorf("拉取 Tushare 股票日线: %w", err)
	}

	// 2. 将二维表转换为强类型记录。
	rows, err := decodeDailyRows(table)
	if err != nil {
		return nil, fmt.Errorf("解析 Tushare 股票日线: %w", err)
	}
	return rows, nil
}

// FundDaily 拉取指定基金和日期范围的日线行情。
// 输入：ctx 控制请求，tsCode 是基金代码，startDate 和 endDate 是日期。
// 输出：返回结构化基金日线记录；失败时返回错误。
// 副作用：调用 Tushare 外部接口。
func (c *TushareClient) FundDaily(ctx context.Context, tsCode, startDate, endDate string) ([]DailyRow, error) {
	// 1. 使用官方 fund_daily 接口请求固定字段。
	table, err := c.FetchTable(ctx, "fund_daily", map[string]any{
		"ts_code":    strings.TrimSpace(tsCode),
		"start_date": compactDate(startDate),
		"end_date":   compactDate(endDate),
	}, dailyFields)
	if err != nil {
		return nil, fmt.Errorf("拉取 Tushare 基金日线: %w", err)
	}

	// 2. 复用唯一日线转换逻辑。
	rows, err := decodeDailyRows(table)
	if err != nil {
		return nil, fmt.Errorf("解析 Tushare 基金日线: %w", err)
	}
	return rows, nil
}

// FetchTable 调用任意当前业务可达的 Tushare 表格接口。
// 输入：ctx 控制请求，apiName 是接口名，params 是参数，fields 是字段列表。
// 输出：返回字段和二维原始值；配置、HTTP 或业务错误时返回错误。
// 副作用：调用 Tushare 外部接口。
func (c *TushareClient) FetchTable(ctx context.Context, apiName string, params map[string]any, fields string) (TushareTable, error) {
	// 1. 校验配置与接口名并序列化官方请求体。
	if !c.Configured() {
		return TushareTable{}, fmt.Errorf("未配置 TUSHARE_TOKEN 或 TUSHARE_BASE_URL")
	}
	if strings.TrimSpace(apiName) == "" {
		return TushareTable{}, fmt.Errorf("Tushare 接口名不能为空")
	}
	payload, err := json.Marshal(tushareRequest{
		APIName: strings.TrimSpace(apiName), Token: c.config.Token, Params: params, Fields: fields,
	})
	if err != nil {
		return TushareTable{}, fmt.Errorf("序列化 Tushare 请求: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return TushareTable{}, fmt.Errorf("创建 Tushare 请求: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	// 2. 执行请求并限制响应体大小。
	response, err := c.httpClient.Do(request)
	if err != nil {
		return TushareTable{}, fmt.Errorf("请求 Tushare: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return TushareTable{}, fmt.Errorf("读取 Tushare 响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return TushareTable{}, fmt.Errorf("Tushare 接口返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	// 3. 解码并区分业务错误、空表和有效表格。
	var result tushareResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return TushareTable{}, fmt.Errorf("Tushare 接口返回非 JSON: %w", err)
	}
	if result.Code != 0 {
		return TushareTable{}, fmt.Errorf("Tushare 业务错误 %d: %v", result.Code, result.Msg)
	}
	if result.Data == nil {
		return TushareTable{Fields: []string{}, Items: [][]json.RawMessage{}}, nil
	}
	return *result.Data, nil
}

// decodeDailyRows 把字段可变顺序的 Tushare 表格转换为日线记录。
// 输入：table 包含字段名和原始 JSON 单元格。
// 输出：返回结构化日线；字段缺失或单元格无效时返回错误。
// 副作用：无。
func decodeDailyRows(table TushareTable) ([]DailyRow, error) {
	// 1. 建立字段名到列号的映射并检查必需字段。
	indexes := make(map[string]int, len(table.Fields))
	for index, field := range table.Fields {
		indexes[field] = index
	}
	required := strings.Split(dailyFields, ",")
	for _, field := range required {
		if _, exists := indexes[field]; !exists {
			return nil, fmt.Errorf("响应缺少字段 %s", field)
		}
	}

	// 2. 按字段名读取每一行，避免依赖上游列顺序。
	rows := make([]DailyRow, 0, len(table.Items))
	for rowIndex, item := range table.Items {
		if len(item) < len(table.Fields) {
			return nil, fmt.Errorf("第 %d 行列数不足", rowIndex+1)
		}
		tsCode, err := rawString(item[indexes["ts_code"]])
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 ts_code: %w", rowIndex+1, err)
		}
		tradeDate, err := rawString(item[indexes["trade_date"]])
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 trade_date: %w", rowIndex+1, err)
		}
		values := make(map[string]float64, 9)
		for _, field := range required[2:] {
			value, valueErr := rawFloat(item[indexes[field]])
			if valueErr != nil {
				return nil, fmt.Errorf("第 %d 行 %s: %w", rowIndex+1, field, valueErr)
			}
			values[field] = value
		}
		rows = append(rows, DailyRow{
			TSCode: tsCode, TradeDate: displayDate(tradeDate), Open: values["open"],
			High: values["high"], Low: values["low"], Close: values["close"],
			PreClose: values["pre_close"], Change: values["change"],
			PctChange: values["pct_chg"], Volume: values["vol"], Amount: values["amount"],
		})
	}
	return rows, nil
}

// rawString 解码 Tushare 字符串单元格。
// 输入：value 是原始 JSON 单元格。
// 输出：返回字符串；类型不匹配时返回错误。
// 副作用：无。
func rawString(value json.RawMessage) (string, error) {
	// 1. 按 JSON 字符串解码并清理首尾空白。
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", fmt.Errorf("不是字符串: %w", err)
	}
	return strings.TrimSpace(result), nil
}

// rawFloat 解码 Tushare 数字或数字字符串单元格。
// 输入：value 是原始 JSON 单元格。
// 输出：返回浮点值；空值返回零，格式无效返回错误。
// 副作用：无。
func rawFloat(value json.RawMessage) (float64, error) {
	// 1. 把 null 和空文本视为旧 DataFrame 中的空数值零值。
	text := strings.TrimSpace(string(value))
	if text == "" || text == "null" || text == `""` {
		return 0, nil
	}
	text = strings.Trim(text, `"`)
	result, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("不是数字: %w", err)
	}
	return result, nil
}

// compactDate 把展示日期转换为 Tushare 使用的 YYYYMMDD。
// 输入：value 是带或不带连字符的日期。
// 输出：返回删除连字符和空白后的日期。
// 副作用：无。
func compactDate(value string) string {
	// 1. 清理空白并删除日期分隔符。
	return strings.ReplaceAll(strings.TrimSpace(value), "-", "")
}

// displayDate 把 Tushare 日期转换为 YYYY-MM-DD 展示和存储格式。
// 输入：value 通常是 YYYYMMDD。
// 输出：八位日期返回带连字符格式，其他格式原样返回。
// 副作用：无。
func displayDate(value string) string {
	// 1. 仅转换确定的八位数字格式。
	value = strings.TrimSpace(value)
	if len(value) == 8 {
		return value[:4] + "-" + value[4:6] + "-" + value[6:]
	}
	return value
}
