"use client";

import * as React from "react";

import { Loader2Icon, WalletIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import type { Settings } from "@/lib/types";

/** 任务级预算设置卡：token / USD / 墙钟，三项 0=不限。 */
export function BudgetCard({ className }: { className?: string }) {
  const [loaded, setLoaded] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [tokens, setTokens] = React.useState("0");
  const [usd, setUsd] = React.useState("0");
  const [minutes, setMinutes] = React.useState("0");
  const [warnPct, setWarnPct] = React.useState("80");

  const apply = React.useCallback((s: Settings) => {
    setTokens(String(s.task_budget_tokens ?? 0));
    setUsd(String(s.task_budget_usd ?? 0));
    setMinutes(String(s.task_budget_minutes ?? 0));
    setWarnPct(String(s.task_budget_warn_pct ?? 80));
  }, []);

  React.useEffect(() => {
    let alive = true;
    api
      .settings()
      .then((s) => {
        if (alive) apply(s);
      })
      .catch(() => undefined)
      .finally(() => {
        if (alive) setLoaded(true);
      });
    return () => {
      alive = false;
    };
  }, [apply]);

  const save = () => {
    const t = Number(tokens);
    if (!Number.isInteger(t) || (t !== 0 && (t < 100000 || t > 2000000000))) {
      toast.error("任务 token 预算必须为 0（不限）或 100000–2000000000");
      return;
    }
    const u = Number(usd);
    if (!Number.isFinite(u) || u < 0 || u > 1000000) {
      toast.error("任务 USD 预算必须在 0–1000000（0=不限）");
      return;
    }
    const m = Number(minutes);
    if (!Number.isInteger(m) || m < 0 || m > 10080) {
      toast.error("任务墙钟预算必须在 0–10080 分钟（0=不限）");
      return;
    }
    const w = Number(warnPct);
    if (!Number.isInteger(w) || w < 50 || w > 95) {
      toast.error("预警阈值必须在 50–95（%）");
      return;
    }
    setSaving(true);
    api
      .setSettings({
        task_budget_tokens: t,
        task_budget_usd: u,
        task_budget_minutes: m,
        task_budget_warn_pct: w,
      })
      .then((s) => {
        apply(s);
        toast.success("已保存任务级预算（每一轮规划前检查）");
      })
      .catch((e) => toast.error(`保存失败：${(e as Error).message}`))
      .finally(() => setSaving(false));
  };

  const disabled = !loaded || saving;
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <WalletIcon className="size-4" />
          任务级预算（token / USD / 墙钟）
        </CardTitle>
        <CardDescription>
          覆盖整个任务（planner 轮、压缩、报告等所有模型调用）。达到预警阈值时注入一次收敛提醒； 达到 100%
          直接收尾（不再跑终局规划轮），终态记「未完成」而非 done。 三项均为 <b>0 = 不限</b>
          ；修改后每一轮规划前检查。USD 按内置参考价估算，部分模型无定价时按 0 计（仅作为下限）。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-3">
          <label htmlFor="task-budget-tokens" className="flex flex-col gap-1 text-sm">
            <span>任务 token 上限（0=不限）</span>
            <Input
              id="task-budget-tokens"
              type="number"
              min={0}
              step={100000}
              className="w-full font-mono text-sm sm:w-52"
              value={tokens}
              disabled={disabled}
              onChange={(e) => setTokens(e.target.value)}
            />
          </label>
          <label htmlFor="task-budget-usd" className="flex flex-col gap-1 text-sm">
            <span>任务 USD 上限（0=不限）</span>
            <Input
              id="task-budget-usd"
              type="number"
              min={0}
              step={0.01}
              className="w-full font-mono text-sm sm:w-40"
              value={usd}
              disabled={disabled}
              onChange={(e) => setUsd(e.target.value)}
            />
          </label>
          <label htmlFor="task-budget-minutes" className="flex flex-col gap-1 text-sm">
            <span>任务墙钟上限（分钟，0=不限）</span>
            <Input
              id="task-budget-minutes"
              type="number"
              min={0}
              max={10080}
              className="w-full font-mono text-sm sm:w-48"
              value={minutes}
              disabled={disabled}
              onChange={(e) => setMinutes(e.target.value)}
            />
          </label>
          <label htmlFor="task-budget-warn" className="flex flex-col gap-1 text-sm">
            <span>预警阈值（%，50–95）</span>
            <Input
              id="task-budget-warn"
              type="number"
              min={50}
              max={95}
              className="w-full font-mono text-sm sm:w-32"
              value={warnPct}
              disabled={disabled}
              onChange={(e) => setWarnPct(e.target.value)}
            />
          </label>
        </div>
        <div>
          <Button onClick={save} disabled={disabled}>
            {saving ? <Loader2Icon className="size-4 animate-spin" /> : null}
            保存
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
