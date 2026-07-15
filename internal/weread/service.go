// Package weread 聚合微信读书实时数据供 React 页面展示。
package weread

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/howiedata/aowugong-go/internal/client"
)

// Service 把微信读书网关响应整理为页面契约。
type Service struct {
	client   *client.WeReadClient
	location *time.Location
}

// NewService 创建微信读书业务服务。
// 输入：gateway 是微信读书外部客户端。
// 输出：返回使用 Asia/Shanghai 日期口径的服务。
// 副作用：无。
func NewService(gateway *client.WeReadClient) *Service {
	// 1. 固定业务时区，系统缺少时区数据库时使用本地时区兜底。
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.Local
	}
	return &Service{client: gateway, location: location}
}

// Dashboard 返回兼容旧接口的完整微信读书看板。
// 输入：ctx 是调用上下文。
// 输出：返回页面标题、指标及 summary/progress/heatmap 合集。
// 副作用：调用微信读书外部 HTTP API，不写数据库。
func (s *Service) Dashboard(ctx context.Context) (map[string]any, error) {
	// 1. 复用三个稳定子接口，避免出现两套聚合规则。
	summary, err := s.Summary(ctx)
	if err != nil {
		return nil, err
	}
	progress, err := s.Progress(ctx)
	if err != nil {
		return nil, err
	}
	heatmap, err := s.Heatmap(ctx)
	if err != nil {
		return nil, err
	}

	// 2. 合并微信读书专属数据并保留控制台页面字段。
	weread := map[string]any{
		"summary":        summary["summary"],
		"recent_books":   progress["recent_books"],
		"progress_books": progress["progress_books"],
		"heatmap":        heatmap["heatmap"],
	}
	return map[string]any{
		"title":       "微信读书",
		"description": "实时读取微信读书账号的阅读统计和阅读进度，不在本地落库。",
		"metrics":     summary["metrics"],
		"weread":      weread,
	}, nil
}

// Summary 返回阅读累计、天数、笔记和最近阅读指标。
// 输入：ctx 是调用上下文。
// 输出：返回 metrics、summary 和 recent_books。
// 副作用：调用微信读书外部 HTTP API，不写数据库。
func (s *Service) Summary(ctx context.Context) (map[string]any, error) {
	// 1. 读取总览、本月、笔记概览和书架。
	overall, err := s.client.Call(ctx, "/readdata/detail", map[string]any{"mode": "overall"})
	if err != nil {
		return nil, fmt.Errorf("读取微信读书总览: %w", err)
	}
	monthly, err := s.client.Call(ctx, "/readdata/detail", map[string]any{"mode": "monthly"})
	if err != nil {
		return nil, fmt.Errorf("读取微信读书本月数据: %w", err)
	}
	notebooks, err := s.allNotebooks(ctx, 100, 5)
	if err != nil {
		return nil, err
	}
	shelf, err := s.client.Call(ctx, "/shelf/sync", nil)
	if err != nil {
		return nil, fmt.Errorf("读取微信读书书架: %w", err)
	}

	// 2. 计算现有页面需要的指标和最近书籍。
	recentBooks := s.buildRecentBooks(shelf, 6)
	totalSeconds := client.IntValue(overall["totalReadTime"])
	readDays := client.IntValue(overall["readDays"])
	noteTotal := client.IntValue(notebooks["totalNoteCount"])
	noteBookTotal := client.IntValue(notebooks["totalBookCount"])
	monthSeconds := client.IntValue(monthly["totalReadTime"])
	monthDays := client.IntValue(monthly["readDays"])
	var recentBook any
	recentTitle := "暂无"
	recentDetail := "暂无最近阅读记录"
	if len(recentBooks) != 0 {
		recentBook = recentBooks[0]
		recentTitle = client.StringValue(recentBooks[0]["title"])
		recentDetail = client.StringValue(recentBooks[0]["read_date"])
	}

	// 3. 返回与 React 类型一致的结构。
	return map[string]any{
		"metrics": []map[string]string{
			{"label": "累计阅读", "value": formatDuration(totalSeconds), "detail": "本月 " + formatDuration(monthSeconds)},
			{"label": "阅读天数", "value": fmt.Sprintf("%d 天", readDays), "detail": fmt.Sprintf("本月 %d 天有阅读", monthDays)},
			{"label": "笔记总数", "value": fmt.Sprintf("%d 条", noteTotal), "detail": fmt.Sprintf("覆盖 %d 本书", noteBookTotal)},
			{"label": "最近阅读", "value": recentTitle, "detail": recentDetail},
		},
		"summary": map[string]any{
			"total_read_seconds": totalSeconds,
			"total_read_text":    formatDuration(totalSeconds),
			"read_days":          readDays,
			"note_total":         noteTotal,
			"note_book_total":    noteBookTotal,
			"recent_book":        recentBook,
			"updated_at":         time.Now().In(s.location).Format("2006-01-02 15:04"),
		},
		"recent_books": recentBooks,
	}, nil
}

