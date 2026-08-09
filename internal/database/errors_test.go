package database_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/howiedata/aowugong-go/internal/database"
	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestIsDuplicateKeyRecognizesWrappedSQLiteError 验证唯一键错误经过业务包装后仍可识别。
// 输入：SQLite 唯一键冲突和普通错误。
// 输出：仅 SQLite 约束冲突返回 true。
// 副作用：创建并写入测试临时 SQLite 文件。
func TestIsDuplicateKeyRecognizesWrappedSQLiteError(t *testing.T) {
	// 1. 创建唯一索引并触发真实 SQLite 驱动错误。
	db := testdatabase.Open(t)
	if _, err := db.Exec(`CREATE TABLE duplicate_test(name TEXT NOT NULL UNIQUE);
		INSERT INTO duplicate_test(name) VALUES('same')`); err != nil {
		t.Fatalf("prepare duplicate table: %v", err)
	}
	_, err := db.Exec("INSERT INTO duplicate_test(name) VALUES(?)", "same")
	duplicate := fmt.Errorf("新增记录: %w", err)

	// 2. 断言唯一键和普通错误被准确区分。
	if !database.IsDuplicateKey(duplicate) {
		t.Error("IsDuplicateKey(SQLite constraint) = false, want true")
	}
	if database.IsDuplicateKey(errors.New("unique constraint")) {
		t.Error("IsDuplicateKey(text error) = true, want false")
	}
}
