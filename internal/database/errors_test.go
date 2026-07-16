package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestIsDuplicateKeyRecognizesWrappedMySQLError 验证唯一键错误经过业务包装后仍可识别。
// 输入：包装后的 MySQL 1062 错误和普通错误。
// 输出：仅 1062 返回 true。
// 副作用：无。
func TestIsDuplicateKeyRecognizesWrappedMySQLError(t *testing.T) {
	// 1. 构造驱动唯一键错误并增加一层业务上下文。
	duplicate := fmt.Errorf("新增记录: %w", &mysql.MySQLError{Number: 1062, Message: "duplicate"})

	// 2. 断言唯一键和普通错误被准确区分。
	if !IsDuplicateKey(duplicate) {
		t.Error("IsDuplicateKey(1062) = false, want true")
	}
	if IsDuplicateKey(errors.New("unique constraint")) {
		t.Error("IsDuplicateKey(text error) = true, want false")
	}
}