// Progress 返回最近阅读书籍和逐书阅读进度。
// 输入：ctx 是调用上下文。
// 输出：返回 recent_books 和 progress_books。
// 副作用：调用微信读书外部 HTTP API，不写数据库。
func (s *Service) Progress(ctx context.Context) (map[string]any, error) {
	// 1. 读取书架并选取最近六本书。
	shelf, err := s.client.Call(ctx, "/shelf/sync", nil)
	if err != nil {
		return nil, fmt.Errorf("读取微信读书书架: %w", err)
	}
	recentBooks := s.buildRecentBooks(shelf, 6)

	// 2. 对每本书读取进度；单本失败使用稳定默认值，不阻塞整页。
	progressBooks := make([]map[string]any, 0, len(recentBooks))
	for _, book := range recentBooks {
		bookID := client.StringValue(book["book_id"])
		if bookID == "" {
			continue
		}
		response, callErr := s.client.Call(ctx, "/book/getprogress", map[string]any{"bookId": bookID})
		progress, _ := response["book"].(map[string]any)
		if callErr != nil || progress == nil {
			progress = map[string]any{}
		}
		updateTime := client.IntValue(progress["updateTime"])
		if updateTime == 0 {
			updateTime = client.IntValue(book["read_time"])
		}
		readingTimeText := formatDuration(client.IntValue(progress["readingTime"]))
		if callErr != nil {
			readingTimeText = "暂无"
		}
		progressBooks = append(progressBooks, map[string]any{
			"book_id":           bookID,
			"title":             book["title"],
			"author":            book["author"],
			"progress":          client.IntValue(progress["progress"]),
			"chapter_idx":       client.IntValue(progress["chapterIdx"]),
			"chapter_uid":       client.IntValue(progress["chapterUid"]),
			"summary":           client.StringValue(progress["summary"]),
			"reading_time_text": readingTimeText,
			"update_time":       updateTime,
			"update_date":       s.formatTimestamp(updateTime),
			"read_date":         book["read_date"],
			"finish_reading":    book["finish_reading"],
			"open_url":          bookURL(bookID),
		})
	}
	return map[string]any{"recent_books": recentBooks, "progress_books": progressBooks}, nil
}

