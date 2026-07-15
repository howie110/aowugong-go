const TOKEN_KEY = "aowugong_token";

export type LoginResponse = {
  access_token: string;
  token_type: string;
};

export type UserProfile = {
  id: number;
  username: string;
  email?: string;
  is_active?: boolean;
  roles: string[];
  permissions: string[];
};

export function getToken() {
  return window.localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  window.localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  window.localStorage.removeItem(TOKEN_KEY);
}

export function isAuthenticated() {
  return Boolean(getToken());
}

export async function login(username: string, password: string): Promise<LoginResponse> {
  const formData = new URLSearchParams();
  formData.set("username", username);
  formData.set("password", password);

  const response = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: formData,
  });

  if (!response.ok) {
    const data = await response.json().catch(() => null);
    throw new Error(data?.detail || "登录失败");
  }

  const data = (await response.json()) as LoginResponse;
  setToken(data.access_token);
  return data;
}

export async function getProfile(): Promise<UserProfile> {
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const response = await fetch("/api/v1/auth/profile", {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok) {
    if (response.status === 401) {
      clearToken();
    }
    throw new Error("获取用户信息失败");
  }

  const data = (await response.json()) as UserProfile;
  return {
    ...data,
    roles: Array.isArray(data.roles) ? data.roles : [],
    permissions: Array.isArray(data.permissions) ? data.permissions : [],
  };
}
