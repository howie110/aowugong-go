package mysqlmigration

import (
	"context"
	"database/sql"
)

// SourceReader 定义只读一致快照需要的查询能力，可由 sql.DB 或只读 sql.Tx 实现。
type SourceReader interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
