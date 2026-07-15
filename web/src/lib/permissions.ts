import { getToken } from "@/lib/auth";

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
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const response = await fetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...(options.headers || {}),
    },
  });

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "请求失败");
  }

  return (await response.json()) as T;
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
