"use client";

import { useEffect, useState } from "react";

import { api } from "@/lib/api";
import { auth, type CurrentUser } from "@/lib/auth";

const FALLBACK: CurrentUser = { id: "", name: "", username: "", email: "", avatar: "", role: "" };

export function useCurrentUser(): CurrentUser {
  const [user, setUser] = useState<CurrentUser>(FALLBACK);

  useEffect(() => {
    if (!auth.getToken()) return;
    let cancelled = false;
    api
      .rbacMe()
      .then((me) => {
        if (cancelled || !me?.user) return;
        const role = me.roles?.[0]?.name || me.roles?.[0]?.id || "";
        setUser({
          id: me.user.id,
          name: me.user.display_name || me.user.username,
          username: me.user.username,
          email: "",
          avatar: "",
          role,
        });
      })
      .catch(() => {
        // 401 由 api 层统一跳转登录，这里不需要重复提示。
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return user;
}
