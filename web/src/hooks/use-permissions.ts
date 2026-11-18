"use client";

import * as React from "react";

import { api } from "@/lib/api";
import type { RbacMe } from "@/lib/types";

let cache: RbacMe | null = null;
let inflight: Promise<RbacMe> | null = null;

// usePermissions 拉取当前用户的 RBAC 画像。页面级导航过滤使用；
// 后端仍是最终权威（未映射/无权限一律 403）。
export function usePermissions() {
  const [me, setMe] = React.useState<RbacMe | null>(cache);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        if (!cache) {
          inflight = inflight ?? api.rbacMe();
          cache = await inflight;
          inflight = null;
        }
        if (alive) setMe(cache);
      } catch (e) {
        if (alive) setError((e as Error).message || "加载权限失败");
      }
    };
    void load();
    return () => {
      alive = false;
    };
  }, []);

  const permissions = React.useMemo(() => (me ? new Set(me.permissions) : null), [me]);
  const isAdmin = React.useMemo(() => !!me?.roles.some((r) => r.id === "admin"), [me]);

  return { me, permissions, isAdmin, error, refresh: () => { cache = null; } };
}
