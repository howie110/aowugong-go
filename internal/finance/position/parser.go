package position

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var numberPattern = regexp.MustCompile(`[-+−－]?[0-9]+(?:[.,，][0-9]+)*`)

// AssetMetadata 描述解析仓位资产时由上传流程提供的元数据。
type AssetMetadata struct {
	SnapshotDate string
	BrokerName   string
	SourceApp    string
	ImagePath    string
	ImageSHA256  string
	OCRProvider  string
}

type ocrWord struct {
	Text   string
	X      float64
	Y      float64
	Width  float64
	Height float64
	HasPos bool
}

// ParseAssetSnapshot 把阿里云 OCR 响应转换为账户资产快照。
// 输入：rawOCR 是规范化响应，metadata 是上传日期和图片信息。
// 输出：返回未写库的资产快照；必需字段缺失时返回错误。
// 副作用：无，不访问数据库和外部接口。
func ParseAssetSnapshot(rawOCR map[string]any, metadata AssetMetadata) (Snapshot, error) {
	// 1. 提取全文和坐标文字块，并识别已知账户后四位。
	data := nestedMap(rawOCR, "data")
	words := extractOCRWords(data)
	text := buildOCRText(stringValue(data["content"]), words)
	suffix, err := findAccountSuffix(text)
	if err != nil {
		return Snapshot{}, err
	}

	// 2. 解析账户级资产字段。
	totalAsset, err := findMoneyField(words, text, "总资产")
	if err != nil {
		return Snapshot{}, err
	}
	marketValue, err := findMoneyField(words, text, "总市值")
	if err != nil {
		return Snapshot{}, err
	}
	availableCash, err := findMoneyField(words, text, "可用")
	if err != nil {
		return Snapshot{}, err
	}
	positionPercent := findPercentField(words, text, "仓位")
	otherAmount := roundMoney(totalAsset - marketValue - availableCash)

	// 3. 对资产勾稽差额写入非阻断提示。
	warnings := make([]string, 0)
	maxGap := math.Max(1000, math.Abs(totalAsset)*0.01)
	if math.Abs(otherAmount) > maxGap {
		warnings = append(warnings, "asset_check_gap_too_large")
	}

	// 4. 返回待补充账户别名和持仓明细的快照。
	return Snapshot{
		SnapshotDate: metadata.SnapshotDate, BrokerName: metadata.BrokerName, SourceApp: metadata.SourceApp,
		AccountSuffix: suffix, TotalAsset: totalAsset, MarketValue: marketValue,
		AvailableCash: availableCash, OtherAmount: otherAmount, PositionPercent: positionPercent,
		ImagePath: metadata.ImagePath, ImageSHA256: metadata.ImageSHA256, OCRProvider: metadata.OCRProvider,
		ProviderRequestID: stringValue(rawOCR["request_id"]), Warnings: warnings,
	}, nil
}

// ParseHoldings 把整张截图 OCR 坐标块转换为持仓明细。
// 输入：rawOCR 是整图识别响应。
// 输出：返回可确认的持仓行；未找到表头时返回空数组。
// 副作用：无。
func ParseHoldings(rawOCR map[string]any) []Holding {
	// 1. 提取坐标文字并定位持仓表头。
	words := extractOCRWords(nestedMap(rawOCR, "data"))
	headerY, ok := holdingsHeaderY(words)
	if !ok {
		return []Holding{}
	}

	// 2. 逐个左侧证券名称候选解析固定列数值。
	names := holdingNameWords(words, headerY)
	results := make([]Holding, 0, len(names))
	for _, name := range names {
		if holding, ok := holdingFromRow(words, name); ok {
			results = append(results, holding)
		}
	}
	return results
}

// extractOCRWords 提取 OCR 文字块中心坐标。
// 输入：data 是阿里云 Data JSON 对象。
// 输出：返回文字和可选坐标列表。
// 副作用：无。
func extractOCRWords(data map[string]any) []ocrWord {
	// 1. 遍历 prism_wordsInfo 并忽略空文字。
	items, _ := data["prism_wordsInfo"].([]any)
	results := make([]ocrWord, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(firstString(item, "word", "text"))
		if text == "" {
			continue
		}
		word := ocrWord{Text: text}
		word.X, word.Y, word.Width, word.Height, word.HasPos = boundingBox(item["pos"])
		results = append(results, word)
	}
	return results
}