// Heatmap 返回最近 365 天的阅读热力图。
// 输入：ctx 是调用上下文。
// 输出：返回 heatmap 对象。
// 副作用：按月份调用微信读书外部 HTTP API，不写数据库。
func (s *Service) Heatmap(ctx context.Context) (map[string]any, error) {
	// 1. 建立从今天向前 365 天的零值日期表。
	today := time.Now().In(s.location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, s.location)
	startDate := today.AddDate(0, 0, -364)
	secondsByDay := make(map[string]int, 365)
	for offset := 0; offset < 365; offset++ {
		secondsByDay[startDate.AddDate(0, 0, offset).Format("2006-01-02")] = 0
	}

	// 2. 按月读取日粒度数据，单月失败时保留零值。
	for _, monthStart := range monthStarts(startDate, today) {
		baseTime := time.Date(monthStart.Year(), monthStart.Month(), 1, 12, 0, 0, 0, s.location).Unix()
		response, err := s.client.Call(ctx, "/readdata/detail", map[string]any{"mode": "monthly", "baseTime": baseTime})
		if err != nil {
			continue
		}
		readTimes, _ := response["readTimes"].(map[string]any)
		for timestamp, seconds := range readTimes {
			unixSeconds := client.IntValue(timestamp)
			date := time.Unix(int64(unixSeconds), 0).In(s.location).Format("2006-01-02")
			if _, exists := secondsByDay[date]; exists {
				secondsByDay[date] = client.IntValue(seconds)
			}
		}
	}

	// 3. 按日期生成热力等级并汇总活跃天数。
	days := make([]map[string]any, 0, 365)
	totalSeconds := 0
	activeDays := 0
	for offset := 0; offset < 365; offset++ {
		date := startDate.AddDate(0, 0, offset).Format("2006-01-02")
		seconds := secondsByDay[date]
		totalSeconds += seconds
		if seconds > 0 {
			activeDays++
		}
		days = append(days, map[string]any{
			"date": date, "seconds": seconds, "minutes": seconds / 60, "level": heatLevel(seconds),
		})
	}
	return map[string]any{"heatmap": map[string]any{
		"start_date":    startDate.Format("2006-01-02"),
		"end_date":      today.Format("2006-01-02"),
		"active_days":   activeDays,
		"total_seconds": totalSeconds,
		"total_text":    formatDuration(totalSeconds),
		"days":          days,
	}}, nil
}

// allNotebooks 分页读取有限页笔记本概览。
// 输入：ctx 是调用上下文，pageSize 是每页数量，maxPages 是页数上限。
// 输出：返回合并后的总数和书籍数组。
// 副作用：调用微信读书外部 HTTP API，不写数据库。
func (s *Service) allNotebooks(ctx context.Context, pageSize, maxPages int) (map[string]any, error) {
	// 1. 初始化合并结构和游标。
	books := make([]any, 0)
	totalBookCount := 0
	totalNoteCount := 0
	var lastSort any
	for page := 0; page < maxPages; page++ {
		params := map[string]any{"count": pageSize}
		if lastSort != nil {
			params["lastSort"] = lastSort
		}
		response, err := s.client.Call(ctx, "/user/notebooks", params)
		if err != nil {
			return nil, fmt.Errorf("读取微信读书笔记: %w", err)
		}
		pageBooks := anySlice(response["books"])
		books = append(books, pageBooks...)
		totalBookCount = client.IntValue(response["totalBookCount"])
		totalNoteCount = client.IntValue(response["totalNoteCount"])

		// 2. 无下一页、空页或缺少游标时停止。
		if client.IntValue(response["hasMore"]) == 0 || len(pageBooks) == 0 {
			break
		}
		lastBook, _ := pageBooks[len(pageBooks)-1].(map[string]any)
		lastSort = lastBook["sort"]
		if lastSort == nil {
			break
		}
	}
	return map[string]any{
		"books": books, "totalBookCount": totalBookCount, "totalNoteCount": totalNoteCount,
	}, nil
}

