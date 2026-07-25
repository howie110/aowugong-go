package database

import (
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsDuplicateKey 判断错误链是否包含 SQLite 唯一键或主键冲突。
// 输入：err 是数据库调用返回且可能被业务上下文包装的错误。
// 输出：SQLite 扩展错误码属于约束冲突时返回 true，其余情况返回 false。
// 副作用：无。
func IsDuplicateKey(err error) bool {
	// 1. 沿错误链提取驱动错误。
	var sqliteError *sqlite.Error
	if !errors.As(err, &sqliteError) {
		return false
	}

	// 2. 扩展错误码低八位为基础 SQLITE_CONSTRAINT。
	return sqliteError.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}
