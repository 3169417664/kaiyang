"use client";

import * as React from "react";

import { Download, FileArchive } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";

export default function DiagnosticsPage() {
  const [detail, setDetail] = React.useState(false);
  const [exporting, setExporting] = React.useState(false);

  async function exportZip() {
    setExporting(true);
    try {
      await api.exportDiagnostics({ detail });
      toast.success("诊断包已开始下载");
    } catch (e) {
      toast.error((e as Error).message || "导出失败");
    } finally {
      setExporting(false);
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 p-4 md:p-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FileArchive className="size-5" />
            诊断包
          </CardTitle>
          <CardDescription>
            导出系统配置、运行指标、任务/意图/轨迹摘要、日志聚合、流量与代理状态。代理 URL 中的
            账号密码会被掩码；不包含流量正文和漏洞证据正文。
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
            <div className="space-y-1">
              <Label htmlFor="diag-detail">详细模式</Label>
              <p className="text-muted-foreground text-xs">
                放大轨迹/日志/事实窗口，包体更大，适合深度排障。
              </p>
            </div>
            <Switch id="diag-detail" checked={detail} onCheckedChange={setDetail} />
          </div>
          <Button onClick={exportZip} disabled={exporting} className="w-fit">
            <Download />
            {exporting ? "正在导出…" : "导出诊断包"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
