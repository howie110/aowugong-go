package database

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

// IsDuplicateKey 判断错误链是否包含 MySQL 唯一键冲突。
// 输入：err 是数据库调用返回且可能被业务上下文包装的错误。
// 输出：MySQL 错误码为 1062 时返回 true，其余情况返回 false。
// 副作用：无。
func IsDuplicateKey(err error) bool {
	// 1. 沿错误链提取驱动错误并比较稳定错误码。
	var mysqlError *mysql.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
}
