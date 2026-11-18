const TOKEN_KEY = "kaiyang_token";
const LEGACY_TOKEN_KEY = "pojun_token";
const COOKIE_MAX_AGE = 8 * 60 * 60; // 480 分钟（秒）

export interface CurrentUser {
  id: string;
  name: string;
  username: string;
  email: string;
  avatar: string;
  role: string;
}

export const auth = {
  getToken(): string | null {
    if (typeof window === "undefined") return null;
    // 兼容旧版 pojun_token；读到后下次写入会迁移到 kaiyang_token。
    const token = localStorage.getItem(TOKEN_KEY) ?? localStorage.getItem(LEGACY_TOKEN_KEY);
    return token ?? (process.env.NEXT_PUBLIC_MOCK === "1" ? "mock-demo" : null);
  },

  setToken(token: string): void {
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.removeItem(LEGACY_TOKEN_KEY);
    document.cookie = `${TOKEN_KEY}=${encodeURIComponent(token)}; path=/; max-age=${COOKIE_MAX_AGE}; SameSite=Lax`;
    document.cookie = `${LEGACY_TOKEN_KEY}=; path=/; max-age=0`;
  },

  clearToken(): void {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(LEGACY_TOKEN_KEY);
    document.cookie = `${TOKEN_KEY}=; path=/; max-age=0`;
    document.cookie = `${LEGACY_TOKEN_KEY}=; path=/; max-age=0`;
  },

  // 从 JWT payload 解析展示信息；username 优先取 usr，旧 token 回退 sub。
  getCurrentUser(): CurrentUser | null {
    const token = this.getToken();
    if (!token) return null;
    try {
      const parts = token.split(".");
      if (parts.length !== 3) return null;
      const payload = JSON.parse(atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")));
      const username: string = payload.usr ?? payload.sub ?? "admin";
      return { id: payload.sub ?? username, name: username, username, email: "", avatar: "", role: "" };
    } catch {
      return null;
    }
  },
};
