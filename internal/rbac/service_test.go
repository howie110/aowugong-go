package rbac

import (
	"context"
	"database/sql"
	"testing"

	"github.com/howiedata/aowugong-go/internal/testdatabase"
)

// TestServiceSyncDefaultsIsIdempotent 验证默认角色和权限按 code 幂等同步。
// 输入：空临时 SQLite 和两次默认同步。
// 输出：角色与权限数量稳定且无重复。
// 副作用：创建并写入临时 SQLite。
func TestServiceSyncDefaultsIsIdempotent(t *testing.T) {
	// 1. 连续同步两次默认数据。
	service, db := newTestService(t)
	if err := service.SyncDefaults(context.Background()); err != nil {
		t.Fatalf("first SyncDefaults() error = %v", err)
	}
	if err := service.SyncDefaults(context.Background()); err != nil {
		t.Fatalf("second SyncDefaults() error = %v", err)
	}

	// 2. 默认权限和角色不应重复写入。
	var roleCount, permissionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM aowugong_roles`).Scan(&roleCount); err != nil {
		t.Fatalf("count roles error = %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM aowugong_permissions`).Scan(&permissionCount); err != nil {
		t.Fatalf("count permissions error = %v", err)
	}
	if roleCount != len(DefaultRoles) || permissionCount != len(DefaultPermissions) {
		t.Errorf("counts = roles:%d permissions:%d, want roles:%d permissions:%d", roleCount, permissionCount, len(DefaultRoles), len(DefaultPermissions))
	}
}

// TestServiceAdminHasEveryPermission 验证 admin 角色天然拥有全部权限。
// 输入：分配 admin 角色的测试用户。
// 输出：用户拥有任意默认页面权限。
// 副作用：创建并写入临时 SQLite。
func TestServiceAdminHasEveryPermission(t *testing.T) {
	// 1. 同步默认 RBAC 数据并创建管理员用户。
	service, db := newTestService(t)
	if err := service.SyncDefaults(context.Background()); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	userID := insertTestUser(t, db, "admin-user", "admin@example.com")
	if err := service.AssignRole(context.Background(), userID, AdminRoleCode); err != nil {
		t.Fatalf("AssignRole() error = %v", err)
	}

	// 2. 管理员权限集合必须覆盖所有默认权限。
	permissions, err := service.PermissionsForUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("PermissionsForUser() error = %v", err)
	}
	if len(permissions) != len(DefaultPermissions) {
		t.Errorf("permission count = %d, want %d", len(permissions), len(DefaultPermissions))
	}
	hasAdminRole, err := service.HasRole(context.Background(), userID, AdminRoleCode)
	if err != nil || !hasAdminRole {
		t.Errorf("HasRole(admin) = %v, %v, want true", hasAdminRole, err)
	}
}

// TestServiceInvestorGetsOnlyArticleAnalysis 验证 investor 仅有文章分析权限。
// 输入：分配 investor 角色的测试用户。
// 输出：文章分析权限通过，管理权限被拒绝。
// 副作用：创建并写入临时 SQLite。
func TestServiceInvestorGetsOnlyArticleAnalysis(t *testing.T) {
	// 1. 同步默认 RBAC 数据并创建投资者用户。
	service, db := newTestService(t)
	if err := service.SyncDefaults(context.Background()); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	userID := insertTestUser(t, db, "investor-user", "investor@example.com")
	if err := service.AssignRole(context.Background(), userID, InvestorRoleCode); err != nil {
		t.Fatalf("AssignRole() error = %v", err)
	}

	// 2. 投资者只能获得文章分析页面权限。
	permissions, err := service.PermissionsForUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("PermissionsForUser() error = %v", err)
	}
	if len(permissions) != 1 || permissions[0] != PermissionFinanceArticleAnalysis {
		t.Errorf("permissions = %v, want [%s]", permissions, PermissionFinanceArticleAnalysis)
	}
	hasAdminRole, err := service.HasRole(context.Background(), userID, AdminRoleCode)
	if err != nil || hasAdminRole {
		t.Errorf("HasRole(admin) = %v, %v, want false", hasAdminRole, err)
	}
}

// TestServiceVPNUserGetsOnlyAssignedResources 验证 VPN 用户只能查看分配给自己的 VPN 资源。
// 输入：分配 VPN 用户角色的测试用户。
// 输出：仅包含 VPN 资源页面权限，不包含管理员分配权限。
// 副作用：创建并写入临时 SQLite。
func TestServiceVPNUserGetsOnlyAssignedResources(t *testing.T) {
	// 1. 同步默认 RBAC 数据并创建 VPN 用户。
	service, db := newTestService(t)
	if err := service.SyncDefaults(context.Background()); err != nil {
		t.Fatalf("SyncDefaults() error = %v", err)
	}
	userID := insertTestUser(t, db, "vpn-user", "vpn@example.com")
	if err := service.AssignRole(context.Background(), userID, VPNUserRoleCode); err != nil {
		t.Fatalf("AssignRole() error = %v", err)
	}

	// 2. VPN 用户只能进入资源页，不能进入管理员分配页。
	permissions, err := service.PermissionsForUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("PermissionsForUser() error = %v", err)
	}
	if len(permissions) != 1 || permissions[0] != PermissionVPNResources {
		t.Errorf("permissions = %v, want [%s]", permissions, PermissionVPNResources)
	}
	hasDistribution, err := service.HasPermission(context.Background(), userID, PermissionVPNDistribution)
	if err != nil || hasDistribution {
		t.Errorf("HasPermission(vpn distribution) = %v, %v, want false", hasDistribution, err)
	}
}

// newTestService 创建使用隔离 SQLite 数据库的 RBAC 服务。
// 输入：t 管理临时数据库生命周期。
// 输出：返回 RBAC 服务和底层 SQLite。
// 副作用：创建、迁移并注册清理临时 SQLite。
func newTestService(t *testing.T) (*Service, *sql.DB) {
	// 1. 打开并迁移独立的 SQLite 测试 schema。
	t.Helper()
	db := testdatabase.Open(t)

	// 2. 组装 RBAC 仓储和服务。
	return NewService(NewRepository(db)), db
}

// insertTestUser 写入供 RBAC 场景使用的用户记录。
// 输入：测试句柄、数据库、用户名和邮箱。
// 输出：返回新用户主键。
// 副作用：向临时 SQLite 插入用户。
func insertTestUser(t *testing.T, db *sql.DB, username, email string) int64 {
	// 1. 写入无角色的活动用户并返回自增标识。
	t.Helper()
	result, err := db.Exec(`INSERT INTO aowugong_fastapi_users (username, email, password, is_active) VALUES (?, ?, ?, 1)`, username, email, "ignored")
	if err != nil {
		t.Fatalf("insert user error = %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() error = %v", err)
	}
	return userID
}
