const REMEMBERED_CREDENTIALS_KEY = "aowugong_remembered_credentials";

export type RememberedCredentials = {
  username: string;
  password: string;
};

type CredentialStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
type CredentialStorageProvider = { readonly localStorage: CredentialStorage };

/** 安全取得浏览器本地存储对象，访问被拒绝时返回空值。 */
export function getCredentialStorage(provider: CredentialStorageProvider): CredentialStorage | null {
  // 1. 某些隐私策略会在读取 localStorage 属性时直接抛出安全异常
  try {
    return provider.localStorage;
  } catch {
    return null;
  }
}

/** 判断浏览器记录是否包含可用的用户名和密码。 */
function isRememberedCredentials(value: unknown): value is RememberedCredentials {
  if (!value || typeof value !== "object") {
    return false;
  }

  const credentials = value as Partial<RememberedCredentials>;
  return typeof credentials.username === "string" && typeof credentials.password === "string";
}

/** 读取浏览器中保存的登录凭据，损坏记录会被自动清除。 */
export function getRememberedCredentials(storage: CredentialStorage | null): RememberedCredentials | null {
  // 1. 浏览器禁用本地存储时按没有保存记录处理
  if (!storage) {
    return null;
  }
  let rawValue: string | null;
  try {
    rawValue = storage.getItem(REMEMBERED_CREDENTIALS_KEY);
  } catch {
    return null;
  }
  if (!rawValue) {
    return null;
  }

  // 2. 只恢复字段完整的 JSON 凭据
  try {
    const credentials: unknown = JSON.parse(rawValue);
    if (isRememberedCredentials(credentials)) {
      return credentials;
    }
  } catch {
    // 非 JSON 内容与字段异常都按无效记录处理。
  }

  // 3. 损坏记录尽量清除，清除失败不影响登录页加载
  clearRememberedCredentials(storage);
  return null;
}

/** 把用户名和明文密码保存到浏览器本地存储。 */
export function saveRememberedCredentials(storage: CredentialStorage | null, credentials: RememberedCredentials) {
  // 1. 记住密码是可选能力，存储失败不能改变登录结果
  if (!storage) {
    return;
  }
  try {
    storage.setItem(REMEMBERED_CREDENTIALS_KEY, JSON.stringify(credentials));
  } catch {
    return;
  }
}

/** 清除浏览器中保存的登录凭据。 */
export function clearRememberedCredentials(storage: CredentialStorage | null) {
  // 1. 浏览器拒绝访问存储时保持静默，不阻断当前操作
  if (!storage) {
    return;
  }
  try {
    storage.removeItem(REMEMBERED_CREDENTIALS_KEY);
  } catch {
    return;
  }
}
