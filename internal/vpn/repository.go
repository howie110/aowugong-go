package vpn

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appdatabase "github.com/howiedata/aowugong-go/internal/database"
)

// Repository 负责 PostgreSQL 中的 VPN 订阅设备状态。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建 VPN 订阅设备仓储。
// 输入：db 是已完成迁移的数据库连接池。
// 输出：返回可供服务层使用的仓储。
// 副作用：无，不访问数据库。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存应用层显式注入的数据库连接。
	return &Repository{db: db}
}

// List 返回全部 VPN 订阅设备。
// 输入：ctx 是调用上下文。
// 输出：返回按主键倒序排列的设备。
// 副作用：读取 PostgreSQL。
func (r *Repository) List(ctx context.Context) ([]storedSubscription, error) {
	// 1. 查询页面展示和订阅派生需要的完整字段。
	rows, err := r.db.QueryContext(ctx, `
		SELECT subscription.id, subscription.user_id, app_user.username,
		       subscription.profile_code, subscription.token_version, subscription.status,
		       subscription.published_at, subscription.last_error,
		       subscription.created_at, subscription.updated_at
		FROM vpn_subscription_device AS subscription
		JOIN aowugong_fastapi_users AS app_user ON app_user.id = subscription.user_id
		ORDER BY app_user.username
	`)
	if err != nil {
		return nil, fmt.Errorf("查询 VPN 订阅设备: %w", err)
	}
	defer rows.Close()

	// 2. 扫描全部设备并检查游标错误。
	devices := make([]storedSubscription, 0)
	for rows.Next() {
		device, scanErr := scanStoredDevice(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("扫描 VPN 订阅设备: %w", scanErr)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 VPN 订阅设备: %w", err)
	}
	return devices, nil
}

// ListForUser 返回当前登录用户自己的 VPN 订阅。
// 输入：ctx 是调用上下文，userID 是当前用户主键。
// 输出：返回零或一条用户订阅。
// 副作用：读取 PostgreSQL。
func (r *Repository) ListForUser(ctx context.Context, userID int64) ([]storedSubscription, error) {
	// 1. 使用用户外键限制查询范围，避免普通用户读取他人地址。
	rows, err := r.db.QueryContext(ctx, `
		SELECT subscription.id, subscription.user_id, app_user.username,
		       subscription.profile_code, subscription.token_version, subscription.status,
		       subscription.published_at, subscription.last_error,
		       subscription.created_at, subscription.updated_at
		FROM vpn_subscription_device AS subscription
		JOIN aowugong_fastapi_users AS app_user ON app_user.id = subscription.user_id
		WHERE subscription.user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("查询用户 VPN 订阅: %w", err)
	}
	defer rows.Close()

	// 2. 扫描当前用户唯一订阅。
	devices := make([]storedSubscription, 0, 1)
	for rows.Next() {
		device, scanErr := scanStoredDevice(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("扫描用户 VPN 订阅: %w", scanErr)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历用户 VPN 订阅: %w", err)
	}
	return devices, nil
}

// ListUsers 返回管理员可分配 VPN 资源的活动登录用户。
// 输入：ctx 是调用上下文。
// 输出：返回用户以及是否已经开通订阅。
// 副作用：读取 PostgreSQL。
func (r *Repository) ListUsers(ctx context.Context) ([]UserOption, error) {
	// 1. 左连接订阅表，在一个查询中得到用户占用状态。
	rows, err := r.db.QueryContext(ctx, `
		SELECT app_user.id, app_user.username, app_user.email,
		       CASE WHEN subscription.id IS NULL THEN 0 ELSE 1 END
		FROM aowugong_fastapi_users AS app_user
		LEFT JOIN vpn_subscription_device AS subscription ON subscription.user_id = app_user.id
		WHERE app_user.is_active = 1
		ORDER BY app_user.username
	`)
	if err != nil {
		return nil, fmt.Errorf("查询 VPN 可分配用户: %w", err)
	}
	defer rows.Close()

	// 2. 扫描完整用户列表供管理员选择。
	users := make([]UserOption, 0)
	for rows.Next() {
		var user UserOption
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.HasSubscription); err != nil {
			return nil, fmt.Errorf("扫描 VPN 可分配用户: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 VPN 可分配用户: %w", err)
	}
	return users, nil
}

// Get 按主键读取一台 VPN 订阅设备。
// 输入：ctx 是调用上下文，deviceID 是设备主键。
// 输出：返回设备；不存在时返回 ErrNotFound。
// 副作用：读取 PostgreSQL。
func (r *Repository) Get(ctx context.Context, deviceID int64) (storedSubscription, error) {
	// 1. 查询单条设备并统一转换不存在错误。
	device, err := scanStoredDevice(r.db.QueryRowContext(ctx, `
		SELECT subscription.id, subscription.user_id, app_user.username,
		       subscription.profile_code, subscription.token_version, subscription.status,
		       subscription.published_at, subscription.last_error,
		       subscription.created_at, subscription.updated_at
		FROM vpn_subscription_device AS subscription
		JOIN aowugong_fastapi_users AS app_user ON app_user.id = subscription.user_id
		WHERE subscription.id = ?
	`, deviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return storedSubscription{}, ErrNotFound
	}
	if err != nil {
		return storedSubscription{}, fmt.Errorf("查询 VPN 用户订阅: %w", err)
	}
	return device, nil
}

// Create 新增一台待发布 VPN 订阅设备。
// 输入：ctx 是调用上下文，request 是已校验名称和资源编码。
// 输出：返回数据库最终记录；名称重复时返回 ErrConflict。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) Create(ctx context.Context, request CreateRequest) (storedSubscription, error) {
	// 1. 读取活动用户名，旧 name 字段只保留数据库兼容用途。
	var username string
	if err := r.db.QueryRowContext(ctx, `
		SELECT username FROM aowugong_fastapi_users WHERE id = ? AND is_active = 1
	`, request.UserID).Scan(&username); errors.Is(err, sql.ErrNoRows) {
		return storedSubscription{}, ErrUserNotFound
	} else if err != nil {
		return storedSubscription{}, fmt.Errorf("查询 VPN 订阅用户: %w", err)
	}

	// 2. 写入用户外键和资源，不保存订阅明文密钥。
	var deviceID int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO vpn_subscription_device (name, user_id, profile_code)
		VALUES (?, ?, ?)
		RETURNING id
	`, username, request.UserID, request.ProfileCode).Scan(&deviceID)
	if err != nil {
		if appdatabase.IsDuplicateKey(err) {
			return storedSubscription{}, ErrConflict
		}
		return storedSubscription{}, fmt.Errorf("新增 VPN 用户订阅: %w", err)
	}
	return r.Get(ctx, deviceID)
}

// UpdateTokenVersion 更新设备的订阅密钥版本。
// 输入：ctx 是调用上下文，deviceID 是设备主键，version 是新版本。
// 输出：返回更新后的设备。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) UpdateTokenVersion(ctx context.Context, deviceID int64, version int) (storedSubscription, error) {
	// 1. 原子更新版本和时间，并确认目标设备存在。
	result, err := r.db.ExecContext(ctx, `
		UPDATE vpn_subscription_device
		SET token_version = ?, status = ?, last_error = '', updated_at = ?
		WHERE id = ?
	`, version, StatusActive, appdatabase.TimestampText(time.Now()), deviceID)
	if err != nil {
		return storedSubscription{}, fmt.Errorf("更新 VPN 订阅密钥版本: %w", err)
	}
	if err := requireAffectedRow(result); err != nil {
		return storedSubscription{}, err
	}
	return r.Get(ctx, deviceID)
}

