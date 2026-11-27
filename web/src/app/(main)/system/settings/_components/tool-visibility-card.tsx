"use client";

import * as React from "react";

import { Loader2Icon, WrenchIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import type { Settings } from "@/lib/types";

/** 工具 schema 可见性设置：0=关闭（全量可见），20–40 启用 deferred + BM25 搜索。 */
export function ToolVisibilityCard({ className }: { className?: string }) {
  const [loaded, setLoaded] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [limit, setLimit] = React.useState("0");

  React.useEffect(() => {
    let alive = true;
    api
      .settings()
      .then((s: Settings) => {
        if (alive) setLimit(String(s.tool_visibility_limit ?? 0));
      })
      .catch(() => undefined)
      .finally(() => {
        if (alive) setLoaded(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  const save = () => {
    const n = Number(limit);
    if (!Number.isInteger(n) || n < 0 || n > 200) {
      toast.error("工具可见上限必须是 0–200 的整数（0=关闭，建议 20–40）");
      return;
    }
    setSaving(true);
    api
      .setSettings({ tool_visibility_limit: n })
      .then(() => toast.success(n === 0 ? "已关闭工具可见性限制（全量工具可见）" : `已启用工具可见上限：${n}`))
      .catch((e) => toast.error(`保存失败：${(e as Error).message}`))
      .finally(() => setSaving(false));
  };

  const disabled = !loaded || saving;
  const on = Number(limit) >= 20;
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <WrenchIcon className="size-4" />
          工具可见性
        </CardTitle>
        <CardDescription>
          限制每轮注入模型的工具 schema 数量。<b>0 = 关闭（默认，全量可见）</b>；设为 20–40
          时只保留常用工具，其余工具改为 deferred，模型通过 BM25 <code>search_tools</code> 按需发现。
          对之后启动的每一轮即时生效，无需重建 Agent。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-2">
        <Input
          type="number"
          min={0}
          max={200}
          className="w-32 font-mono text-sm"
          value={limit}
          disabled={disabled}
          onChange={(e) => setLimit(e.target.value)}
        />
        <Button onClick={save} disabled={disabled}>
          {saving ? <Loader2Icon className="size-4 animate-spin" /> : null}
          保存
        </Button>
        <span className="text-muted-foreground text-xs">
          {on ? `当前启用：最多 ${limit} 个工具 schema 常驻` : "当前关闭：全部工具 schema 常驻"}
        </span>
      </CardContent>
    </Card>
  );
}
