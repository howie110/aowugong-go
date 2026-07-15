package strategy

import "testing"

// TestMA5CrossMA20CreatesDelayedExecutionSignals 验证五日均线穿越二十日均线的信号方向。
// 输入：先让短均线低于长均线，再用一个高收盘价完成上穿。
// 输出：只返回均线完整的周期，并在交叉周期标记买入信号。
// 副作用：无。
func TestMA5CrossMA20CreatesDelayedExecutionSignals(t *testing.T) {
	// 1. 构造二十根短均线偏低的历史和一根明显上穿行情。
	points := make([]Point, 0, 21)
	for index := 0; index < 15; index++ {
		points = append(points, point(index, 20))
	}
	for index := 15; index < 20; index++ {
		points = append(points, point(index, 10))
	}
	points = append(points, point(20, 100))

	// 2. 生成信号并核对旧项目 dropna 后仅保留第二十一个周期。
	result, err := MA5CrossMA20(points)
	if err != nil {
		t.Fatalf("MA5CrossMA20() error = %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("signal count = %d, want 1", len(result))
	}
	if !result[0].Enter || result[0].Exit {
		t.Errorf("signals = enter:%t exit:%t, want true/false", result[0].Enter, result[0].Exit)
	}
	if result[0].MA5 <= result[0].MA20 || result[0].PreviousMA5 >= result[0].PreviousMA20 {
		t.Errorf("moving averages = %+v, want upward cross", result[0])
	}
}

// TestMA20CrossMA5ReversesSignalDirection 验证反向策略把短均线下穿作为买入信号。
// 输入：短均线原先高于长均线，随后向下交叉。
// 输出：MA20CrossMA5 标记买入，MA5CrossMA20 对同一周期标记卖出。
// 副作用：无。
func TestMA20CrossMA5ReversesSignalDirection(t *testing.T) {
	// 1. 构造能够在最后一根完成短均线下穿的行情。
	points := make([]Point, 0, 21)
	for index := 0; index < 15; index++ {
		points = append(points, point(index, 10))
	}
	for index := 15; index < 20; index++ {
		points = append(points, point(index, 11))
	}
	points = append(points, point(20, 1))

	// 2. 分别执行正向和反向策略并核对信号互换。
	shortCross, err := MA5CrossMA20(points)
	if err != nil {
		t.Fatalf("MA5CrossMA20() error = %v", err)
	}
	longCross, err := MA20CrossMA5(points)
	if err != nil {
		t.Fatalf("MA20CrossMA5() error = %v", err)
	}
	if !shortCross[0].Exit || shortCross[0].Enter {
		t.Errorf("short cross signals = %+v", shortCross[0])
	}
	if !longCross[0].Enter || longCross[0].Exit {
		t.Errorf("long cross signals = %+v", longCross[0])
	}
}

// point 创建策略测试使用的单根行情。
// 输入：index 用于生成严格递增时间，close 是收盘价。
// 输出：返回开盘、昨收均有效的行情点。
// 副作用：无。
func point(index int, close float64) Point {
	// 1. 使用固定宽度日期序号保证词法顺序与时间顺序一致。
	return Point{Time: formatIndex(index), Open: close, Close: close, PreClose: close}
}

// formatIndex 把测试序号转换为固定宽度时间文本。
// 输入：index 是从零开始的测试序号。
// 输出：返回 YYYY-MM-DD 形式的递增日期文本。
// 副作用：无。
func formatIndex(index int) string {
	// 1. 测试数据不跨月，直接生成一月日期。
	day := index + 1
	if day < 10 {
		return "2026-01-0" + string(rune('0'+day))
	}
	return "2026-01-" + string([]byte{byte('0' + day/10), byte('0' + day%10)})
}
