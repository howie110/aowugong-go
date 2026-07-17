package monitoring

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
	"github.com/howiedata/aowugong-go/internal/config"
)

// Service 统一处理监控目标、探测、落库和页面摘要。
type Service struct {
	repository *Repository
	client     *client.MonitoringClient
	config     config.Clients
	now        func() time.Time
	location   *time.Location
}

// NewService 创建服务监控业务入口。
// 输入：repository 是应用监控仓储，monitorClient 是外部探测客户端，cfg 是目标配置。
// 输出：返回使用 Asia/Shanghai 时间的服务。
// 副作用：无。
func NewService(repository *Repository, monitorClient *client.MonitoringClient, cfg config.Clients) *Service {
	// 1. 固定业务时区并保存显式依赖。
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.Local
	}
	return &Service{repository: repository, client: monitorClient, config: cfg, now: time.Now, location: location}
}

// CheckAll 探测全部当前目标并持久化结果。
// 输入：ctx 是调用上下文。
// 输出：返回正常、异常和总目标数量。
// 副作用：调用外部 HTTP/API，并写入应用 MySQL。
func (s *Service) CheckAll(ctx context.Context) (CheckResult, error) {
	// 1. 顺序探测小型目标清单，避免同时给外部服务制造突发请求。
	targets := BuildTargets(s.config)
	results := make([]Result, 0, len(targets))
	for _, target := range targets {
		result := s.checkTarget(ctx, target)
		if err := s.repository.Insert(ctx, result); err != nil {
			return CheckResult{}, fmt.Errorf("保存监控结果: %w", err)
		}
		results = append(results, result)
	}

	// 2. 汇总 up/down 数量并返回同一批结果。
	response := CheckResult{CheckedCount: len(results), Results: results}
	for _, result := range results {
		if result.Status == "up" {
			response.UpCount++
		} else if result.Status == "down" {
			response.DownCount++
		}
	}
	return response, nil
}

// Summary 构建当前目标及最近一次结果的页面摘要。
// 输入：ctx 是调用上下文。
// 输出：返回四张指标卡和服务列表。
// 副作用：读取应用 MySQL。
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	// 1. 读取当前目标对应的最近一次结果。
	targets := BuildTargets(s.config)
	codes := make([]string, 0, len(targets))
	for _, target := range targets {
		codes = append(codes, target.Code)
	}
	latest, err := s.repository.Latest(ctx, codes)
	if err != nil {
		return Summary{}, fmt.Errorf("读取监控摘要: %w", err)
	}

	// 2. 使用当前目标元信息覆盖历史名称和地址，缺失结果显示 unknown。
	services := make([]Result, 0, len(targets))
	upCount := 0
	downCount := 0
	unknownCount := 0
	latestCheckedAt := ""
	for _, target := range targets {
		result, exists := latest[target.Code]
		if !exists {
			result = Result{
				TargetCode: target.Code, TargetName: target.Name, TargetURL: target.URL,
				Status: "unknown", ErrorMessage: target.Description,
			}
		} else {
			result.TargetName = target.Name
			result.TargetURL = target.URL
		}
		services = append(services, result)
		switch result.Status {
		case "up":
			upCount++
		case "down":
			downCount++
		default:
			unknownCount++
		}
		if result.CheckedAt != nil && *result.CheckedAt > latestCheckedAt {
			latestCheckedAt = *result.CheckedAt
		}
	}

	// 3. 格式化页面顶部指标。
	lastCheckedValue := "-"
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", latestCheckedAt, s.location); err == nil {
		lastCheckedValue = parsed.Format("01-02 15:04")
	}
	downStatus := "normal"
	if downCount != 0 {
		downStatus = "danger"
	}
	unknownDetail := "全部已有结果"
	if unknownCount != 0 {
		unknownDetail = fmt.Sprintf("未检测 %d 个", unknownCount)
	}
	return Summary{
		Title:       "监控管理",
		Description: "每天检测一次关键服务是否可访问，也可以手动立即检测。",
		Metrics: []map[string]string{
			{"label": "服务", "value": strconv.Itoa(len(services)), "detail": "监控清单", "status": "normal"},
			{"label": "正常", "value": strconv.Itoa(upCount), "detail": "最近一次可访问", "status": "normal"},
			{"label": "异常", "value": strconv.Itoa(downCount), "detail": "最近一次不可访问", "status": downStatus},
			{"label": "最后检测", "value": lastCheckedValue, "detail": unknownDetail, "status": "normal"},
		},
		Services: services,
	}, nil
}

