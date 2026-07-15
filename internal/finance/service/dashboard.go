// Package service 提供 finance 页面和任务复用的业务入口。
package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"
)

const unknownLatest = "未知"

// DashboardOptions 描述 finance 摘要所需的运行时开关，不包含任何密钥明文。
type DashboardOptions struct {
	HTTPAddress         string
	OpenILinkConfigured bool
	SchedulerEnabled    bool
	RealTradeEnabled    bool
	QMTConfigured       bool
	BinanceConfigured   bool
	OKXConfigured       bool
}

// DashboardService 汇总 SQLite 数据进度和当前 Go 运行时状态。
type DashboardService struct {
	db      *sql.DB
	options DashboardOptions
}

// Metric 描述控制台顶部状态指标。
type Metric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status,omitempty"`
}

// Item 描述 finance 页面可复用的列表项。
type Item struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Latest      string `json:"latest,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	Status      string `json:"status,omitempty"`
	Value       string `json:"value,omitempty"`
	Command     string `json:"command,omitempty"`
	Entry       string `json:"entry,omitempty"`
	DateColumn  string `json:"date_column,omitempty"`
}

// Overview 描述 finance 控制台响应。
type Overview struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Metrics      []Metric `json:"metrics"`
	Modules      []Item   `json:"modules"`
	DataProgress []Item   `json:"data_progress"`
}

// PageSummary 描述通用模块摘要响应。
type PageSummary struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Modules     []Item `json:"modules,omitempty"`
	Items       []Item `json:"items,omitempty"`
}

// DataPage 描述行情与基础数据进度响应。
type DataPage struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Tables      []Item `json:"tables"`
	Sources     []Item `json:"sources"`
}

// JobsPage 描述内嵌定时任务摘要响应。
type JobsPage struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Jobs        []Item `json:"jobs"`
	Runner      string `json:"runner"`
	FailNotify  string `json:"fail_notify"`
}

// TradingPage 描述真实交易保护状态响应。
type TradingPage struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Guards      []Item `json:"guards"`
	Modules     []Item `json:"modules"`
}

// NotificationsPage 描述统一通知渠道状态响应。
type NotificationsPage struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Channels      []Item `json:"channels"`
	ReceiverCount int    `json:"receiver_count"`
}

type progressQuery struct {
	name        string
	dateColumn  string
	description string
	query       string
}

var progressQueries = []progressQuery{
	{name: "tushare_trade_cal", dateColumn: "cal_date", description: "交易日历", query: "SELECT MAX(cal_date) FROM tushare_trade_cal"},
	{name: "tushare_daily", dateColumn: "trade_date", description: "股票日线", query: "SELECT MAX(trade_date) FROM tushare_daily"},
	{name: "basic_operation", dateColumn: "trade_date", description: "策略操作记录", query: "SELECT MAX(trade_date) FROM basic_operation"},
	{name: "tushare_stock_basic", dateColumn: "update_date", description: "股票基础信息", query: "SELECT MAX(update_date) FROM tushare_stock_basic"},
	{name: "tushare_etf_basic", dateColumn: "update_date", description: "ETF 基础信息", query: "SELECT MAX(update_date) FROM tushare_etf_basic"},
}

// NewDashboardService 创建 finance 摘要服务。
// 输入：db 是 SQLite 连接，options 是不含密钥的运行时状态。
// 输出：返回可并发复用的摘要服务。
// 副作用：无，不访问数据库和外部接口。
func NewDashboardService(db *sql.DB, options DashboardOptions) *DashboardService {
	// 1. 保存显式依赖供各摘要入口复用。
	return &DashboardService{db: db, options: options}
}

// Overview 汇总控制台指标、模块和两项核心数据进度。
// 输入：ctx 控制 SQLite 查询生命周期。
// 输出：返回控制台完整响应；查询失败时返回带业务上下文的错误。
// 副作用：只读 SQLite。
func (s *DashboardService) Overview(ctx context.Context) (Overview, error) {
	// 1. 查询全部数据进度并截取控制台关心的两项。
	progress, err := s.loadProgress(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("读取控制台数据进度: %w", err)
	}

	// 2. 根据当前 Go 服务和 OpeniLink 配置构建运行指标。
	notifyValue, notifyStatus := configuredState(s.options.OpenILinkConfigured)
	result := Overview{
		Title:       "控制台",
		Description: "集中查看投资研究、内容服务、定时任务和系统运维状态。",
		Metrics: []Metric{
			{Label: "Web 服务", Value: displayPort(s.options.HTTPAddress), Detail: "Go + React", Status: "normal"},
			{Label: "文章同步", Value: "08:00 / 20:00", Detail: "RSS + DeepSeek", Status: "normal"},
			{Label: "服务监控", Value: "08:30", Detail: "项目连通性检测", Status: "normal"},
			{Label: "通知通道", Value: notifyValue, Detail: "OpeniLink 微信 Bot", Status: notifyStatus},
		},
		Modules: moduleList(),
	}
	if len(progress) >= 2 {
		result.DataProgress = progress[:2]
	}
	return result, nil
}

// BacktestSummary 返回纯 Go 回测模块和三个统一入口的页面摘要。
// 输入：无。
// 输出：返回股票、ETF 和 Web3 回测能力说明。
// 副作用：无。
func (s *DashboardService) BacktestSummary() PageSummary {
	// 1. 返回与纯计算回测包一致的前端展示模型。
	return PageSummary{
		Title:       "回测",
		Description: "策略回测由纯 Go 计算引擎执行，统一返回收益、回撤和交易统计。",
		Modules: []Item{
			{Name: "engine", Description: "只处理回测执行流程和持仓变化"},
			{Name: "metrics", Description: "只处理收益、回撤和胜率等统计"},
			{Name: "commission", Description: "只处理不同市场的手续费"},
			{Name: "service", Description: "供 API、定时任务和 CLI 复用的统一入口"},
		},
		Items: []Item{
			{Name: "股票回测", Entry: "RunStockBacktest", Status: "ready"},
			{Name: "ETF 回测", Entry: "RunETFBacktest", Status: "ready"},
			{Name: "Web3 回测", Entry: "RunWeb3Backtest", Status: "ready"},
		},
	}
}

// DataSummary 读取核心 SQLite 表的最新业务日期。
// 输入：ctx 控制 SQLite 查询生命周期。
// 输出：返回五张核心表的进度和当前数据源；失败时返回错误。
// 副作用：只读 SQLite。
func (s *DashboardService) DataSummary(ctx context.Context) (DataPage, error) {
	// 1. 使用固定白名单 SQL 读取全部核心表进度。
	tables, err := s.loadProgress(ctx)
	if err != nil {
		return DataPage{}, fmt.Errorf("读取行情数据进度: %w", err)
	}

	// 2. 返回 SQLite 与 Tushare 的最终运行时说明。
	return DataPage{
		Title:       "数据",
		Description: "SQLite 作为本地数据底座，内嵌任务负责补齐行情和交易日历。",
		Tables:      tables,
		Sources: []Item{
			{Name: "Tushare", Description: "股票、ETF、交易日历"},
			{Name: "SQLite", Description: "本地持久化，避免每次实时请求"},
		},
	}, nil
}

// JobsSummary 返回 Go 进程内注册的固定任务频率。
// 输入：无。
// 输出：返回七项任务、统一执行入口和失败通知状态。
// 副作用：无。
func (s *DashboardService) JobsSummary() JobsPage {
	// 1. 根据调度器和 OpeniLink 配置生成运行状态。
	jobStatus := "disabled"
	if s.options.SchedulerEnabled {
		jobStatus = "active"
	}
	failNotify, _ := configuredState(s.options.OpenILinkConfigured)

	// 2. 返回与任务注册表保持一致的任务清单。
	return JobsPage{
		Title:       "定时任务",
		Description: "任务由 Go 进程内调度器按 Asia/Shanghai 时区统一执行。",
		Jobs: []Item{
			{Name: "test_crontab", Schedule: "0 9 * * *", Description: "每日任务链路测试", Command: "scheduler.Run(test_crontab)", Status: jobStatus},
			{Name: "update_tushare_daily_data", Schedule: "0 20 * * *", Description: "更新 Tushare 日线数据", Command: "scheduler.Run(update_tushare_daily_data)", Status: jobStatus},
			{Name: "sync_investment_articles", Schedule: "0 8,20 * * *", Description: "同步并分析投资文章", Command: "scheduler.Run(sync_investment_articles)", Status: jobStatus},
			{Name: "check_service_monitors", Schedule: "30 8 * * *", Description: "检查服务连通性", Command: "scheduler.Run(check_service_monitors)", Status: jobStatus},
			{Name: "check_subscription_expiry_notify", Schedule: "30 9 * * *", Description: "检查订阅到期并提醒", Command: "scheduler.Run(check_subscription_expiry_notify)", Status: jobStatus},
			{Name: "openilink_reply_reminder", Schedule: "0 10 * * *", Description: "检查 OpeniLink 待回复消息", Command: "scheduler.Run(openilink_reply_reminder)", Status: jobStatus},
			{Name: "backup_sqlite", Schedule: "30 3 * * *", Description: "安全快照备份 SQLite", Command: "scheduler.Run(backup_sqlite)", Status: jobStatus},
		},
		Runner:     "internal/scheduler.Registry.Run",
		FailNotify: failNotify + "微信",
	}
}

// TradingSummary 返回真实交易总开关和各交易端配置状态。
// 输入：无。
// 输出：返回交易保护与模块边界说明。
// 副作用：无，不发起真实下单。
func (s *DashboardService) TradingSummary() TradingPage {
	// 1. 真实交易默认以安全关闭状态展示。
	realValue, realStatus := "关闭", "safe"
	if s.options.RealTradeEnabled {
		realValue, realStatus = "开启", "danger"
	}

	// 2. 只显示各交易端是否配置，不暴露凭据。
	return TradingPage{
		Title:       "交易",
		Description: "实盘交易和回测隔离，真实下单必须显式开启总开关。",
		Guards: []Item{
			{Name: "真实交易总开关", Value: realValue, Status: realStatus},
			{Name: "QMT 配置", Value: configuredValue(s.options.QMTConfigured), Status: "normal"},
			{Name: "Binance 配置", Value: configuredValue(s.options.BinanceConfigured), Status: "normal"},
			{Name: "OKX 配置", Value: configuredValue(s.options.OKXConfigured), Status: "normal"},
		},
		Modules: []Item{
			{Name: "trading", Description: "A 股下单和订单状态检查"},
			{Name: "web3", Description: "Web3 交易入口与交易端适配"},
			{Name: "guard", Description: "统一校验 FINANCE_ENABLE_REAL_TRADE"},
		},
	}
}

// NotificationsSummary 返回统一 OpeniLink 通知渠道状态。
// 输入：无。
// 输出：返回渠道配置状态和接收方数量。
// 副作用：无，不发送通知。
func (s *DashboardService) NotificationsSummary() NotificationsPage {
	// 1. 只保留项目最终使用的 OpeniLink 微信通道。
	value := configuredValue(s.options.OpenILinkConfigured)
	receiverCount := 0
	if s.options.OpenILinkConfigured {
		receiverCount = 1
	}
	return NotificationsPage{
		Title:         "通知",
		Description:   "OpeniLink 统一发送任务失败提醒和业务消息。",
		Channels:      []Item{{Name: "微信", Value: value, Description: "OpeniLink Hub 通知"}},
		ReceiverCount: receiverCount,
	}
}

// loadProgress 查询五张白名单表的最新日期。
// 输入：ctx 控制数据库操作。
// 输出：按页面固定顺序返回表进度；任一查询失败时返回错误。
// 副作用：只读 SQLite。
func (s *DashboardService) loadProgress(ctx context.Context) ([]Item, error) {
	// 1. 逐条执行源码内固定的查询，避免动态表名进入 SQL。
	items := make([]Item, 0, len(progressQueries))
	for _, definition := range progressQueries {
		var latest sql.NullString
		if err := s.db.QueryRowContext(ctx, definition.query).Scan(&latest); err != nil {
			return nil, fmt.Errorf("查询 %s 最新日期: %w", definition.name, err)
		}
		value := unknownLatest
		if latest.Valid && strings.TrimSpace(latest.String) != "" {
			value = latest.String
		}
		items = append(items, Item{
			Name:        definition.name,
			DateColumn:  definition.dateColumn,
			Description: definition.description,
			Latest:      value,
		})
	}
	return items, nil
}

// moduleList 返回控制台当前可达模块说明。
// 输入：无。
// 输出：返回前端展示的模块清单。
// 副作用：无。
func moduleList() []Item {
	// 1. 按用户工作流分组返回稳定展示数据。
	return []Item{
		{Name: "投资研究", Description: "投资文章分析、文章抓取、仓位分析和仓位导入", Status: "4 个页面"},
		{Name: "量化工具", Description: "回测、数据和交易能力入口", Status: "3 个页面"},
		{Name: "内容服务", Description: "微信读书和工作导航", Status: "2 个页面"},
		{Name: "系统运维", Description: "监控、定时任务、通知和权限管理", Status: "4 个页面"},
		{Name: "外部联动", Description: "RSS、DeepSeek、OpeniLink 和阿里 OCR", Status: "已接入"},
	}
}

// displayPort 从监听地址提取前端展示端口。
// 输入：address 是 host:port 形式的监听地址。
// 输出：解析成功时返回端口，否则返回原地址。
// 副作用：无。
func displayPort(address string) string {
	// 1. 使用标准库解析监听地址并兼容测试中的简写值。
	_, port, err := net.SplitHostPort(address)
	if err == nil && port != "" {
		return port
	}
	return address
}

// configuredValue 将配置布尔值转换为中文展示值。
// 输入：configured 表示配置是否完整。
// 输出：返回“已配置”或“未配置”。
// 副作用：无。
func configuredValue(configured bool) string {
	// 1. 映射为前端沿用的中文状态。
	if configured {
		return "已配置"
	}
	return "未配置"
}

// configuredState 将配置布尔值转换为中文值和状态色。
// 输入：configured 表示配置是否完整。
// 输出：返回展示值及 normal 或 danger 状态。
// 副作用：无。
func configuredState(configured bool) (string, string) {
	// 1. 缺失配置使用危险状态提示。
	if configured {
		return "已配置", "normal"
	}
	return "未配置", "danger"
}
