package database

import (
	"errors"
)

// IsDuplicateKey 判断错误链是否包含 PostgreSQL 或迁移测试库的唯一键冲突。
// 输入：err 是数据库调用返回且可能被业务上下文包装的错误。
// 输出：PostgreSQL 或隔离测试 SQLite 的约束冲突返回 true，其余情况返回 false。
// 副作用：无。
func IsDuplicateKey(err error) bool {
	// 1. PostgreSQL 使用标准 SQLSTATE 23505 表示唯一键冲突。
	var stateError interface{ SQLState() string }
	if errors.As(err, &stateError) && stateError.SQLState() == "23505" {
		return true
	}

	// 2. SQLite 仅供一次性迁移和隔离测试，基础约束错误码为 19。
	var codeError interface{ Code() int }
	return errors.As(err, &codeError) && codeError.Code()&0xff == 19
}
