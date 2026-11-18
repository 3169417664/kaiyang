const TOKEN_KEY = "kaiyang_token";
const LEGACY_TOKEN_KEY = "pojun_token";
const SESSION_MARKER = "kaiyang_authed";

export interface CurrentUser {
  id: string;
  name: string;
  username: string;
  email: string;
  avatar: string;
  role: string;
}

// 会话令牌由后端写入 HttpOnly Cookie，JS 读不到；localStorage 只保留一个
// 非敏感的“已登录”标记。展示信息由 /api/rbac/me 实时获取，不再落盘。
export const auth = {
  getToken(): string | null {
    if (typeof window === "undefined") return null;
    const authed = localStorage.getItem(SESSION_MARKER);
    if (authed === "1") return "cookie";
    // 兼容旧版本：首次加载把 localStorage 里的 JWT 用完即清（后端仍接受
    // Authorization，但新会话不再落盘）。
    const legacy = localStorage.getItem(TOKEN_KEY) ?? localStorage.getItem(LEGACY_TOKEN_KEY);
    if (legacy) {
      this.setToken(legacy);
      return "cookie";
    }
    return process.env.NEXT_PUBLIC_MOCK === "1" ? "mock-demo" : null;
  },

  // 保留签名以兼容调用点；token 参数不再落盘，只记一个登录标记。
  setToken(_token: string): void {
    localStorage.setItem(SESSION_MARKER, "1");
    // 迁移旧版落盘的 JWT：删除，不再由 JS 持有。
    // 注意：不要在这里用 document.cookie 清理同名 Cookie——后端刚下发的
    // 是 HttpOnly，JS 的写入会被浏览器拒绝，反而可能产生同名冲突。
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(LEGACY_TOKEN_KEY);
  },

  clearToken(): void {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(LEGACY_TOKEN_KEY);
    localStorage.removeItem(SESSION_MARKER);
    document.cookie = `${TOKEN_KEY}=; path=/; max-age=0`;
    document.cookie = `${LEGACY_TOKEN_KEY}=; path=/; max-age=0`;
    // 通知后端作废会话并清除 HttpOnly Cookie（失败也不阻塞本地登出）。
    try {
      void fetch("/api/auth/logout", { method: "POST", credentials: "same-origin" });
    } catch {
      // ignore
    }
  },

  // 展示信息改由 useCurrentUser 调用 /api/rbac/me 获取，避免把任何服务端
  // 返回数据持久化到 localStorage。
  getCurrentUser(): CurrentUser | null {
    return null;
  },
};
