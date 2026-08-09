package mahjong

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/howiedata/aowugong-go/internal/money"
	"github.com/xuri/excelize/v2"
)

const dateLayout = "2006-01-02"

// Service 统一处理麻将录入、Excel 解析和纯统计计算。
type Service struct {
	repository *Repository
}

// NewService 创建麻将服务。
// 输入：repository 是麻将 PostgreSQL 仓储。
// 输出：返回可供 HTTP 和任务复用的服务。
// 副作用：无。
func NewService(repository *Repository) *Service {
	// 1. 保存应用层显式注入的仓储。
	return &Service{repository: repository}
}

// Save 保存页面录入的单日麻将战绩。
// 输入：ctx 是调用上下文，request 是日期和金额，createdBy 是当前用户名。
// 输出：返回 inserted、updated 或 unchanged 及最终记录。
// 副作用：写入并读取 PostgreSQL。
func (s *Service) Save(ctx context.Context, request WriteRequest, createdBy string) (WriteResponse, error) {
	// 1. 校验日期并按统一金额规则转换为分。
	record, err := parseWriteRequest(request)
	if err != nil {
		return WriteResponse{}, err
	}

	// 2. 复用批量 upsert 并映射写入状态。
	stats, err := s.repository.Upsert(ctx, []parsedRecord{record}, "页面录入", createdBy)
	if err != nil {
		return WriteResponse{}, fmt.Errorf("保存麻将战绩: %w", err)
	}
	status := "unchanged"
	if stats.insertedCount != 0 {
		status = "inserted"
	} else if stats.updatedCount != 0 {
		status = "updated"
	}
	if stats.latestRecord == nil {
		return WriteResponse{}, fmt.Errorf("保存麻将战绩后缺少记录")
	}
	return WriteResponse{Status: status, Record: stats.latestRecord.Record}, nil
}

// ImportExcel 解析第一个工作表并按日期批量覆盖战绩。
// 输入：ctx 是调用上下文，content 是 xlsx 内容，filename 和 createdBy 是来源信息。
// 输出：返回解析、插入、更新和跳过数量。
// 副作用：读取 Excel 内存内容并写入 PostgreSQL。
func (s *Service) ImportExcel(ctx context.Context, content []byte, filename, createdBy string) (ImportResponse, error) {
	// 1. 解析 Excel 前两列并校验日期唯一性。
	records, err := parseExcel(content)
	if err != nil {
		return ImportResponse{}, err
	}

	// 2. 在一个事务中批量 upsert 并组装响应。
	stats, err := s.repository.Upsert(ctx, records, filename, createdBy)
	if err != nil {
		return ImportResponse{}, fmt.Errorf("导入麻将 Excel: %w", err)
	}
	response := ImportResponse{
		Filename:      filename,
		ParsedCount:   len(records),
		InsertedCount: stats.insertedCount,
		UpdatedCount:  stats.updatedCount,
		SkippedCount:  stats.skippedCount,
	}
	if stats.latestRecord != nil {
		record := stats.latestRecord.Record
		response.LatestRecord = &record
	}
	return response, nil
}

// Report 读取有界记录并生成完整麻将报告。
// 输入：ctx 是调用上下文，limit 是记录上限，tableFee 是每场费用文本。
// 输出：返回摘要、频率、趋势、周期和最近记录。
// 副作用：读取 PostgreSQL。
func (s *Service) Report(ctx context.Context, limit int, tableFee string) (Report, error) {
	// 1. 约束查询上限并解析场费。
	if limit < 1 {
		limit = 1
	}
	if limit > 5000 {
		limit = 5000
	}
	tableFeeCents, err := money.ParseCents(tableFee)
	if err != nil {
		return Report{}, ErrInvalidInput
	}

	// 2. 读取日期正序窗口和倒序最近记录。
	records, err := s.repository.ListRecentWindow(ctx, limit)
	if err != nil {
		return Report{}, fmt.Errorf("读取麻将报告记录: %w", err)
	}
	recent, err := s.repository.ListRecent(ctx, 30)
	if err != nil {
		return Report{}, fmt.Errorf("读取最近麻将记录: %w", err)
	}

	// 3. 使用纯计算函数组装全部页面数据。
	return Report{
		Summary:       buildSummary(records, tableFeeCents),
		Frequency:     buildFrequency(records),
		Timeline:      buildTimeline(records),
		Monthly:       buildPeriods(records, "2006-01"),
		Yearly:        buildPeriods(records, "2006"),
		RecentRecords: publicRecords(recent),
		RecordCount:   len(records),
	}, nil
}

// PageSummary 返回控制台使用的轻量麻将摘要。
// 输入：ctx 是调用上下文。
// 输出：返回标题、说明和四张指标卡。
// 副作用：读取 PostgreSQL。
func (s *Service) PageSummary(ctx context.Context) (map[string]any, error) {
	// 1. 复用完整报告保证统计口径一致。
	report, err := s.Report(ctx, 1000, "9")
	if err != nil {
		return nil, err
	}
	summary := report.Summary
	return map[string]any{
		"title":       "麻将战绩",
		"description": "记录每次打牌的当日输赢，观察累计输赢、胜率、场均和月度波动。",
		"metrics": []map[string]string{
			{"label": "总场次", "value": strconv.Itoa(summary.TotalGames), "detail": "Excel 战绩记录", "status": "normal"},
			{"label": "总输赢", "value": signedAmount(summary.TotalResult), "detail": "累计当日输赢", "status": amountStatus(summary.TotalResult)},
			{"label": "胜率", "value": summary.WinRate + "%", "detail": fmt.Sprintf("%d 胜 / %d 负", summary.WinGames, summary.LossGames), "status": "normal"},
			{"label": "实际场均", "value": signedAmount(summary.AdjustedAverageResult), "detail": "按场费 9 修正", "status": amountStatus(summary.AdjustedAverageResult)},
		},
	}, nil
}

