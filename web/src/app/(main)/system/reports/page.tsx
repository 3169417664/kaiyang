"use client";

import * as React from "react";

import { DownloadIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";
import type { ReportRecord, ReportSettings } from "@/lib/types";

export default function ReportsPage() {
  const [settings, setSettings] = React.useState<ReportSettings>({
    company_name: "", company_name_en: "", logo_path: "", personnel: [], cvss_version: "3.1",
  });
  const [personnelText, setPersonnelText] = React.useState("[]");
  const [reports, setReports] = React.useState<ReportRecord[]>([]);
  const [loading, setLoading] = React.useState(true);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [st, rs] = await Promise.all([api.reportSettings(), api.reports()]);
      setSettings(st);
      setPersonnelText(JSON.stringify(st.personnel ?? [], null, 2));
      setReports(rs);
    } catch (e) {
      toast.error((e as Error).message || "加载报告失败");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { void load(); }, [load]);

  async function save() {
    let personnel: ReportSettings["personnel"] = [];
    try {
      personnel = JSON.parse(personnelText || "[]");
    } catch {
      toast.error("人员必须是合法 JSON 数组");
      return;
    }
    try {
      await api.saveReportSettings({ ...settings, personnel });
      toast.success("报告设置已保存");
    } catch (e) { toast.error((e as Error).message || "保存失败"); }
  }

  async function download(r: ReportRecord) {
    const token = auth.getToken();
    const res = await fetch(`/api/reports/${r.id}/download`, { headers: token ? { Authorization: `Bearer ${token}` } : {} });
    if (!res.ok) { toast.error(`下载失败: ${res.status}`); return; }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url; a.download = r.filename || `report-${r.id}.docx`; a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-6 p-4 lg:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">报告管理</h1>
          <p className="mt-1 text-muted-foreground text-sm">中文 docx 交付件；报告文件存放在 keyDir/report_assets，不在工作区。</p>
        </div>
        <Button variant="outline" onClick={load} disabled={loading}>
          <RefreshCwIcon className="size-4" /> 刷新
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>组织级报告设置</CardTitle>
          <CardDescription>出具方、Logo、人员与 CVSS 版本会写入每份报告封面/属性表。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label>出具方（中文）</Label>
            <Input value={settings.company_name} onChange={(e) => setSettings({ ...settings, company_name: e.target.value })} />
          </div>
          <div className="space-y-1.5">
            <Label>出具方（英文）</Label>
            <Input value={settings.company_name_en} onChange={(e) => setSettings({ ...settings, company_name_en: e.target.value })} />
          </div>
          <div className="space-y-1.5">
            <Label>Logo 路径（可选）</Label>
            <Input value={settings.logo_path} onChange={(e) => setSettings({ ...settings, logo_path: e.target.value })} placeholder="report_assets/logo.png" />
          </div>
          <div className="space-y-1.5">
            <Label>CVSS 版本</Label>
            <Input value={settings.cvss_version} onChange={(e) => setSettings({ ...settings, cvss_version: e.target.value })} />
          </div>
          <div className="space-y-1.5 md:col-span-2">
            <Label>测试人员（JSON 数组）</Label>
            <textarea
              className="min-h-[100px] w-full rounded-md border bg-background p-3 font-mono text-xs"
              value={personnelText}
              onChange={(e) => setPersonnelText(e.target.value)}
              placeholder='[{"name":"张三","role":"渗透测试工程师","contact":"..."}]'
            />
          </div>
          <div className="md:col-span-2">
            <Button onClick={save}>保存设置</Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>已生成报告</CardTitle>
          <CardDescription>共 {reports.length} 份。生成入口在任务详情「报告」Tab。</CardDescription>
        </CardHeader>
        <CardContent className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="border-b text-muted-foreground">
              <tr className="text-left">
                <th className="py-2 pr-4 font-medium">ID</th>
                <th className="py-2 pr-4 font-medium">任务</th>
                <th className="py-2 pr-4 font-medium">文件</th>
                <th className="py-2 pr-4 font-medium">状态</th>
                <th className="py-2 pr-4 font-medium">大小</th>
                <th className="py-2 pr-4 font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {reports.length === 0 ? (
                <tr><td colSpan={6} className="py-8 text-center text-muted-foreground">暂无报告</td></tr>
              ) : reports.map((r) => (
                <tr key={r.id} className="border-b last:border-0">
                  <td className="py-2 pr-4 font-mono">{r.id}</td>
                  <td className="max-w-xs truncate py-2 pr-4" title={r.task_desc}>{r.task_desc || (r.task_id ? `#${r.task_id}` : "—")}</td>
                  <td className="max-w-xs truncate py-2 pr-4 font-mono" title={r.filename}>{r.filename || "—"}</td>
                  <td className="py-2 pr-4">
                    <Badge variant={r.status === "done" ? "secondary" : r.status === "failed" ? "destructive" : "outline"}>
                      {r.status}{r.error ? ` · ${r.error}` : ""}
                    </Badge>
                  </td>
                  <td className="py-2 pr-4">{r.size ? `${Math.round(r.size / 1024)} KB` : "—"}</td>
                  <td className="py-2 pr-4">
                    <div className="flex gap-2">
                      <Button size="sm" variant="outline" disabled={r.status !== "done"} onClick={() => void download(r)}>
                        <DownloadIcon className="size-4" /> 下载
                      </Button>
                      <Button size="sm" variant="ghost" className="text-destructive" onClick={async () => {
                        if (!window.confirm(`删除报告 #${r.id}？`)) return;
                        try { await api.deleteReport(r.id); toast.success("已删除"); await load(); }
                        catch (e) { toast.error((e as Error).message || "删除失败"); }
                      }}>
                        <Trash2Icon className="size-4" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>
    </div>
  );
}