// boundingBox 计算 OCR 四角点的中心和宽高。
// 输入：rawPos 是阿里云 pos 数组。
// 输出：返回中心、尺寸和坐标是否有效。
// 副作用：无。
func boundingBox(rawPos any) (float64, float64, float64, float64, bool) {
	// 1. 收集字典或二元数组形式的坐标点。
	items, ok := rawPos.([]any)
	if !ok || len(items) == 0 {
		return 0, 0, 0, 0, false
	}
	xs := make([]float64, 0, len(items))
	ys := make([]float64, 0, len(items))
	for _, rawPoint := range items {
		switch point := rawPoint.(type) {
		case map[string]any:
			x, xOK := numberValue(point["x"])
			y, yOK := numberValue(point["y"])
			if xOK && yOK {
				xs, ys = append(xs, x), append(ys, y)
			}
		case []any:
			if len(point) >= 2 {
				x, xOK := numberValue(point[0])
				y, yOK := numberValue(point[1])
				if xOK && yOK {
					xs, ys = append(xs, x), append(ys, y)
				}
			}
		}
	}
	if len(xs) == 0 {
		return 0, 0, 0, 0, false
	}

	// 2. 根据最小和最大坐标计算矩形。
	minX, maxX, minY, maxY := xs[0], xs[0], ys[0], ys[0]
	for index := 1; index < len(xs); index++ {
		minX, maxX = math.Min(minX, xs[index]), math.Max(maxX, xs[index])
		minY, maxY = math.Min(minY, ys[index]), math.Max(maxY, ys[index])
	}
	return (minX + maxX) / 2, (minY + maxY) / 2, maxX - minX, maxY - minY, true
}

// buildOCRText 合并全文和单个文字块供标签回退解析。
// 输入：content 是 OCR 全文，words 是文字块。
// 输出：返回换行分隔搜索文本。
// 副作用：无。
func buildOCRText(content string, words []ocrWord) string {
	// 1. 过滤空值并按原始顺序拼接。
	pieces := make([]string, 0, len(words)+1)
	if strings.TrimSpace(content) != "" {
		pieces = append(pieces, content)
	}
	for _, word := range words {
		pieces = append(pieces, word.Text)
	}
	return strings.Join(pieces, "\n")
}

// findAccountSuffix 从 OCR 全文中识别项目已配置账户后四位。
// 输入：text 是合并后的 OCR 文本。
// 输出：返回标准数字后四位；无法识别时返回错误。
// 副作用：无。
func findAccountSuffix(text string) (string, error) {
	// 1. 清理空白并优先识别星号账户标记。
	compact := strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "").Replace(text)
	marked := regexp.MustCompile(`\*{1,2}([0-9OoIiLlSsZzB]{4})`).FindStringSubmatch(compact)
	if len(marked) == 2 {
		suffix := normalizeSuffix(marked[1])
		if knownAccountSuffix(suffix) {
			return suffix, nil
		}
	}

	// 2. 星号失败时在全文查找已知账户。
	for _, account := range defaultAccounts {
		if strings.Contains(compact, account.AccountSuffix) {
			return account.AccountSuffix, nil
		}
	}
	return "", fmt.Errorf("未识别到账户后四位")
}

// normalizeSuffix 修正常见 OCR 字母和数字混淆。
// 输入：value 是四位候选文本。
// 输出：返回纯数字候选。
// 副作用：无。
func normalizeSuffix(value string) string {
	// 1. 使用固定映射替换视觉近似字符。
	replacer := strings.NewReplacer(
		"O", "0", "o", "0", "I", "1", "i", "1", "L", "1", "l", "1",
		"S", "5", "s", "5", "Z", "2", "z", "2", "B", "8",
	)
	return replacer.Replace(value)
}

// knownAccountSuffix 判断账户后四位是否在当前配置基线中。
// 输入：suffix 是标准数字后四位。
// 输出：存在时返回 true。
// 副作用：无。
func knownAccountSuffix(suffix string) bool {
	// 1. 与默认账户配置共用同一数据源。
	for _, account := range defaultAccounts {
		if account.AccountSuffix == suffix {
			return true
		}
	}
	return false
}

