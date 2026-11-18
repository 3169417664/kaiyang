"use client";

import * as React from "react";

import { RefreshCwIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";
import type { IntranetCredential } from "@/lib/types";

export default function CredentialsPage() {
  const [rows, setRows] = React.useState<IntranetCredential[]>([]);
  const [total, setTotal] = React.useState(0);
  const [loading, setLoading] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.intranetCredentials({ limit: 200 });
      setRows(r.credentials ?? []); setTotal(r.total ?? 0);
    } catch (e) { toast.error((e as Error).message || "加载凭据失败"); }
    finally { setLoading(false); }
  }, []);
  React.useEffect(() => { void load(); }, [load]);

  return (
    <div className="space-y-6 p-4 lg:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">内网凭据</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            仅展示 RFC1918 内网资产上的 discovered / verified 关系；secret 永不回显。
          </p>
        </div>
        <Button variant="outline" onClick={load} disabled={loading}><RefreshCwIcon className="size-4" /> 刷新</Button>
      </div>

      <Card>
        <CardHeader><CardTitle>凭据关系</CardTitle><CardDescription>共 {total} 条。</CardDescription></CardHeader>
        <CardContent className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="border-b text-muted-foreground"><tr className="text-left">
              <th className="py-2 pr-4 font-medium">资产</th><th className="py-2 pr-4 font-medium">类型</th><th className="py-2 pr-4 font-medium">账号</th><th className="py-2 pr-4 font-medium">域</th><th className="py-2 pr-4 font-medium">目标</th><th className="py-2 pr-4 font-medium">关系</th>
            </tr></thead>
            <tbody>
              {loading ? <tr><td colSpan={6} className="py-8 text-center text-muted-foreground">加载中…</td></tr>
                : rows.length === 0 ? <tr><td colSpan={6} className="py-8 text-center text-muted-foreground">暂无内网凭据</td></tr>
                : rows.map((r, i) => (
                  <tr key={`${r.credential_id}-${r.asset_id}-${i}`} className="border-b last:border-0">
                    <td className="py-2 pr-4 font-mono">{r.asset_name || `#${r.asset_id}`}</td>
                    <td className="py-2 pr-4">{r.cred_type}</td>
                    <td className="py-2 pr-4 font-mono">{r.username || "—"}</td>
                    <td className="py-2 pr-4">{r.domain || "—"}</td>
                    <td className="py-2 pr-4">{r.target || "—"}</td>
                    <td className="py-2 pr-4"><Badge variant={r.relation === "verified" ? "secondary" : "outline"}>{r.relation}</Badge></td>
                  </tr>
                ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
