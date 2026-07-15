package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository 负责 SQLite 中的角色、权限和关联关系。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建 RBAC 仓储。
// 输入：db 是已完成迁移的 SQLite 连接池。
// 输出：返回 RBAC 仓储。
// 副作用：无。
func NewRepository(db *sql.DB) *Repository {
	// 1. 保存应用层显式注入的数据库连接。
	return &Repository{db: db}
}

// SyncDefaults 幂等同步系统角色、页面权限及默认绑定。
// 输入：ctx 是调用上下文，permissions 和 roles 是代码基线。
// 输出：成功返回 nil。
// 副作用：在单个事务中写入 SQLite。
func (r *Repository) SyncDefaults(ctx context.Context, permissions []Permission, roles []Role) error {
	// 1. 开启事务，避免角色和权限处于半同步状态。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 RBAC 同步事务: %w", err)
	}
	defer tx.Rollback()

	// 2. 按 code 更新或创建全部系统权限。
	for _, permission := range permissions {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO aowugong_permissions (code, name, "group", description)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = excluded.name,
				"group" = excluded."group",
				description = excluded.description
		`, permission.Code, permission.Name, permission.Group, permission.Description)
		if err != nil {
			return fmt.Errorf("同步权限 %s: %w", permission.Code, err)
		}
	}

	// 3. 按 code 更新或创建全部系统角色。
	for _, role := range roles {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO aowugong_roles (code, name, description, is_active, is_system)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				is_active = excluded.is_active,
				is_system = excluded.is_system
		`, role.Code, role.Name, role.Description, role.IsActive, role.IsSystem)
		if err != nil {
			return fmt.Errorf("同步角色 %s: %w", role.Code, err)
		}
	}

	// 4. 重建两个系统角色的权限绑定，确保代码基线是唯一来源。
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM aowugong_role_permissions
		WHERE role_id IN (SELECT id FROM aowugong_roles WHERE code IN (?, ?))
	`, AdminRoleCode, InvestorRoleCode); err != nil {
		return fmt.Errorf("清理默认角色权限: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO aowugong_role_permissions (role_id, permission_id)
		SELECT role.id, permission.id
		FROM aowugong_roles role
		CROSS JOIN aowugong_permissions permission
		WHERE role.code = ?
	`, AdminRoleCode); err != nil {
		return fmt.Errorf("绑定管理员权限: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO aowugong_role_permissions (role_id, permission_id)
		SELECT role.id, permission.id
		FROM aowugong_roles role
		JOIN aowugong_permissions permission ON permission.code = ?
		WHERE role.code = ?
	`, PermissionFinanceArticleAnalysis, InvestorRoleCode); err != nil {
		return fmt.Errorf("绑定投资者权限: %w", err)
	}

	// 5. 原子提交完整 RBAC 基线。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 RBAC 同步事务: %w", err)
	}
	return nil
}

// AssignRole 幂等地给用户分配一个启用角色。
// 输入：ctx 是调用上下文，userID 是用户主键，roleCode 是角色编码。
// 输出：用户或角色不存在时返回对应业务错误。
// 副作用：写入 SQLite 用户角色关联。
func (r *Repository) AssignRole(ctx context.Context, userID int64, roleCode string) error {
	// 1. 确认用户存在，避免把外键错误暴露给服务层。
	var userExists int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM aowugong_fastapi_users WHERE id = ?`, userID).Scan(&userExists); err != nil {
		return fmt.Errorf("检查待分配角色用户: %w", err)
	}
	if userExists == 0 {
		return ErrUserNotFound
	}

	// 2. 查找启用角色并幂等写入关联。
	result, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO aowugong_user_roles (user_id, role_id)
		SELECT ?, id FROM aowugong_roles WHERE code = ? AND is_active = 1
	`, userID, roleCode)
	if err != nil {
		return fmt.Errorf("给用户分配角色: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取角色分配结果: %w", err)
	}
	if rowsAffected == 0 {
		var assigned int
		err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM aowugong_user_roles user_role
			JOIN aowugong_roles role ON role.id = user_role.role_id
			WHERE user_role.user_id = ? AND role.code = ? AND role.is_active = 1
		`, userID, roleCode).Scan(&assigned)
		if err != nil {
			return fmt.Errorf("检查已有角色分配: %w", err)
		}
		if assigned == 0 {
			return ErrRoleNotFound
		}
	}
	return nil
}

