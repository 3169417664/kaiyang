"use client";

import { useEffect, useState } from "react";

import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

// 首次登录强制改密：迁移过来的 admin 可能沿用旧弱密码，后端会阻塞除 /api/auth/* 外
// 的所有接口，直到这里改密成功。
export default function ForceChangePasswordPage() {
  const router = useRouter();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!auth.getToken()) router.replace("/login");
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!currentPassword || !newPassword) {
      setError("请填写当前密码与新密码");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("两次输入的新密码不一致");
      return;
    }
    setLoading(true);
    try {
      const res = await api.changePassword(currentPassword, newPassword);
      if (res.token) auth.setToken(res.token);
      router.replace("/function/tasks");
    } catch (err) {
      setError((err as Error)?.message || "修改密码失败");
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-background p-8">
      <div className="w-full max-w-md space-y-8">
        <div className="space-y-4 text-center">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/brand/kaiyang-mark.svg" alt="KAIYANG" width={64} height={64} className="mx-auto" />
          <div className="space-y-1.5">
            <h2 className="text-2xl font-medium tracking-tight">首次登录需修改密码</h2>
            <p className="text-muted-foreground">为保障平台安全，请先设置一个新的管理员密码。</p>
          </div>
        </div>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="space-y-1.5">
            <Label htmlFor="fcp-current">当前密码</Label>
            <Input
              id="fcp-current"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              disabled={loading}
              onChange={(e) => setCurrentPassword(e.target.value)}
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="fcp-new">新密码</Label>
            <Input
              id="fcp-new"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              disabled={loading}
              onChange={(e) => setNewPassword(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="fcp-confirm">确认新密码</Label>
            <Input
              id="fcp-confirm"
              type="password"
              autoComplete="new-password"
              value={confirmPassword}
              disabled={loading}
              onChange={(e) => setConfirmPassword(e.target.value)}
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? "提交中…" : "修改密码并进入平台"}
          </Button>
        </form>
      </div>
    </div>
  );
}
