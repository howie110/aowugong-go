package subscription

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/money"
)

const dateLayout = "2006-01-02"

// Service 统一处理订阅清洗、派生状态、费用摘要和到期筛选。
type Service struct {
	repository *Repository
	today      func() time.Time
	location   *time.Location
}

// NewService 创建订阅服务。
// 输入：repository 是订阅 SQLite 仓储。
// 输出：返回使用 Asia/Shanghai 日期口径的服务。
// 副作用：无。
func NewService(repository *Repository) *Service {
	// 1. 固定业务日期时区，加载失败时使用本地时区兜底。
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.Local
	}
	return &Service{repository: repository, today: time.Now, location: location}
}

// List 返回订阅记录并实时补充状态和离到期天数。
// 输入：ctx 是调用上下文。
// 输出：返回按到期日排序的记录。
// 副作用：空表时写入默认记录，随后读取 SQLite。
func (s *Service) List(ctx context.Context) ([]Record, error) {
	// 1. 保持旧项目空表初始化行为。
	if err := s.repository.SeedDefaults(ctx); err != nil {
		return nil, fmt.Errorf("初始化订阅记录: %w", err)
	}
	rows, err := s.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取订阅记录: %w", err)
	}

	// 2. 使用同一业务日期转换全部派生字段。
	baseDate := s.businessDate()
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		record, err := s.toRecord(row, baseDate)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// Create 清洗并新增订阅记录。
// 输入：ctx 是调用上下文，request 是页面字段，createdBy 是当前用户名。
// 输出：返回带派生状态的新记录。
// 副作用：写入并读取 SQLite。
func (s *Service) Create(ctx context.Context, request WriteRequest, createdBy string) (Record, error) {
	// 1. 清洗文本、金额和日期。
	normalized, err := s.normalizeRequest(request)
	if err != nil {
		return Record{}, err
	}

	// 2. 写入仓储并转换派生字段。
	row, err := s.repository.Create(ctx, normalized, createdBy)
	if err != nil {
		return Record{}, fmt.Errorf("创建订阅: %w", err)
	}
	return s.toRecord(row, s.businessDate())
}

// Update 清洗并全量更新订阅记录。
// 输入：ctx 是调用上下文，recordID 是主键，request 是页面字段。
// 输出：返回带派生状态的更新记录。
// 副作用：写入并读取 SQLite。
func (s *Service) Update(ctx context.Context, recordID int64, request WriteRequest) (Record, error) {
	// 1. 校验主键并清洗全部可编辑字段。
	if recordID <= 0 {
		return Record{}, ErrInvalidInput
	}
	normalized, err := s.normalizeRequest(request)
	if err != nil {
		return Record{}, err
	}

	// 2. 更新仓储并转换派生字段。
	row, err := s.repository.Update(ctx, recordID, normalized)
	if err != nil {
		return Record{}, fmt.Errorf("更新订阅: %w", err)
	}
	return s.toRecord(row, s.businessDate())
}

// Delete 删除指定订阅记录。
// 输入：ctx 是调用上下文，recordID 是主键。
// 输出：成功删除返回 true。
// 副作用：写入 SQLite。
func (s *Service) Delete(ctx context.Context, recordID int64) (bool, error) {
	// 1. 拒绝无效主键并调用唯一删除入口。
	if recordID <= 0 {
		return false, ErrInvalidInput
	}
	deleted, err := s.repository.Delete(ctx, recordID)
	if err != nil {
		return false, fmt.Errorf("删除订阅: %w", err)
	}
	return deleted, nil
}

// Summary 生成订阅页面摘要。
// 输入：ctx 是调用上下文。
// 输出：返回总数、有效数、近期到期数和费用合计。
// 副作用：读取 SQLite，空表时写入默认记录。
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	// 1. 复用列表口径统计状态和费用。
	records, err := s.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	activeCount := 0
	expiredCount := 0
	upcomingCount := 0
	annualCents := int64(0)
	monthlyCents := int64(0)
	for _, record := range records {
		if record.CurrentStatus == "订阅中" {
			activeCount++
			if record.DaysUntilExpiry <= 30 {
				upcomingCount++
			}
			annual, _ := money.ParseCents(record.AnnualFee)
			monthly, _ := money.ParseCents(record.MonthlyFee)
			annualCents += annual
			monthlyCents += monthly
		} else {
			expiredCount++
		}
	}

	// 2. 构建与现有 React 页面一致的四张摘要卡片。
	upcomingStatus := "normal"
	if upcomingCount != 0 {
		upcomingStatus = "warning"
	}
	return Summary{
		Title:       "订阅管理",
		Description: "记录域名、云服务和生活会员的费用与到期日。",
		Metrics: []SummaryMetric{
			{Label: "订阅总数", Value: strconv.Itoa(len(records)), Detail: "全部订阅记录", Status: "normal"},
			{Label: "订阅中", Value: strconv.Itoa(activeCount), Detail: fmt.Sprintf("已结束 %d 项", expiredCount), Status: "normal"},
			{Label: "30 天内到期", Value: strconv.Itoa(upcomingCount), Detail: "按到期日动态计算", Status: upcomingStatus},
			{Label: "年费合计", Value: money.FormatCents(annualCents, true), Detail: "月费约 " + money.FormatCents(monthlyCents, true), Status: "normal"},
		},
	}, nil
}

