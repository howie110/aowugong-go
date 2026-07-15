// Package strategy 提供不访问数据库和外部服务的纯策略信号计算。
package strategy

import (
	"fmt"
	"strings"
)

const (
	shortWindow = 5
	longWindow  = 20
)

// Point 描述生成均线信号所需的一根原始行情。
type Point struct {
	Time     string  `json:"time"`
	Open     float64 `json:"open"`
	Close    float64 `json:"close"`
	PreClose float64 `json:"pre_close,omitempty"`
}

// Signal 描述带均线、上一周期均线和买卖信号的行情。
type Signal struct {
	Point
	MA5          float64 `json:"ma5"`
	MA20         float64 `json:"ma20"`
	PreviousMA5  float64 `json:"ma5_shift1"`
	PreviousMA20 float64 `json:"ma20_shift1"`
	Enter        bool    `json:"if_in"`
	Exit         bool    `json:"if_out"`
}

// MA5CrossMA20 生成五周期均线上穿买入、下穿卖出的信号。
// 输入：points 是按时间升序排列的行情。
// 输出：返回均线及前值完整的信号行情；无效输入返回错误。
// 副作用：无。
func MA5CrossMA20(points []Point) ([]Signal, error) {
	// 1. 使用正向交叉规则调用唯一均线计算入口。
	return generateMACross(points, false)
}

// MA20CrossMA5 生成二十周期均线上穿买入、下穿卖出的信号。
// 输入：points 是按时间升序排列的行情。
// 输出：返回均线及前值完整的信号行情；无效输入返回错误。
// 副作用：无。
func MA20CrossMA5(points []Point) ([]Signal, error) {
	// 1. 使用反向交叉规则调用唯一均线计算入口。
	return generateMACross(points, true)
}

// generateMACross 计算五与二十周期均线并生成交叉信号。
// 输入：points 是行情，reverse 为 true 时交换买卖方向。
// 输出：返回从第二十一个周期开始的有效信号。
// 副作用：无。
func generateMACross(points []Point, reverse bool) ([]Signal, error) {
	// 1. 校验价格、时间和最小行情数量。
	if len(points) <= longWindow {
		return nil, fmt.Errorf("均线策略至少需要 %d 根行情", longWindow+1)
	}
	previousTime := ""
	for index, point := range points {
		if strings.TrimSpace(point.Time) == "" || (previousTime != "" && point.Time <= previousTime) {
			return nil, fmt.Errorf("第 %d 根行情时间为空或未严格递增", index+1)
		}
		if point.Open <= 0 || point.Close <= 0 {
			return nil, fmt.Errorf("第 %d 根行情价格必须大于零", index+1)
		}
		previousTime = point.Time
	}

	// 2. 使用滚动和生成每个周期的五与二十周期均线。
	ma5 := make([]float64, len(points))
	ma20 := make([]float64, len(points))
	shortSum := float64(0)
	longSum := float64(0)
	for index, point := range points {
		shortSum += point.Close
		longSum += point.Close
		if index >= shortWindow {
			shortSum -= points[index-shortWindow].Close
		}
		if index >= longWindow {
			longSum -= points[index-longWindow].Close
		}
		if index >= shortWindow-1 {
			ma5[index] = shortSum / shortWindow
		}
		if index >= longWindow-1 {
			ma20[index] = longSum / longWindow
		}
	}

	// 3. 从均线前值完整的周期开始判断严格交叉。
	result := make([]Signal, 0, len(points)-longWindow)
	for index := longWindow; index < len(points); index++ {
		shortCrossesUp := ma20[index-1] > ma5[index-1] && ma20[index] < ma5[index]
		shortCrossesDown := ma20[index-1] < ma5[index-1] && ma20[index] > ma5[index]
		enter, exit := shortCrossesUp, shortCrossesDown
		if reverse {
			enter, exit = exit, enter
		}
		result = append(result, Signal{
			Point: points[index], MA5: ma5[index], MA20: ma20[index],
			PreviousMA5: ma5[index-1], PreviousMA20: ma20[index-1],
			Enter: enter, Exit: exit,
		})
	}
	return result, nil
}