// findMoneyField 按标签坐标或全文读取金额。
// 输入：words 是坐标块，text 是全文，label 是字段名。
// 输出：返回四舍五入到分的金额；找不到时返回错误。
// 副作用：无。
func findMoneyField(words []ocrWord, text, label string) (float64, error) {
	// 1. 优先在标签下方查找距离最近的数字。
	if value, ok := numberByPosition(words, label, false); ok {
		return roundMoney(value), nil
	}

	// 2. 坐标不可用时在标签后的短窗口查找。
	if value, ok := numberAfterLabel(text, label); ok {
		return roundMoney(value), nil
	}
	return 0, fmt.Errorf("未识别到%s", label)
}

// findPercentField 按坐标或全文读取仓位百分比。
// 输入：words 是坐标块，text 是全文，label 是字段名。
// 输出：找到时返回四位小数指针，否则返回 nil。
// 副作用：无。
func findPercentField(words []ocrWord, text, label string) *float64 {
	// 1. 优先读取标签附近数值。
	if value, ok := numberByPosition(words, label, true); ok {
		result := math.Round(value*10000) / 10000
		return &result
	}

	// 2. 回退到全文标签窗口。
	if value, ok := numberAfterLabel(text, label); ok {
		result := math.Round(value*10000) / 10000
		return &result
	}
	return nil
}

// numberByPosition 根据标签相对位置读取数值。
// 输入：words 是文字块，label 是标签，sameLine 控制同一行或下方查找。
// 输出：返回距离评分最小的数字及是否找到。
// 副作用：无。
func numberByPosition(words []ocrWord, label string, sameLine bool) (float64, bool) {
	// 1. 定位第一个带坐标的标签文字块。
	var labelWord ocrWord
	found := false
	for _, word := range words {
		if word.HasPos && strings.Contains(strings.ReplaceAll(word.Text, " ", ""), label) {
			labelWord, found = word, true
			break
		}
	}
	if !found {
		return 0, false
	}

	// 2. 收集合理坐标范围内的数字候选。
	type candidate struct {
		Score float64
		Value float64
	}
	candidates := make([]candidate, 0)
	for _, word := range words {
		if !word.HasPos || word.Text == labelWord.Text {
			continue
		}
		dx, dy := math.Abs(word.X-labelWord.X), word.Y-labelWord.Y
		if sameLine {
			if math.Abs(dy) > 60 || dx > 190 {
				continue
			}
		} else if dy <= 0 || dy > 120 || dx > 170 {
			continue
		}
		value, ok := parseNumber(word.Text)
		if ok {
			candidates = append(candidates, candidate{Score: math.Abs(dy) + dx*0.25, Value: value})
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].Score < candidates[right].Score })
	return candidates[0].Value, true
}

// numberAfterLabel 从标签后短窗口提取第一个数字。
// 输入：text 是全文，label 是字段名。
// 输出：返回数字和是否找到。
// 副作用：无。
func numberAfterLabel(text, label string) (float64, bool) {
	// 1. 限制搜索窗口避免串到其他字段。
	index := strings.Index(text, label)
	if index < 0 {
		return 0, false
	}
	start := index + len(label)
	end := start + 160
	if end > len(text) {
		end = len(text)
	}
	return parseNumber(text[start:end])
}

// parseNumber 从文本中解析第一个普通数值。
// 输入：text 是可能包含千分位、百分号或中文负号的文本。
// 输出：返回浮点值和是否成功。
// 副作用：无。
func parseNumber(text string) (float64, bool) {
	// 1. 查找第一个数字片段，并兼容 OCR 把千分位逗号识别成小数点的情况。
	match := numberPattern.FindString(text)
	if match == "" {
		return 0, false
	}
	cleaned := normalizeOCRNumber(match)
	value, err := strconv.ParseFloat(cleaned, 64)
	return value, err == nil
}