// PermissionsForUser 返回用户通过启用角色获得的全部权限。
// 输入：ctx 是调用上下文，userID 是用户主键。
// 输出：返回按 code 排序的权限；用户不存在时返回 ErrUserNotFound。
// 副作用：读取 SQLite。
func (r *Repository) PermissionsForUser(ctx context.Context, userID int64) ([]string, error) {
	// 1. 确认用户存在并读取超级用户标记。
	var isSuperuser bool
	if err := r.db.QueryRowContext(ctx, `SELECT is_superuser FROM aowugong_fastapi_users WHERE id = ?`, userID).Scan(&isSuperuser); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, fmt.Errorf("查询用户权限身份: %w", err)
	}

	// 2. 超级用户或 admin 角色直接读取全部权限，其他用户读取关联权限。
	query := `
		SELECT DISTINCT permission.code
		FROM aowugong_user_roles user_role
		JOIN aowugong_roles role ON role.id = user_role.role_id AND role.is_active = 1
		JOIN aowugong_role_permissions relation ON relation.role_id = role.id
		JOIN aowugong_permissions permission ON permission.id = relation.permission_id
		WHERE user_role.user_id = ?
		ORDER BY permission.code
	`
	args := []any{userID}
	if isSuperuser {
		query = `SELECT code FROM aowugong_permissions ORDER BY code`
		args = nil
	} else {
		var isAdmin int
		if err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM aowugong_user_roles user_role
			JOIN aowugong_roles role ON role.id = user_role.role_id
			WHERE user_role.user_id = ? AND role.code = ? AND role.is_active = 1
		`, userID, AdminRoleCode).Scan(&isAdmin); err != nil {
			return nil, fmt.Errorf("检查管理员角色: %w", err)
		}
		if isAdmin != 0 {
			query = `SELECT code FROM aowugong_permissions ORDER BY code`
			args = nil
		}
	}
	return scanStrings(ctx, r.db, query, args...)
}

// ListRoles 返回全部启用角色。
// 输入：ctx 是调用上下文。
// 输出：返回按主键排序的角色。
// 副作用：读取 SQLite。
func (r *Repository) ListRoles(ctx context.Context) ([]Role, error) {
	// 1. 查询权限页面可分配的启用角色。
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, code, name, description, is_active, is_system
		FROM aowugong_roles
		WHERE is_active = 1
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("查询启用角色: %w", err)
	}
	defer rows.Close()

	// 2. 扫描完整角色响应字段。
	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.IsActive, &role.IsSystem); err != nil {
			return nil, fmt.Errorf("扫描角色: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历角色: %w", err)
	}
	return roles, nil
}

// ListUsers 返回用户及其已分配角色。
// 输入：ctx 是调用上下文。
// 输出：返回按用户名排序的权限管理用户列表。
// 副作用：读取 SQLite。
func (r *Repository) ListUsers(ctx context.Context) ([]UserRoles, error) {
	// 1. 查询用户基础字段，角色由后续有界查询补齐。
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, email, is_active
		FROM aowugong_fastapi_users
		ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("查询权限用户: %w", err)
	}
	defer rows.Close()

	users := make([]UserRoles, 0)
	for rows.Next() {
		var user UserRoles
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.IsActive); err != nil {
			return nil, fmt.Errorf("扫描权限用户: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历权限用户: %w", err)
	}

	// 2. 按用户主键查询小型角色集合并保持排序。
	for index := range users {
		roles, err := scanStrings(ctx, r.db, `
			SELECT role.code
			FROM aowugong_user_roles user_role
			JOIN aowugong_roles role ON role.id = user_role.role_id
			WHERE user_role.user_id = ?
			ORDER BY role.code
		`, users[index].ID)
		if err != nil {
			return nil, fmt.Errorf("查询用户 %d 角色: %w", users[index].ID, err)
		}
		users[index].Roles = roles
	}
	return users, nil
}

// scanStrings 扫描单列字符串查询结果。
// 输入：ctx 是调用上下文，db 是 SQLite 连接，query 和 args 是参数化查询。
// 输出：返回字符串列表。
// 副作用：读取 SQLite。
func scanStrings(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	// 1. 执行查询并确保释放游标。
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 2. 扫描全部字符串并检查游标错误。
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
