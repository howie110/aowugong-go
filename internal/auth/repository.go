package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	appdatabase "github.com/howiedata/aowugong-go/internal/database"
)

// Repository 负责在 MySQL 中读写认证用户及其资料。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建认证仓储。
// 输入：db 是已完成迁移的 MySQL 连接池。
// 输出：返回可供认证服务使用的仓储。
// 副作用：无，不访问数据库。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存由应用层显式注入的数据库连接。
	return &Repository{db: db}
}

// FindByUsername 按用户名读取包含密码哈希的活动判断所需记录。
// 输入：ctx 是调用上下文，username 是精确用户名。
// 输出：返回内部用户记录；不存在时返回 ErrNotFound。
// 副作用：读取 MySQL。
func (r *Repository) FindByUsername(ctx context.Context, username string) (userRecord, error) {
	// 1. 查询登录校验需要的最小字段。
	var record userRecord
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, email, password, is_active, is_superuser
		FROM aowugong_fastapi_users
		WHERE username = ?
	`, username).Scan(
		&record.ID,
		&record.Username,
		&record.Email,
		&record.PasswordHash,
		&record.IsActive,
		&record.IsSuperuser,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return userRecord{}, ErrNotFound
	}
	if err != nil {
		return userRecord{}, fmt.Errorf("按用户名查询认证用户: %w", err)
	}
	return record, nil
}

// FindByID 按主键读取不含密码的用户记录。
// 输入：ctx 是调用上下文，userID 是用户主键。
// 输出：返回公开用户记录；不存在时返回 ErrNotFound。
// 副作用：读取 MySQL。
func (r *Repository) FindByID(ctx context.Context, userID int64) (User, error) {
	// 1. 查询 API 可以公开的用户字段。
	var user User
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, email, is_active, is_superuser
		FROM aowugong_fastapi_users
		WHERE id = ?
	`, userID).Scan(&user.ID, &user.Username, &user.Email, &user.IsActive, &user.IsSuperuser)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("按主键查询认证用户: %w", err)
	}
	return user, nil
}

// Create 写入一个使用 bcrypt 密码哈希的新用户。
// 输入：ctx 是调用上下文，req 是用户字段，passwordHash 是已生成的 bcrypt 哈希。
// 输出：返回新建用户；唯一键冲突时返回 ErrConflict。
// 副作用：写入 MySQL。
func (r *Repository) Create(ctx context.Context, req CreateUserRequest, passwordHash string) (User, error) {
	// 1. 插入用户并取得 MySQL 自增主键。
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO aowugong_fastapi_users (username, email, password, is_active, is_superuser)
		VALUES (?, ?, ?, 1, 0)
	`, req.Username, req.Email, passwordHash)
	if err != nil {
		if appdatabase.IsDuplicateKey(err) {
			return User{}, ErrConflict
		}
		return User{}, fmt.Errorf("创建认证用户: %w", err)
	}

	// 2. 使用自增主键读取数据库中的最终公开记录。
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("读取新用户主键: %w", err)
	}
	return r.FindByID(ctx, userID)
}

// Profile 读取用户以及去重排序后的角色和权限。
// 输入：ctx 是调用上下文，userID 是当前用户主键。
// 输出：返回资料、角色和权限；用户不存在时返回 ErrNotFound。
// 副作用：读取 MySQL。
func (r *Repository) Profile(ctx context.Context, userID int64) (Profile, error) {
	// 1. 读取用户基本资料，提前区分用户不存在。
	user, err := r.FindByID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	// 2. 查询启用角色并按 code 排序。
	roles, err := queryStrings(ctx, r.db, `
		SELECT DISTINCT role.code
		FROM aowugong_user_roles user_role
		JOIN aowugong_roles role ON role.id = user_role.role_id
		WHERE user_role.user_id = ? AND role.is_active = 1
		ORDER BY role.code
	`, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("查询用户角色: %w", err)
	}

	// 3. 管理员和超级用户拥有全部权限，其他用户读取角色绑定权限。
	permissionQuery := `
		SELECT DISTINCT permission.code
		FROM aowugong_user_roles user_role
		JOIN aowugong_roles role ON role.id = user_role.role_id AND role.is_active = 1
		JOIN aowugong_role_permissions role_permission ON role_permission.role_id = role.id
		JOIN aowugong_permissions permission ON permission.id = role_permission.permission_id
		WHERE user_role.user_id = ?
		ORDER BY permission.code
	`
	permissionArgs := []any{userID}
	if user.IsSuperuser || containsString(roles, "admin") {
		permissionQuery = `SELECT code FROM aowugong_permissions ORDER BY code`
		permissionArgs = nil
	}
	permissions, err := queryStrings(ctx, r.db, permissionQuery, permissionArgs...)
	if err != nil {
		return Profile{}, fmt.Errorf("查询用户权限: %w", err)
	}
	return Profile{User: user, Roles: roles, Permissions: permissions}, nil
}

// queryStrings 执行单列字符串查询并返回完整结果。
// 输入：ctx 是调用上下文，db 是 MySQL 连接，query 和 args 是参数化查询。
// 输出：返回结果字符串列表。
// 副作用：读取 MySQL。
func queryStrings(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	// 1. 执行查询并确保游标被释放。
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 2. 逐行扫描单列字符串。
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// containsString 判断字符串列表是否包含目标值。
// 输入：values 是候选列表，target 是目标值。
// 输出：找到目标值时返回 true。
// 副作用：无。
func containsString(values []string, target string) bool {
	// 1. 顺序检查已排序的小型角色列表。
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
