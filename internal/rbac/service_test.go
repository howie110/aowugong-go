package rbac

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/howiedata/aowugong-go/internal/config"
	"github.com/howiedata/aowugong-go/internal/database"
)

// TestServiceSyncDefaultsIsIdempotent 验证默认角色和权限按 code 幂等同步。
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
	if roleCount != 2 || permissionCount != len(DefaultPermissions) {
		t.Errorf("counts = roles:%d permissions:%d, want roles:2 permissions:%d", roleCount, permissionCount, len(DefaultPermissions))
	}
}

// TestServiceAdminHasEveryPermission 验证 admin 角色天然拥有全部权限。
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
}

// TestServiceInvestorGetsOnlyArticleAnalysis 验证 investor 仅有文章分析权限。
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
}

// newTestService 创建使用临时 SQLite 数据库的 RBAC 服务。
func newTestService(t *testing.T) (*Service, *sql.DB) {
	// 1. 打开并迁移独立的临时数据库。
	t.Helper()
	db, err := database.OpenSQLite(context.Background(), config.Database{Path: filepath.Join(t.TempDir(), "rbac.db")})
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(context.Background(), db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// 2. 组装 RBAC 仓储和服务。
	return NewService(NewRepository(db)), db
}

// insertTestUser 写入供 RBAC 场景使用的用户记录。
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