// Recent 返回最近麻将战绩。
// 输入：ctx 是调用上下文，limit 是 1 到 200 的上限。
// 输出：返回日期倒序记录。
// 副作用：读取 PostgreSQL。
func (s *Service) Recent(ctx context.Context, limit int) ([]Record, error) {
	// 1. 约束列表上限并转换公开记录。
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	records, err := s.repository.ListRecent(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("读取最近麻将战绩: %w", err)
	}
	return publicRecords(records), nil
}

// parseWriteRequest 校验页面录入并转换为内部记录。
// 输入：request 是日期和金额文本。
// 输出：返回标准日期和整数分。
// 副作用：无。
func parseWriteRequest(request WriteRequest) (parsedRecord, error) {
	// 1. 日期必须是 ISO 日历日。
	playedDate := strings.TrimSpace(request.PlayedDate)
	if _, err := time.Parse(dateLayout, playedDate); err != nil {
		return parsedRecord{}, ErrInvalidInput
	}

	// 2. 金额统一按 ROUND_HALF_UP 转换为分。
	amountCents, err := money.ParseCents(request.ResultAmount)
	if err != nil {
		return parsedRecord{}, ErrInvalidInput
	}
	return parsedRecord{playedDate: playedDate, amountCents: amountCents}, nil
}

// parseExcel 从 xlsx 第一个工作表解析日期和当日输赢。
// 输入：content 是完整 xlsx 文件内容。
// 输出：返回按日期排序且日期唯一的记录。
// 副作用：读取内存中的压缩文件。
func parseExcel(content []byte) ([]parsedRecord, error) {
	// 1. 打开工作簿并读取第一个工作表的格式化行。
	workbook, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("打开麻将 Excel: %w", ErrInvalidInput)
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, ErrInvalidInput
	}
	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("读取麻将 Excel: %w", ErrInvalidInput)
	}
	if len(rows) == 0 || len(rows[0]) < 2 || strings.TrimSpace(rows[0][0]) != "日期" || strings.TrimSpace(rows[0][1]) != "当日输赢" {
		return nil, fmt.Errorf("Excel 前两列表头必须是：日期、当日输赢: %w", ErrInvalidInput)
	}

	// 2. 跳过全空行，拒绝缺列、重复日期和无效金额。
	records := make([]parsedRecord, 0, len(rows)-1)
	seenDates := make(map[string]struct{})
	for rowIndex, row := range rows[1:] {
		if len(row) == 0 || (strings.TrimSpace(row[0]) == "" && (len(row) < 2 || strings.TrimSpace(row[1]) == "")) {
			continue
		}
		if len(row) < 2 || strings.TrimSpace(row[0]) == "" || strings.TrimSpace(row[1]) == "" {
			return nil, fmt.Errorf("第 %d 行日期或当日输赢为空: %w", rowIndex+2, ErrInvalidInput)
		}
		playedDate, err := parseExcelDate(row[0])
		if err != nil {
			return nil, fmt.Errorf("第 %d 行日期格式无法识别: %w", rowIndex+2, ErrInvalidInput)
		}
		if _, exists := seenDates[playedDate]; exists {
			return nil, fmt.Errorf("第 %d 行日期重复：%s: %w", rowIndex+2, playedDate, ErrInvalidInput)
		}
		amountCents, err := money.ParseCents(row[1])
		if err != nil {
			return nil, fmt.Errorf("第 %d 行当日输赢不是数字: %w", rowIndex+2, ErrInvalidInput)
		}
		seenDates[playedDate] = struct{}{}
		records = append(records, parsedRecord{playedDate: playedDate, amountCents: amountCents})
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("Excel 没有可导入的战绩记录: %w", ErrInvalidInput)
	}

	// 3. 日期正序保证写入和趋势结果稳定。
	sort.Slice(records, func(left, right int) bool { return records[left].playedDate < records[right].playedDate })
	return records, nil
}

// parseExcelDate 兼容 ISO 文本、日期时间和 Excel 序列号。
// 输入：value 是单元格格式化文本。
// 输出：返回 ISO 日期。
// 副作用：无。
func parseExcelDate(value string) (string, error) {
	// 1. 尝试常见日期文本格式。
	value = strings.TrimSpace(strings.ReplaceAll(value, "/", "-"))
	for _, layout := range []string{"2006-01-02", "2006-1-2", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.Format(dateLayout), nil
		}
	}

	// 2. 数字单元格按 Excel 1900 日期系统转换。
	serial, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "", ErrInvalidInput
	}
	parsed, err := excelize.ExcelDateToTime(serial, false)
	if err != nil {
		return "", ErrInvalidInput
	}
	return parsed.Format(dateLayout), nil
}
