"use client";

import * as React from "react";

import { PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { TopologyData } from "@/lib/types";

export function TopologyTab({ taskId, categoryName }: { taskId: string; categoryName?: string }) {
  const [data, setData] = React.useState<TopologyData>({ nodes: [], edges: [], groups: [], notes: [] });
  const [ip, setIP] = React.useState("");
  const [hostname, setHostname] = React.useState("");
  const [label, setLabel] = React.useState("");
  const [busy, setBusy] = React.useState(false);

  const load = React.useCallback(async () => {
    try { setData(await api.topology(taskId)); }
    catch (e) { toast.error((e as Error).message || "加载拓扑失败"); }
  }, [taskId]);
  React.useEffect(() => { void load(); }, [load]);

  async function add() {
    if (!ip.trim()) { toast.error("IP 必填"); return; }
    setBusy(true);
    try {
      await api.upsertTopologyNode(taskId, { ip: ip.trim(), hostname: hostname.trim(), display_label: label.trim() });
      toast.success("节点已保存"); setIP(""); setHostname(""); setLabel(""); await load();
    } catch (e) { toast.error((e as Error).message || "保存失败"); } finally { setBusy(false); }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div>
            <CardTitle>内网拓扑</CardTitle>
            <CardDescription>
              任务内节点/边/分组/便签。建议仅在「内网渗透」任务使用；当前提供节点管理，图形画布在后续版本接入。
              {categoryName ? `（当前分类：${categoryName}）` : ""}
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={load}><RefreshCwIcon className="size-4" /> 刷新</Button>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-4">
          <div className="space-y-1.5"><Label>IP / 主机</Label><Input value={ip} onChange={(e) => setIP(e.target.value)} placeholder="10.0.0.5" /></div>
          <div className="space-y-1.5"><Label>主机名</Label><Input value={hostname} onChange={(e) => setHostname(e.target.value)} /></div>
          <div className="space-y-1.5"><Label>显示标签</Label><Input value={label} onChange={(e) => setLabel(e.target.value)} /></div>
          <div className="flex items-end"><Button onClick={add} disabled={busy}><PlusIcon className="size-4" /> 添加节点</Button></div>
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader><CardTitle>节点</CardTitle><CardDescription>共 {data.nodes.length} 个。</CardDescription></CardHeader>
          <CardContent className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="border-b text-muted-foreground"><tr className="text-left">
                <th className="py-2 pr-4 font-medium">IP</th><th className="py-2 pr-4 font-medium">主机名</th><th className="py-2 pr-4 font-medium">标签</th><th className="py-2 pr-4 font-medium">操作</th>
              </tr></thead>
              <tbody>
                {data.nodes.length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-muted-foreground">暂无节点</td></tr> : data.nodes.map((n) => (
                  <tr key={n.ip} className="border-b last:border-0">
                    <td className="py-2 pr-4 font-mono">{n.ip}</td>
                    <td className="py-2 pr-4">{n.hostname || "—"}</td>
                    <td className="py-2 pr-4">{n.display_label || "—"}</td>
                    <td className="py-2 pr-4">
                      <Button size="sm" variant="ghost" className="text-destructive" onClick={async () => {
                        if (!window.confirm(`删除节点 ${n.ip}？`)) return;
                        try { await api.deleteTopologyNode(taskId, n.ip); toast.success("已删除"); await load(); }
                        catch (e) { toast.error((e as Error).message || "删除失败"); }
                      }}><Trash2Icon className="size-4" /></Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>边 / 便签</CardTitle><CardDescription>{data.edges.length} 条边，{data.notes.length} 条便签。</CardDescription></CardHeader>
          <CardContent className="space-y-2 text-sm">
            {data.edges.length === 0 ? <p className="text-muted-foreground">暂无连接关系</p> : data.edges.map((e) => (
              <div key={e.id} className="flex items-center gap-2"><Badge variant="outline">{e.src_ip}</Badge> → <Badge variant="outline">{e.dst_ip}</Badge><span className="text-muted-foreground text-xs">{e.label}</span></div>
            ))}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
