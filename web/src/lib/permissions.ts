import type { UserProfile } from "@/lib/auth";
import { pagePermissionMap, type FinancePageKey } from "@/lib/finance";
import { requestJSON } from "@/lib/request";

export type RoleRead = {
  id: number;
  code: string;
  name: string;
  description?: string;
  is_active: boolean;
  is_system: boolean;
};

export type PermissionUser = {
  id: number;
  username: string;
  email: string;
  is_active: boolean;
  roles: string[];
};

async function requestJson<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  return requestJSON<T>(url, { ...options, headers });
}

export function fetchPermissionUsers() {
  return requestJson<PermissionUser[]>("/api/v1/permissions/users");
}

export function fetchPermissionRoles() {
  return requestJson<RoleRead[]>("/api/v1/permissions/roles");
}

export function addRoleToUser(userId: number, roleCode: string) {
  return requestJson<PermissionUser>(`/api/v1/permissions/users/${userId}/roles`, {
    method: "POST",
    body: JSON.stringify({ role_code: roleCode }),
  });
}

// 只允许管理员访问的页面，dashboard 与侧栏共用同一份策略。
export const adminOnlyPages = new Set<FinancePageKey>([
  "trajectory",
  "chatArchive",
  "analytics",
  "receiptSplit",
]);

/** canAccessPage 判断用户是否可以访问指定工作台页面。 */
export function canAccessPage(user: UserProfile | null, pageKey: FinancePageKey): boolean {
  const roles = user?.roles ?? [];
  if (adminOnlyPages.has(pageKey)) {
    return roles.includes("admin");
  }
  if (roles.includes("admin")) {
    return true;
  }
  return (user?.permissions ?? []).includes(pagePermissionMap[pageKey]);
}
