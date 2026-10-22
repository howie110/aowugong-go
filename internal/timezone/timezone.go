// Package timezone 固定业务时区，不依赖服务器本地设置或已安装的时区文件。
package timezone

import (
	"time"
	_ "time/tzdata"
)

// Shanghai 保留 IANA 历史夏令时规则，用于日历日期和调度。
var Shanghai = loadShanghai()

// Beijing 保留现有统计与时间文本的固定 UTC+8 契约。
var Beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

func loadShanghai() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	} // 标准库内嵌 tzdata 必须包含此时区。
	return location
}