// buildRecentBooks 从书架中选取最近阅读的书。
// 输入：shelf 是书架响应，limit 是返回上限。
// 输出：返回最近阅读时间倒序书籍。
// 副作用：无。
func (s *Service) buildRecentBooks(shelf map[string]any, limit int) []map[string]any {
	// 1. 只保留有阅读时间的有效书籍并按时间倒序。
	books := make([]map[string]any, 0)
	for _, raw := range anySlice(shelf["books"]) {
		book, ok := raw.(map[string]any)
		if ok && client.IntValue(book["readUpdateTime"]) > 0 {
			books = append(books, book)
		}
	}
	sort.Slice(books, func(left, right int) bool {
		return client.IntValue(books[left]["readUpdateTime"]) > client.IntValue(books[right]["readUpdateTime"])
	})
	if len(books) > limit {
		books = books[:limit]
	}

	// 2. 清洗页面所需书籍字段。
	result := make([]map[string]any, 0, len(books))
	for _, book := range books {
		bookID := client.StringValue(book["bookId"])
		title := client.StringValue(book["title"])
		if title == "" {
			title = "未命名书籍"
		}
		author := client.StringValue(book["author"])
		if author == "" {
			author = "未知作者"
		}
		readTime := client.IntValue(book["readUpdateTime"])
		result = append(result, map[string]any{
			"book_id": bookID, "title": title, "author": author,
			"finish_reading": boolValue(book["finishReading"]),
			"read_time":      readTime, "read_date": s.formatTimestamp(readTime), "open_url": bookURL(bookID),
		})
	}
	return result
}

// formatTimestamp 把 Unix 时间转换为上海日期。
// 输入：timestamp 是秒级 Unix 时间。
// 输出：零值返回“暂无”，否则返回 YYYY-MM-DD。
// 副作用：无。
func (s *Service) formatTimestamp(timestamp int) string {
	// 1. 转换到固定业务时区。
	if timestamp == 0 {
		return "暂无"
	}
	return time.Unix(int64(timestamp), 0).In(s.location).Format("2006-01-02")
}

// formatDuration 把秒数转换为小时分钟文本。
// 输入：seconds 是阅读秒数。
// 输出：返回中文时长。
// 副作用：无。
func formatDuration(seconds int) string {
	// 1. 负数按零处理并分离小时与分钟。
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := seconds % 3600 / 60
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟", hours, minutes)
	}
	return fmt.Sprintf("%d分钟", minutes)
}

// heatLevel 把阅读秒数映射为 0 到 4 热力等级。
// 输入：seconds 是单日阅读秒数。
// 输出：返回热力等级。
// 副作用：无。
func heatLevel(seconds int) int {
	// 1. 沿用旧页面的半小时、两小时和五小时阈值。
	switch {
	case seconds <= 0:
		return 0
	case seconds <= 30*60:
		return 1
	case seconds <= 2*60*60:
		return 2
	case seconds <= 5*60*60:
		return 3
	default:
		return 4
	}
}

// monthStarts 返回起止日期覆盖的每个月第一天。
// 输入：start 和 end 是闭区间日期。
// 输出：返回月份正序列表。
// 副作用：无。
func monthStarts(start, end time.Time) []time.Time {
	// 1. 从开始月逐月推进到结束月。
	current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())
	months := make([]time.Time, 0, 13)
	for !current.After(last) {
		months = append(months, current)
		current = current.AddDate(0, 1, 0)
	}
	return months
}

// bookURL 生成微信读书 App 书籍跳转链接。
// 输入：bookID 是书籍标识。
// 输出：返回 weread scheme 链接。
// 副作用：无。
func bookURL(bookID string) string {
	// 1. 空标识不生成无效链接。
	if bookID == "" {
		return ""
	}
	return "weread://reading?bId=" + bookID
}

// anySlice 把外部 JSON 数组安全转换为 []any。
// 输入：value 是任意外部值。
// 输出：数组原样返回，否则返回空数组。
// 副作用：无。
func anySlice(value any) []any {
	// 1. 统一外部数组空值语义。
	if values, ok := value.([]any); ok {
		return values
	}
	return []any{}
}

// boolValue 把外部 JSON 布尔或数值安全转换为 bool。
// 输入：value 是任意外部值。
// 输出：布尔真或非零数值返回 true。
// 副作用：无。
func boolValue(value any) bool {
	// 1. 优先保留原始布尔，否则按外部整数转换。
	if parsed, ok := value.(bool); ok {
		return parsed
	}
	return client.IntValue(value) != 0
}