// EnsureWeChatRSSLoginOK 单独验证 WeChatRSS 登录状态。
// 输入：ctx 是调用上下文。
// 输出：未配置目标或健康时返回 nil，异常时返回可通知错误。
// 副作用：调用 WeChatRSS 外部 HTTP API，不写数据库。
func (s *Service) EnsureWeChatRSSLoginOK(ctx context.Context) error {
	// 1. 从统一目标清单寻找 WeChatRSS。
	for _, target := range BuildTargets(s.config) {
		if target.Code != "wechat-rss" {
			continue
		}
		result := s.checkTarget(ctx, target)
		if result.Status != "up" {
			if result.ErrorMessage != nil {
				return fmt.Errorf("%s", *result.ErrorMessage)
			}
			return fmt.Errorf("WeChatRSS 登录状态异常")
		}
		return nil
	}
	return nil
}

// checkTarget 探测单个目标并返回标准结果。
// 输入：ctx 是调用上下文，target 是目标定义。
// 输出：返回 up/down 结果，不把业务异常向上抛出。
// 副作用：调用外部 HTTP/API 或只读访问 OpeniLink DB。
func (s *Service) checkTarget(ctx context.Context, target Target) Result {
	// 1. 为每个目标设置独立十秒超时，并优先选择不对外展示的内部探测地址。
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	probeURL := strings.TrimSpace(target.ProbeURL)
	if probeURL == "" {
		probeURL = target.URL
	}
	checkedAt := s.now().In(s.location).Format("2006-01-02 15:04:05")
	base := Result{
		TargetCode: target.Code, TargetName: target.Name, TargetURL: target.URL,
		Status: "down", CheckedAt: &checkedAt,
	}

	// 2. WeChatRSS 使用登录业务字段判断健康。
	if target.Code == "wechat-rss" {
		payload, httpStatus, latency, err := s.client.FetchJSON(checkCtx, probeURL)
		base.HTTPStatus = httpStatus
		base.LatencyMS = &latency
		if err != nil {
			message := "WeChatRSS 登录状态接口请求失败：" + err.Error()
			base.ErrorMessage = &message
			return base
		}
		if message := ValidateWeChatRSSLoginPayload(payload); message != "" {
			base.ErrorMessage = &message
			return base
		}
		base.Status = "up"
		return base
	}

	// 3. OpeniLink 优先读本机 SQLite，缺少文件时才做空内容 HTTP 探测。
	if target.Code == "openilink-hub" {
		return s.checkOpenILink(checkCtx, probeURL, base)
	}

	// 4. 普通服务按网络错误和 HTTP 5xx 判断健康。
	probe := s.client.ProbeURL(checkCtx, probeURL)
	base.HTTPStatus = probe.HTTPStatus
	base.LatencyMS = intValuePointer(probe.LatencyMS)
	if probe.Healthy {
		base.Status = "up"
	} else {
		base.ErrorMessage = textPointer(probe.Message)
	}
	return base
}

// checkOpenILink 静默验证 OpeniLink 是否具备发送能力。
// 输入：ctx 是调用上下文，probeURL 是内部探测地址，base 是标准结果基础字段。
// 输出：返回 up/down 结果。
// 副作用：调用空内容外部 HTTP API，不发送有效消息。
func (s *Service) checkOpenILink(ctx context.Context, probeURL string, base Result) Result {
	// 1. 必要配置缺失时直接返回可读异常。
	if s.config.OpenILink.AppToken == "" {
		base.ErrorMessage = textPointer("未配置 OPENILINK_APP_TOKEN，无法验证微信通知链路")
		return base
	}
	if s.config.OpenILink.DefaultTo == "" {
		base.ErrorMessage = textPointer("未配置 OPENILINK_DEFAULT_TO，无法验证微信通知链路")
		return base
	}

	// 2. 发空内容探测，预期请求在投递前因缺少正文被拒绝。
	probe := s.client.ProbeOpenILink(ctx, probeURL, s.config.OpenILink.AppToken)
	base.HTTPStatus = probe.HTTPStatus
	base.LatencyMS = intValuePointer(probe.LatencyMS)
	if probe.Healthy {
		base.Status = "up"
	} else {
		base.ErrorMessage = textPointer(probe.Message)
	}
	return base
}

// intValuePointer 把探测耗时转换为可空字段。
// 输入：value 是毫秒数。
// 输出：返回整数指针。
// 副作用：无。
func intValuePointer(value int) *int {
	// 1. 即使耗时为零也保留实际探测字段。
	return &value
}
