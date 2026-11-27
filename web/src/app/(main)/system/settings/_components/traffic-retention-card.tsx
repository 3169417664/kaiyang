"use client";

import * as React from "react";

import { AlertTriangleIcon, Loader2Icon, RefreshCwIcon, ShieldCheckIcon, TimerResetIcon } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { api } from "@/lib/api";
import type { TrafficRetentionResult, TrafficRetentionStatus } from "@/lib/types";

const MODE_LABEL: Record<string, string> = {
  off: "关闭（不抓流量）",
  evidence: "证据模式（短窗口 + 引用保护）",
  full: "全量模式（按保留天数）",
};

function fmt(n: number) {
  return n.toLocaleString("zh-CN");
}

function cutoffText(ts: number) {
  if (!ts) return "未开启";
  const d = new Date(ts * 1000);
  return Number.isNaN(d.getTime()) ? "—" : d.toLocaleString("zh-CN");
}

/** 流量留存设置卡：三段式 traffic_mode + 未引用流量 TTL + 证据窗口 + 预览/清理。 */
export function TrafficRetentionCard({ className }: { className?: string }) {
  const [status, setStatus] = React.useState<TrafficRetentionStatus | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState<"save" | "preview" | "run" | null>(null);
  const [result, setResult] = React.useState<TrafficRetentionResult | null>(null);
  const [confirmOpen, setConfirmOpen] = React.useState(false);

  const [mode, setMode] = React.useState("off");
  const [days, setDays] = React.useState("0");
  const [evidenceHours, setEvidenceHours] = React.useState("24");
  const [evidenceMaxGB, setEvidenceMaxGB] = React.useState("2");

  const applyStatus = React.useCallback((s: TrafficRetentionStatus) => {
    setStatus(s);
    setMode(s.mode === "evidence" || s.mode === "full" ? s.mode : "off");
    setDays(String(s.days ?? 0));
    setEvidenceHours(String(s.traffic_evidence_window_hours ?? 24));
    setEvidenceMaxGB(String(s.traffic_evidence_max_gb ?? 2));
  }, []);

  const load = React.useCallback(
    async (silent = false) => {
      if (!silent) setLoading(true);
      setLoadError(null);
      try {
        applyStatus(await api.trafficRetention());
      } catch (e) {
        const msg = e instanceof Error ? e.message : "读取流量留存状态失败";
        setLoadError(msg);
        if (!silent) toast.error(msg);
      } finally {
        if (!silent) setLoading(false);
      }
    },
    [applyStatus],
  );

  React.useEffect(() => {
    void load();
  }, [load]);

  // 保存并返回是否成功，供预览/执行先落库再计算。
  const save = React.useCallback(async (): Promise<boolean> => {
    const d = Number(days);
    if (!Number.isInteger(d) || d < 0 || d > 3650) {
      toast.error("保留天数必须是 0–3650 的整数（0=关闭）");
      return false;
    }
    const h = Number(evidenceHours);
    if (!Number.isInteger(h) || h < 0 || h > 8760) {
      toast.error("证据窗口必须是 0–8760 小时（0=关闭）");
      return false;
    }
    const g = Number(evidenceMaxGB);
    if (!Number.isInteger(g) || g < 0 || g > 4096) {
      toast.error("证据容量上限必须是 0–4096 GB（0=关闭）");
      return false;
    }
    setBusy("save");
    try {
      await api.setSettings({
        traffic_mode: mode as "off" | "evidence" | "full",
        traffic_retention_days: d,
        traffic_evidence_window_hours: h,
        traffic_evidence_max_gb: g,
      });
      await load(true);
      toast.success("已保存流量留存策略");
      return true;
    } catch (e) {
      toast.error(`保存失败：${(e as Error).message}`);
      return false;
    } finally {
      setBusy(null);
    }
  }, [days, evidenceHours, evidenceMaxGB, mode, load]);

  const preview = async () => {
    if (!(await save())) return;
    setBusy("preview");
    try {
      const r = await api.trafficRetentionRun(true);
      setResult(r);
      toast.success(`预览：预计清理 ${fmt(r.deletable ?? 0)} 条`);
    } catch (e) {
      toast.error(`预览失败：${(e as Error).message}`);
    } finally {
      setBusy(null);
    }
  };

  const execute = async () => {
    setConfirmOpen(false);
    setBusy("run");
    try {
      const r = await api.trafficRetentionRun(false);
      setResult(r);
      toast.success(`已清理 ${fmt(r.deleted ?? 0)} 条未引用流量`);
      await load(true);
    } catch (e) {
      toast.error(`清理失败：${(e as Error).message}`);
    } finally {
      setBusy(null);
    }
  };

  const enabled = status?.enabled ?? false;
  const deletable = result?.deletable ?? status?.deletable ?? 0;
  const protectedCount = result?.protected ?? status?.protected ?? 0;
  const disabled = loading || busy !== null;

  return (
    <>
      <Card className={className}>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2 text-base">
            <TimerResetIcon className="size-4" />
            流量留存
            <Badge variant={enabled ? "secondary" : "outline"}>{enabled ? "已开启" : "默认关闭"}</Badge>
            {status?.mode ? (
              <span className="font-normal text-muted-foreground text-xs">
                {MODE_LABEL[status.mode] ?? status.mode}
              </span>
            ) : null}
          </CardTitle>
          <CardDescription>
            三段式策略：<b>关闭</b>不抓流量；<b>证据模式</b>只长期保留被漏洞证据 / 回归 spec / 探索图引用的流量，
            其余按短窗口清理；<b>全量模式</b>按保留天数保存。清理只删除超期且未被引用的流量，受保护流量永不删除。
            默认关闭。
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {loadError && !status ? (
            <div className="flex flex-wrap items-center gap-2 text-destructive text-sm">
              <AlertTriangleIcon className="size-4" />
              读取留存状态失败：{loadError}
              <Button variant="outline" size="sm" onClick={() => void load()}>
                重试
              </Button>
            </div>
          ) : null}

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="traffic-mode">留存模式</Label>
              <Select value={mode} onValueChange={setMode} disabled={disabled}>
                <SelectTrigger id="traffic-mode" className="w-full min-w-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(MODE_LABEL) as string[]).map((m) => (
                    <SelectItem key={m} value={m}>
                      {MODE_LABEL[m]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="traffic-retention-days">全量模式保留天数（0=关闭）</Label>
              <Input
                id="traffic-retention-days"
                type="number"
                min={0}
                max={3650}
                className="font-mono text-sm"
                value={days}
                disabled={disabled || mode !== "full"}
                onChange={(e) => setDays(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="traffic-evidence-hours">证据窗口（小时，0=关闭）</Label>
              <Input
                id="traffic-evidence-hours"
                type="number"
                min={0}
                max={8760}
                className="font-mono text-sm"
                value={evidenceHours}
                disabled={disabled || mode !== "evidence"}
                onChange={(e) => setEvidenceHours(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="traffic-evidence-gb">证据容量上限（GB，0=不限制）</Label>
              <Input
                id="traffic-evidence-gb"
                type="number"
                min={0}
                max={4096}
                className="font-mono text-sm"
                value={evidenceMaxGB}
                disabled={disabled || mode !== "evidence"}
                onChange={(e) => setEvidenceMaxGB(e.target.value)}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {[
              ["总流量", status?.total ?? 0],
              ["候选", status?.candidates ?? 0],
              ["受保护", protectedCount],
              ["可清理", deletable],
            ].map(([label, value]) => (
              <div key={String(label)} className="rounded-md border p-2.5">
                <div className="text-muted-foreground text-[11px]">{label}</div>
                <div className="mt-0.5 font-semibold tabular-nums">{fmt(Number(value))}</div>
              </div>
            ))}
          </div>

          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-muted-foreground text-xs">
            <span>截止时间：{cutoffText(result?.cutoff || status?.cutoff || 0)}</span>
            {status?.evidence_pinned != null ? <span>证据引用 {fmt(status.evidence_pinned)} 条</span> : null}
            {result?.dry_run ? <Badge variant="outline">dry-run 预览</Badge> : null}
            {result?.deleted != null ? <span>本次删除 {fmt(result.deleted)} 条</span> : null}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={() => void save()} disabled={disabled}>
              {busy === "save" ? <Loader2Icon className="size-4 animate-spin" /> : null}
              保存策略
            </Button>
            <Button variant="outline" onClick={() => void preview()} disabled={disabled}>
              {busy === "preview" ? (
                <Loader2Icon className="size-4 animate-spin" />
              ) : (
                <RefreshCwIcon className="size-4" />
              )}
              预览清理
            </Button>
            <Button
              variant="destructive"
              onClick={() => setConfirmOpen(true)}
              disabled={disabled || !enabled || deletable <= 0}
            >
              {busy === "run" ? <Loader2Icon className="size-4 animate-spin" /> : null}
              执行清理
            </Button>
          </div>

          <p className="flex items-start gap-1.5 text-muted-foreground text-xs">
            <ShieldCheckIcon className="mt-0.5 size-3.5 shrink-0 text-emerald-500" />
            清理前从 PostgreSQL 读取证据快照、回归 spec 与探索图引用；读取失败会中止本轮，保护集为空时不会误删。
            执行清理不可撤销，建议先预览核对候选数与保护数。
          </p>
        </CardContent>
      </Card>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>执行不可逆的流量清理？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除 {fmt(deletable)} 条超期且未被引用的流量；受保护的 {fmt(protectedCount)} 条不会删除。
              删除后无法恢复，但已绑定到漏洞的证据快照会保留。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => void execute()}>确认清理</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
