package monitoring

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Repository 负责应用 MySQL 中的监控结果持久化。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建服务监控仓储。
// 输入：db 是应用 MySQL 连接池。
// 输出：返回监控仓储。
// 副作用：无。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存应用层显式注入的数据库连接。
	return &Repository{db: db}
}

// Insert 写入一条服务监控结果。
// 输入：ctx 是调用上下文，result 是标准探测结果。
// 输出：成功返回 nil。
// 副作用：追加写入 MySQL。
func (r *Repository) Insert(ctx context.Context, result Result) error {
	// 1. 使用参数化 SQL 保存可空状态、耗时和错误字段。
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO service_monitor_result (
			target_code, target_name, target_url, status,
			http_status, latency_ms, error_message, checked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, result.TargetCode, result.TargetName, result.TargetURL, result.Status,
		nullInt(result.HTTPStatus), nullInt(result.LatencyMS), nullString(result.ErrorMessage), nullString(result.CheckedAt))
	if err != nil {
		return fmt.Errorf("写入服务监控结果 %s: %w", result.TargetCode, err)
	}
	return nil
}

// Latest 返回指定目标各自最近一次监控结果。
// 输入：ctx 是调用上下文，codes 是当前目标编码。
// 输出：返回按 target_code 索引的结果。
// 副作用：读取 MySQL。
func (r *Repository) Latest(ctx context.Context, codes []string) (map[string]Result, error) {
	// 1. 空目标直接返回空映射。
	if len(codes) == 0 {
		return map[string]Result{}, nil
	}

	// 2. 构造仅包含固定问号的 IN 查询并传入目标编码参数。
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(codes)), ",")
	args := make([]any, len(codes))
	for index, code := range codes {
		args[index] = code
	}
	query := fmt.Sprintf(`
		SELECT result.target_code, result.target_name, result.target_url, result.status,
		       result.http_status, result.latency_ms, result.error_message, result.checked_at
		FROM service_monitor_result result
		JOIN (
			SELECT target_code, MAX(id) AS latest_id
			FROM service_monitor_result
			WHERE target_code IN (%s)
			GROUP BY target_code
		) latest ON latest.latest_id = result.id
	`, placeholders)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询最近监控结果: %w", err)
	}
	defer rows.Close()

	// 3. 扫描可空字段并按目标编码建立映射。
	results := make(map[string]Result, len(codes))
	for rows.Next() {
		var result Result
		var httpStatus, latency sql.NullInt64
		var errorMessage, checkedAt sql.NullString
		if err := rows.Scan(
			&result.TargetCode, &result.TargetName, &result.TargetURL, &result.Status,
			&httpStatus, &latency, &errorMessage, &checkedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描最近监控结果: %w", err)
		}
		result.HTTPStatus = intPointer(httpStatus)
		result.LatencyMS = intPointer(latency)
		result.ErrorMessage = stringPointer(errorMessage)
		result.CheckedAt = stringPointer(checkedAt)
		results[result.TargetCode] = result
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历最近监控结果: %w", err)
	}
	return results, nil
}

// nullInt 把整数指针转换为 SQL 参数。
// 输入：value 是可选整数。
// 输出：nil 指针返回 SQL NULL。
// 副作用：无。
func nullInt(value *int) any {
	// 1. 保留 API 可空字段语义。
	if value == nil {
		return nil
	}
	return *value
}

// nullString 把字符串指针转换为 SQL 参数。
// 输入：value 是可选字符串。
// 输出：nil 指针返回 SQL NULL。
// 副作用：无。
func nullString(value *string) any {
	// 1. 保留 API 可空字段语义。
	if value == nil {
		return nil
	}
	return *value
}

// intPointer 把有效 sql.NullInt64 转换为 int 指针。
// 输入：value 是数据库可空整数。
// 输出：有效时返回 int 指针。
// 副作用：无。
func intPointer(value sql.NullInt64) *int {
	// 1. 保留数据库 NULL 语义。
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

// stringPointer 把有效 sql.NullString 转换为字符串指针。
// 输入：value 是数据库可空字符串。
// 输出：有效时返回字符串指针。
// 副作用：无。
func stringPointer(value sql.NullString) *string {
	// 1. 保留数据库 NULL 语义。
	if !value.Valid {
		return nil
	}
	return &value.String
}