// ListExpiring 返回正好在指定天数后到期的有效订阅。
// 输入：ctx 是调用上下文，reminderDays 是提前提醒天数。
// 输出：返回匹配记录。
// 副作用：读取 SQLite，空表时写入默认记录。
func (s *Service) ListExpiring(ctx context.Context, reminderDays int) ([]Record, error) {
	// 1. 复用列表中的实时状态和天数计算。
	records, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	expiring := make([]Record, 0)
	for _, record := range records {
		if record.CurrentStatus == "订阅中" && record.DaysUntilExpiry == reminderDays {
			expiring = append(expiring, record)
		}
	}
	return expiring, nil
}

// normalizeRequest 统一清洗订阅文本、金额和日期。
// 输入：request 是页面提交字段。
// 输出：返回可持久化字段；无效时返回 ErrInvalidInput。
// 副作用：无。
func (s *Service) normalizeRequest(request WriteRequest) (WriteRequest, error) {
	// 1. 清理必填文本并应用默认分类。
	request.ServiceName = strings.TrimSpace(request.ServiceName)
	request.Note = strings.TrimSpace(request.Note)
	request.Category = strings.TrimSpace(request.Category)
	if request.Category == "" {
		request.Category = "生活"
	}
	if request.ServiceName == "" || len([]rune(request.ServiceName)) > 120 || len([]rune(request.Category)) > 20 {
		return WriteRequest{}, ErrInvalidInput
	}

	// 2. 使用十进制有理数按 ROUND_HALF_UP 规范化金额。
	annualCents, err := money.ParseCents(request.AnnualFee)
	if err != nil || annualCents < 0 {
		return WriteRequest{}, ErrInvalidInput
	}
	monthlyCents, err := money.ParseCents(request.MonthlyFee)
	if err != nil || monthlyCents < 0 {
		return WriteRequest{}, ErrInvalidInput
	}
	request.AnnualFee = money.FormatCents(annualCents, false)
	request.MonthlyFee = money.FormatCents(monthlyCents, false)

	// 3. 校验 ISO 日期并保留可选开始日期。
	if _, err := time.ParseInLocation(dateLayout, strings.TrimSpace(request.ExpiresOn), s.location); err != nil {
		return WriteRequest{}, ErrInvalidInput
	}
	request.ExpiresOn = strings.TrimSpace(request.ExpiresOn)
	request.StartsOn = strings.TrimSpace(request.StartsOn)
	if request.StartsOn != "" {
		if _, err := time.ParseInLocation(dateLayout, request.StartsOn, s.location); err != nil {
			return WriteRequest{}, ErrInvalidInput
		}
	}
	return request, nil
}

// toRecord 把数据库记录转换为 API 记录并补充动态状态。
// 输入：row 是数据库原始记录，baseDate 是业务当天零点。
// 输出：返回页面记录。
// 副作用：无。
func (s *Service) toRecord(row storedRecord, baseDate time.Time) (Record, error) {
	// 1. 解析到期日并计算日历天差。
	expiresOn, err := time.ParseInLocation(dateLayout, row.ExpiresOn, s.location)
	if err != nil {
		return Record{}, fmt.Errorf("解析订阅 %d 到期日: %w", row.ID, err)
	}
	days := int(expiresOn.Sub(baseDate).Hours() / 24)
	status := "订阅中"
	if days < 0 {
		status = "已结束"
	}

	// 2. 规范化数据库可能省略小数位的金额文本。
	annualCents, err := money.ParseCents(row.AnnualFee)
	if err != nil {
		return Record{}, fmt.Errorf("解析订阅 %d 年费: %w", row.ID, err)
	}
	monthlyCents, err := money.ParseCents(row.MonthlyFee)
	if err != nil {
		return Record{}, fmt.Errorf("解析订阅 %d 月费: %w", row.ID, err)
	}
	return Record{
		ID:              row.ID,
		ServiceName:     row.ServiceName,
		Note:            row.Note,
		Category:        row.Category,
		AnnualFee:       money.FormatCents(annualCents, false),
		MonthlyFee:      money.FormatCents(monthlyCents, false),
		StartsOn:        row.StartsOn,
		ExpiresOn:       row.ExpiresOn,
		CurrentStatus:   status,
		DaysUntilExpiry: days,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

// businessDate 返回 Asia/Shanghai 业务当天零点。
// 输入：无。
// 输出：返回当前业务日期。
// 副作用：读取系统时钟。
func (s *Service) businessDate() time.Time {
	// 1. 转换时区并截断到日历日零点。
	now := s.today().In(s.location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
}