// MarkPublished 记录设备配置已成功推送。
// 输入：ctx 是调用上下文，deviceID 是设备主键。
// 输出：返回更新后的设备。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) MarkPublished(ctx context.Context, deviceID int64) (storedSubscription, error) {
	// 1. 使用同一时间记录发布和更新时间。
	now := appdatabase.TimestampText(time.Now())
	result, err := r.db.ExecContext(ctx, `
		UPDATE vpn_subscription_device
		SET status = ?, published_at = ?, last_error = '', updated_at = ?
		WHERE id = ?
	`, StatusActive, now, now, deviceID)
	if err != nil {
		return storedSubscription{}, fmt.Errorf("记录 VPN 订阅发布成功: %w", err)
	}
	if err := requireAffectedRow(result); err != nil {
		return storedSubscription{}, err
	}
	return r.Get(ctx, deviceID)
}

// MarkPublishFailed 记录设备最近一次发布错误。
// 输入：ctx 是调用上下文，deviceID 是设备主键，message 是安全错误文本。
// 输出：返回更新后的设备。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) MarkPublishFailed(ctx context.Context, deviceID int64, message string) (storedSubscription, error) {
	// 1. 保存不含节点内容和 Token 的错误摘要。
	result, err := r.db.ExecContext(ctx, `
		UPDATE vpn_subscription_device
		SET status = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`, StatusError, message, appdatabase.TimestampText(time.Now()), deviceID)
	if err != nil {
		return storedSubscription{}, fmt.Errorf("记录 VPN 订阅发布失败: %w", err)
	}
	if err := requireAffectedRow(result); err != nil {
		return storedSubscription{}, err
	}
	return r.Get(ctx, deviceID)
}

// MarkRevoked 记录设备已撤销。
// 输入：ctx 是调用上下文，deviceID 是设备主键。
// 输出：返回更新后的设备。
// 副作用：写入并读取 PostgreSQL。
func (r *Repository) MarkRevoked(ctx context.Context, deviceID int64) (storedSubscription, error) {
	// 1. 清理发布状态并保留设备审计记录。
	result, err := r.db.ExecContext(ctx, `
		UPDATE vpn_subscription_device
		SET status = ?, published_at = NULL, last_error = '', updated_at = ?
		WHERE id = ?
	`, StatusRevoked, appdatabase.TimestampText(time.Now()), deviceID)
	if err != nil {
		return storedSubscription{}, fmt.Errorf("撤销 VPN 用户订阅: %w", err)
	}
	if err := requireAffectedRow(result); err != nil {
		return storedSubscription{}, err
	}
	return r.Get(ctx, deviceID)
}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanStoredDevice 从数据库当前行读取一台 VPN 订阅设备。
// 输入：scanner 是 QueryRow 或 Rows 的当前行。
// 输出：返回内部设备记录。
// 副作用：读取数据库游标。
func scanStoredDevice(scanner rowScanner) (storedSubscription, error) {
	// 1. 使用 NullString 保留未发布状态。
	var device storedSubscription
	var publishedAt sql.NullString
	err := scanner.Scan(
		&device.ID, &device.UserID, &device.Username, &device.ProfileCode, &device.TokenVersion,
		&device.Status, &publishedAt, &device.LastError, &device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return storedSubscription{}, err
	}
	if publishedAt.Valid {
		device.PublishedAt = &publishedAt.String
	}
	return device, nil
}

// requireAffectedRow 确认数据库更新命中目标设备。
// 输入：result 是更新执行结果。
// 输出：未命中时返回 ErrNotFound。
// 副作用：读取数据库驱动返回值。
func requireAffectedRow(result sql.Result) error {
	// 1. 读取影响行数并统一转换业务错误。
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 VPN 订阅更新结果: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