// normalizeOCRNumber 把 OCR 数字片段转换为 strconv 可解析的小数文本。
// 输入：value 可能包含中英文千分位、多个小数点和中文负号。
// 输出：返回仅保留符号及一个可选小数点的数字文本。
// 副作用：无。
func normalizeOCRNumber(value string) string {
	// 1. 统一符号并移除明确的千分位逗号。
	cleaned := strings.NewReplacer(",", "", "，", "", "+", "", "−", "-", "－", "-").Replace(value)
	dotCount := strings.Count(cleaned, ".")
	if dotCount <= 1 {
		return cleaned
	}

	// 2. 多个点且末段为两位时，只保留末尾金额小数点；否则全部按千分位移除。
	lastDot := strings.LastIndex(cleaned, ".")
	if len(cleaned)-lastDot-1 == 2 {
		integerPart := strings.ReplaceAll(cleaned[:lastDot], ".", "")
		return integerPart + cleaned[lastDot:]
	}
	return strings.ReplaceAll(cleaned, ".", "")
}

// roundMoney 把浮点金额四舍五入到分。
// 输入：value 是 OCR 数值。
// 输出：返回两位小数金额。
// 副作用：无。
func roundMoney(value float64) float64 {
	// 1. 与项目金额规则一致执行 ROUND_HALF_UP 等价处理。
	if value >= 0 {
		return math.Floor(value*100+0.5) / 100
	}
	return math.Ceil(value*100-0.5) / 100
}

// holdingsHeaderY 定位持仓表头的最小纵坐标。
// 输入：words 是整图文字块。
// 输出：返回纵坐标和是否找到。
// 副作用：无。
func holdingsHeaderY(words []ocrWord) (float64, bool) {
	// 1. 收集持仓相关表头并取最靠上位置。
	found, result := false, 0.0
	for _, word := range words {
		if !word.HasPos || !(strings.Contains(word.Text, "持仓股") || strings.Contains(word.Text, "持仓/可用") || strings.HasPrefix(word.Text, "市值")) {
			continue
		}
		if !found || word.Y < result {
			found, result = true, word.Y
		}
	}
	return result, found
}

// holdingNameWords 定位持仓表左侧证券名称候选。
// 输入：words 是整图文字块，headerY 是表头纵坐标。
// 输出：返回按纵坐标排序且同行去重的名称。
// 副作用：无。
func holdingNameWords(words []ocrWord, headerY float64) []ocrWord {
	// 1. 按坐标排序后过滤表头、数值和非左侧内容。
	sorted := append([]ocrWord(nil), words...)
	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].Y == sorted[right].Y {
			return sorted[left].X < sorted[right].X
		}
		return sorted[left].Y < sorted[right].Y
	})
	results := make([]ocrWord, 0)
	for _, word := range sorted {
		text := strings.TrimSpace(word.Text)
		if !word.HasPos || word.Y <= headerY+55 || word.X > 220 || text == "" ||
			strings.HasPrefix(text, "查看") || strings.Contains(text, "持仓") || strings.Contains(text, "市值") || isPlainNumber(text) {
			continue
		}
		duplicateRow := false
		for _, existing := range results {
			if math.Abs(existing.Y-word.Y) < 35 {
				duplicateRow = true
				break
			}
		}
		if !duplicateRow {
			results = append(results, word)
		}
	}
	return results
}

// holdingFromRow 按固定列坐标解析一个证券持仓。
// 输入：words 是整图文字块，name 是证券名称块。
// 输出：返回持仓及是否存在有效金额或数量。
// 副作用：无。
func holdingFromRow(words []ocrWord, name ocrWord) (Holding, bool) {
	// 1. 按同花顺表格列读取市值、数量、盈亏和价格。
	topY, bottomY := name.Y, name.Y+48
	marketValue, marketOK := rowNumber(words, bottomY, 20, 220)
	quantity, quantityOK := rowNumber(words, topY, 470, 660)
	if !marketOK && !quantityOK {
		return Holding{}, false
	}
	available, availableOK := rowNumber(words, bottomY, 470, 660)
	profit, profitOK := rowNumber(words, topY, 250, 450)
	profitPercent, profitPercentOK := rowNumber(words, bottomY, 250, 450)
	cost, costOK := rowNumber(words, topY, 680, 850)
	current, currentOK := rowNumber(words, bottomY, 680, 850)

	// 2. 组装可空字段并返回。
	holding := Holding{SecurityName: strings.TrimSpace(name.Text), MarketValue: roundMoney(marketValue)}
	holding.Quantity = optionalNumber(quantity, quantityOK)
	holding.AvailableQuantity = optionalNumber(available, availableOK)
	holding.ProfitAmount = optionalNumber(roundMoney(profit), profitOK)
	holding.ProfitPercent = optionalNumber(math.Round(profitPercent*10000)/10000, profitPercentOK)
	holding.CostPrice = optionalNumber(cost, costOK)
	holding.CurrentPrice = optionalNumber(current, currentOK)
	return holding, true
}

