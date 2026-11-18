"use client";

import * as React from "react";

import { CheckIcon, RefreshCwIcon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";
import type { Task } from "@/lib/types";

type Proposed = {
  id: number;
  summary?: string;
  priority?: number;
  pending_executor?: string;
  review_note?: string;
  content_version: number;
  created_at: number;
};

// 审核面板：任务级开关 + proposed 待审列表 + 人工批准/驳回。
// AI 预审、审核对话在后续版本接入。
export function ReviewPanel({ task, onTaskChange }: { task: Task; onTaskChange?: (t: Task) => void }) {
  const [enabled, setEnabled] = React.useState(!!task.review_enabled);
  const [auto, setAuto] = React.useState(!!task.review_auto);
  const [items, setItems] = React.useState<Proposed[]>([]);
  const [loading, setLoading] = React.useState(false);

  const load = React.useCallback(async () => {
    if (!enabled) {
      setItems([]);
      return;
    }
    setLoading(true);
    try {
      setItems(await api.reviewProposed(task.id));
    } catch (e) {
      toast.error((e as Error).message || "加载待审意图失败");
    } finally {
      setLoading(false);
    }
  }, [enabled, task.id]);

  React.useEffect(() => {
    void load();
    const timer = window.setInterval(() => { void load(); }, 10_000);
    return () => window.clearInterval(timer);
  }, [load]);

  async function saveConfig(nextEnabled: boolean, nextAuto: boolean) {
    try {
      await api.reviewConfig(task.id, nextEnabled, nextAuto);
      setEnabled(nextEnabled);
      setAuto(nextAuto);
      onTaskChange?.({ ...task, review_enabled: nextEnabled, review_auto: nextAuto });
      toast.success("审核配置已保存");
      await load();
    } catch (e) {
      toast.error((e as Error).message || "保存失败");
    }
  }

  async function act(id: number, version: number, approve: boolean) {
    try {
      if (approve) await api.reviewApprove(task.id, id, version);
      else await api.reviewReject(task.id, id, version, "人工驳回");
      toast.success(approve ? "已批准" : "已驳回");
      await load();
    } catch (e) {
      toast.error((e as Error).message || "操作失败");
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div>
            <CardTitle>意图审核</CardTitle>
            <CardDescription>
              开启后 planner 必须通过 <code>add_review_intent</code> 送审，批准前 worker 不会执行。
              内网渗透/权限提升/漏洞利用场景默认开启。
            </CardDescription>
          </div>
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? "已开启" : "未开启"}</Badge>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center gap-4 text-sm">
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={enabled} onChange={(e) => void saveConfig(e.target.checked, auto)} />
            开启审核
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={auto} disabled={!enabled} onChange={(e) => void saveConfig(enabled, e.target.checked)} />
            自动处置（AI 预审通过后自动批准；当前仅保留开关）
          </label>
          <Button size="sm" variant="outline" onClick={load} disabled={!enabled || loading}>
            <RefreshCwIcon className="size-4" />
            刷新
          </Button>
        </CardContent>
      </Card>

      {enabled && (
        <Card>
          <CardHeader>
            <CardTitle>待审意图</CardTitle>
            <CardDescription>共 {items.length} 条。批准后转为 open，worker 立即可以领取。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {loading ? (
              <p className="py-6 text-center text-muted-foreground text-sm">加载中…</p>
            ) : items.length === 0 ? (
              <p className="py-6 text-center text-muted-foreground text-sm">暂无待审意图</p>
            ) : (
              items.map((it) => (
                <div key={it.id} className="rounded-lg border p-3">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="outline">#{it.id}</Badge>
                        <Badge variant="secondary">P{it.priority ?? 5}</Badge>
                        {it.pending_executor && <Badge variant="outline">执行器 {it.pending_executor}</Badge>}
                      </div>
                      <p className="mt-2 whitespace-pre-wrap text-sm">{it.summary || "—"}</p>
                      {it.review_note && <p className="mt-1 text-muted-foreground text-xs">预审：{it.review_note}</p>}
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button size="sm" onClick={() => void act(it.id, it.content_version, true)}>
                        <CheckIcon className="size-4" />
                        批准
                      </Button>
                      <Button size="sm" variant="outline" onClick={() => void act(it.id, it.content_version, false)}>
                        <XIcon className="size-4" />
                        驳回
                      </Button>
                    </div>
                  </div>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
