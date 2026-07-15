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

export async function authorizedFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  const token = getToken();
  if (!token) {
    throw new Error("未登录");
  }

  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) {
    clearToken();
    window.location.href = "/login";
  }
  return response;
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
  const response = await authorizedFetch("/api/v1/auth/profile");

  if (!response.ok) {
    throw new Error("获取用户信息失败");
  }

  const data = (await response.json()) as UserProfile;
  return {
    ...data,
    roles: Array.isArray(data.roles) ? data.roles : [],
    permissions: Array.isArray(data.permissions) ? data.permissions : [],
  };
}
