package rbac

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrUserNotFound = errors.New("用户不存在")
	ErrRoleNotFound = errors.New("角色不存在")
)

// Service 统一处理默认权限同步、角色分配和权限查询。
type Service struct {
	repository *Repository
}

// NewService 创建 RBAC 服务。
// 输入：repository 是 RBAC SQLite 仓储。
// 输出：返回可供 HTTP 和应用启动流程共用的服务。
// 副作用：无。
func NewService(repository *Repository) *Service {
	// 1. 保存应用层显式注入的仓储。
	return &Service{repository: repository}
}

// SyncDefaults 幂等同步系统角色、权限和默认关系。
// 输入：ctx 是调用上下文。
// 输出：成功返回 nil，失败返回带业务上下文的错误。
// 副作用：写入 SQLite。
func (s *Service) SyncDefaults(ctx context.Context) error {
	// 1. 以代码定义作为唯一基线同步全部默认数据。
	if err := s.repository.SyncDefaults(ctx, DefaultPermissions, DefaultRoles); err != nil {
		return fmt.Errorf("同步默认角色权限: %w", err)
	}
	return nil
}

// AssignRole 幂等地给用户添加角色。
// 输入：ctx 是调用上下文，userID 是用户主键，roleCode 是角色编码。
// 输出：成功返回 nil；用户或角色不存在时返回业务错误。
// 副作用：写入 SQLite 用户角色关联。
func (s *Service) AssignRole(ctx context.Context, userID int64, roleCode string) error {
	// 1. 委托仓储完成存在性判断和幂等关联写入。
	if err := s.repository.AssignRole(ctx, userID, roleCode); err != nil {
		return fmt.Errorf("分配用户角色: %w", err)
	}
	return nil
}

// PermissionsForUser 返回用户获得的全部权限编码。
// 输入：ctx 是调用上下文，userID 是用户主键。
// 输出：返回按编码排序的权限列表。
// 副作用：读取 SQLite。
func (s *Service) PermissionsForUser(ctx context.Context, userID int64) ([]string, error) {
	// 1. 使用仓储的统一权限展开逻辑。
	permissions, err := s.repository.PermissionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("读取用户权限: %w", err)
	}
	return permissions, nil
}

// HasPermission 判断用户是否拥有指定权限。
// 输入：ctx 是调用上下文，userID 是用户主键，permissionCode 是权限编码。
// 输出：拥有权限时返回 true。
// 副作用：读取 SQLite。
func (s *Service) HasPermission(ctx context.Context, userID int64, permissionCode string) (bool, error) {
	// 1. 展开用户权限并在小型有序集合中查找目标编码。
	permissions, err := s.PermissionsForUser(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, permission := range permissions {
		if permission == permissionCode {
			return true, nil
		}
	}
	return false, nil
}

// HasRole 判断用户是否拥有指定角色。
// 输入：ctx 是调用上下文，userID 是用户主键，roleCode 是角色编码。
// 输出：拥有角色时返回 true；用户不存在或查询失败时返回错误。
// 副作用：读取 SQLite。
func (s *Service) HasRole(ctx context.Context, userID int64, roleCode string) (bool, error) {
	// 1. 委托仓储执行唯一角色判断逻辑。
	result, err := s.repository.HasRole(ctx, userID, roleCode)
	if err != nil {
		return false, fmt.Errorf("检查用户角色: %w", err)
	}
	return result, nil
}

// ListRoles 同步基线后返回可分配的启用角色。
// 输入：ctx 是调用上下文。
// 输出：返回按主键排序的角色列表。
// 副作用：读写 SQLite。
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	// 1. 每次读取前同步代码基线，保持与旧接口行为一致。
	if err := s.SyncDefaults(ctx); err != nil {
		return nil, err
	}
	roles, err := s.repository.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("列出启用角色: %w", err)
	}
	return roles, nil
}

// ListUsers 返回权限管理页面使用的用户角色列表。
// 输入：ctx 是调用上下文。
// 输出：返回按用户名排序的用户列表。
// 副作用：读取 SQLite。
func (s *Service) ListUsers(ctx context.Context) ([]UserRoles, error) {
	// 1. 从仓储读取用户和角色映射。
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("列出权限用户: %w", err)
	}
	return users, nil
}