// rowNumber 在指定行和横向范围内读取最近数字。
// 输入：words 是坐标块，targetY 是目标行，minX 和 maxX 是列范围。
// 输出：返回数字和是否找到。
// 副作用：无。
func rowNumber(words []ocrWord, targetY, minX, maxX float64) (float64, bool) {
	// 1. 在行高容差内按距离评分查找候选。
	type candidate struct{ score, value float64 }
	candidates := make([]candidate, 0)
	centerX := (minX + maxX) / 2
	for _, word := range words {
		if !word.HasPos || word.X < minX || word.X > maxX || math.Abs(word.Y-targetY) > 26 {
			continue
		}
		if value, ok := parseNumber(word.Text); ok {
			candidates = append(candidates, candidate{score: math.Abs(word.Y-targetY) + math.Abs(centerX-word.X)*0.02, value: value})
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].score < candidates[right].score })
	return candidates[0].value, true
}

// optionalNumber 把带有效标记的数值转换为可空指针。
// 输入：value 是数值，valid 表示是否识别成功。
// 输出：有效时返回指针，否则返回 nil。
// 副作用：无。
func optionalNumber(value float64, valid bool) *float64 {
	// 1. 保留 OCR 字段缺失语义。
	if !valid {
		return nil
	}
	result := value
	return &result
}

// isPlainNumber 判断文字块是否只包含一个数字。
// 输入：text 是文字块文本。
// 输出：纯数字时返回 true。
// 副作用：无。
func isPlainNumber(text string) bool {
	// 1. 清理百分号后进行完整匹配。
	cleaned := strings.TrimSpace(strings.NewReplacer("，", ",", "%", "").Replace(text))
	return numberPattern.FindString(cleaned) == cleaned
}

// nestedMap 从通用 JSON 对象读取子对象。
// 输入：value 是父对象，key 是字段名。
// 输出：字段为对象时返回该对象，否则返回空对象。
// 副作用：无。
func nestedMap(value map[string]any, key string) map[string]any {
	// 1. 统一 OCR JSON 对象读取规则。
	if result, ok := value[key].(map[string]any); ok {
		return result
	}
	return map[string]any{}
}

// firstString 读取对象中第一个非空字符串字段。
// 输入：value 是对象，keys 是候选字段顺序。
// 输出：返回第一个非空文本。
// 副作用：无。
func firstString(value map[string]any, keys ...string) string {
	// 1. 按优先级读取候选字段。
	for _, key := range keys {
		if result := stringValue(value[key]); result != "" {
			return result
		}
	}
	return ""
}

// stringValue 把通用 JSON 值转换为字符串。
// 输入：value 是任意 JSON 值。
// 输出：字符串原样返回，其他值返回 fmt 文本，nil 返回空字符串。
// 副作用：无。
func stringValue(value any) string {
	// 1. 保持 nil 和字符串的常见语义。
	if value == nil {
		return ""
	}
	if result, ok := value.(string); ok {
		return result
	}
	return fmt.Sprint(value)
}

// numberValue 把 JSON 数值转换为 float64。
// 输入：value 是解码后的数字或数字文本。
// 输出：返回数值和是否成功。
// 副作用：无。
func numberValue(value any) (float64, bool) {
	// 1. 覆盖 encoding/json 和手工测试常见数值类型。
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		result, err := strconv.ParseFloat(typed, 64)
		return result, err == nil
	default:
		return 0, false
	}
}
