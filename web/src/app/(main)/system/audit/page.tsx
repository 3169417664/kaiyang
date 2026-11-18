"use client";

import * as React from "react";

import { RefreshCwIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import type { AuditLog } from "@/lib/types";

export default function AuditPage() {
  const [logs, setLogs] = React.useState<AuditLog[]>([]);
  const [total, setTotal] = React.useState(0);
  const [category, setCategory] = React.useState("");
  const [actor, setActor] = React.useState("");
  const [result, setResult] = React.useState("");
  const [loading, setLoading] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.auditLogs({ category, actor, result, limit: 100 });
      setLogs(res.logs);
      setTotal(res.total);
    } catch (e) {
      toast.error((e as Error).message || "加载审计日志失败");
    } finally {
      setLoading(false);
    }
  }, [category, actor, result]);

  React.useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="space-y-6 p-4 lg:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">审计日志</h1>
          <p className="mt-1 text-muted-foreground text-sm">登录、用户管理、审批、配置变更与高危操作留痕。</p>
        </div>
        <Button variant="outline" onClick={load} disabled={loading}>
          <RefreshCwIcon className="size-4" />
          刷新
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>筛选</CardTitle>
          <CardDescription>共 {total} 条；默认显示最近 100 条。</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-3">
          <Input className="w-40" placeholder="category（如 auth）" value={category} onChange={(e) => setCategory(e.target.value)} />
          <Input className="w-40" placeholder="actor（用户名）" value={actor} onChange={(e) => setActor(e.target.value)} />
          <Input className="w-40" placeholder="result（success/fail）" value={result} onChange={(e) => setResult(e.target.value)} />
        </CardContent>
      </Card>

      <Card>
        <CardContent className="overflow-x-auto pt-6">
          <table className="w-full text-sm">
            <thead className="border-b text-muted-foreground">
              <tr className="text-left">
                <th className="py-2 pr-4 font-medium">时间</th>
                <th className="py-2 pr-4 font-medium">操作者</th>
                <th className="py-2 pr-4 font-medium">类别</th>
                <th className="py-2 pr-4 font-medium">动作</th>
                <th className="py-2 pr-4 font-medium">结果</th>
                <th className="py-2 pr-4 font-medium">来源 IP</th>
                <th className="py-2 pr-4 font-medium">说明</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={7} className="py-8 text-center text-muted-foreground">加载中…</td></tr>
              ) : logs.length === 0 ? (
                <tr><td colSpan={7} className="py-8 text-center text-muted-foreground">暂无日志</td></tr>
              ) : (
                logs.map((l) => (
                  <tr key={l.id} className="border-b last:border-0">
                    <td className="whitespace-nowrap py-2 pr-4 text-muted-foreground">{new Date(l.created_at).toLocaleString()}</td>
                    <td className="py-2 pr-4 font-mono">{l.actor || "—"}</td>
                    <td className="py-2 pr-4">{l.category || "—"}</td>
                    <td className="py-2 pr-4 font-mono">{l.action || "—"}</td>
                    <td className="py-2 pr-4">
                      <Badge variant={l.result === "success" ? "secondary" : "destructive"}>{l.result}</Badge>
                    </td>
                    <td className="py-2 pr-4 font-mono">{l.ip || "—"}</td>
                    <td className="max-w-md truncate py-2 pr-4" title={l.message}>{l.message || "—"}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
