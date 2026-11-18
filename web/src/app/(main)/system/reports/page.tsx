"use client";

import * as React from "react";

import { DownloadIcon, RefreshCwIcon, Trash2Icon, UploadIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";
import type { ReportRecord, ReportSettings, ReportTemplate } from "@/lib/types";

type TemplateForm = {
  id: number; // 0 = 新建
  name: string;
  company_name: string;
  company_name_en: string;
  logo_path: string;
  personnel_text: string;
  cvss_version: string;
  locale: string;
};

const emptyTemplate: TemplateForm = {
  id: 0,
  name: "",
  company_name: "",
  company_name_en: "",
  logo_path: "",
  personnel_text: "[]",
  cvss_version: "3.1",
  locale: "zh-CN",
};

function templateToForm(t: ReportTemplate): TemplateForm {
  return {
    id: t.id,
    name: t.name,
    company_name: t.company_name,
    company_name_en: t.company_name_en,
    logo_path: t.logo_path,
    personnel_text: JSON.stringify(t.personnel_json ?? [], null, 2),
    cvss_version: t.cvss_version || "3.1",
    locale: t.locale || "zh-CN",
  };
}

export default function ReportsPage() {
  const [settings, setSettings] = React.useState<ReportSettings>({
    company_name: "", company_name_en: "", logo_path: "", personnel: [], cvss_version: "3.1",
  });
  const [personnelText, setPersonnelText] = React.useState("[]");
  const [reports, setReports] = React.useState<ReportRecord[]>([]);
  const [templates, setTemplates] = React.useState<ReportTemplate[]>([]);
  const [form, setForm] = React.useState<TemplateForm>(emptyTemplate);
  const [loading, setLoading] = React.useState(true);
  const [uploading, setUploading] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [st, rs, tpls] = await Promise.all([api.reportSettings(), api.reports(), api.reportTemplates()]);
      setSettings(st);
      setPersonnelText(JSON.stringify(st.personnel ?? [], null, 2));
      setReports(rs);
      setTemplates(tpls);
    } catch (e) {
      toast.error((e as Error).message || "加载报告失败");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { void load(); }, [load]);

  async function saveSettings() {
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

  async function uploadLogo(target: "settings" | "template", file: File) {
    setUploading(true);
    try {
      const asset = await api.uploadReportLogo(file);
      if (target === "settings") {
        setSettings((prev) => ({ ...prev, logo_path: asset.path }));
      } else {
        setForm((prev) => ({ ...prev, logo_path: asset.path }));
      }
      toast.success("Logo 已上传");
    } catch (e) {
      toast.error((e as Error).message || "上传失败");
    } finally {
      setUploading(false);
    }
  }

  async function saveTemplate() {
    let personnel: unknown;
    try {
      personnel = JSON.parse(form.personnel_text || "[]");
    } catch {
      toast.error("人员必须是合法 JSON 数组");
      return;
    }
    const input = {
      name: form.name,
      company_name: form.company_name,
      company_name_en: form.company_name_en,
      logo_path: form.logo_path,
      personnel_json: personnel,
      cvss_version: form.cvss_version,
      locale: form.locale,
    };
    try {
      if (form.id > 0) {
        await api.updateReportTemplate(form.id, input);
        toast.success("模板已更新");
      } else {
        await api.createReportTemplate(input);
        toast.success("模板已创建");
      }
      setForm(emptyTemplate);
      await load();
    } catch (e) { toast.error((e as Error).message || "保存模板失败"); }
  }

  async function removeTemplate(t: ReportTemplate) {
    if (!window.confirm(`删除模板「${t.name}」？`)) return;
    try {
      await api.deleteReportTemplate(t.id);
      if (form.id === t.id) setForm(emptyTemplate);
      toast.success("模板已删除");
      await load();
    } catch (e) { toast.error((e as Error).message || "删除失败"); }
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
            <div className="flex gap-2">
              <Input value={settings.logo_path} onChange={(e) => setSettings({ ...settings, logo_path: e.target.value })} placeholder="assets/<sha>.png" />
              <label className="inline-flex cursor-pointer items-center gap-1 rounded-md border px-2 text-sm">
                <UploadIcon className="size-4" />
                <span>上传</span>
                <input type="file" className="hidden" accept="image/png,image/jpeg,image/gif"
                  disabled={uploading}
                  onChange={(e) => { const f = e.target.files?.[0]; if (f) void uploadLogo("settings", f); e.target.value = ""; }} />
              </label>
            </div>
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
            <Button onClick={saveSettings}>保存设置</Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>报告模板</CardTitle>
          <CardDescription>生成报告时可选模板，带出公司名、Logo、人员、CVSS 版本与语言。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="border-b text-muted-foreground">
                <tr className="text-left">
                  <th className="py-2 pr-4 font-medium">ID</th>
                  <th className="py-2 pr-4 font-medium">名称</th>
                  <th className="py-2 pr-4 font-medium">公司</th>
                  <th className="py-2 pr-4 font-medium">CVSS</th>
                  <th className="py-2 pr-4 font-medium">语言</th>
                  <th className="py-2 pr-4 font-medium">Logo</th>
                  <th className="py-2 pr-4 font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {templates.length === 0 ? (
                  <tr><td colSpan={7} className="py-6 text-center text-muted-foreground">暂无模板</td></tr>
                ) : templates.map((t) => (
                  <tr key={t.id} className="border-b last:border-0">
                    <td className="py-2 pr-4 font-mono">{t.id}</td>
                    <td className="py-2 pr-4">{t.name}</td>
                    <td className="py-2 pr-4">{t.company_name || "—"}</td>
                    <td className="py-2 pr-4">{t.cvss_version || "3.1"}</td>
                    <td className="py-2 pr-4">{t.locale || "zh-CN"}</td>
                    <td className="max-w-[160px] truncate py-2 pr-4 font-mono text-xs" title={t.logo_path}>{t.logo_path || "—"}</td>
                    <td className="py-2 pr-4">
                      <div className="flex gap-2">
                        <Button size="sm" variant="outline" onClick={() => setForm(templateToForm(t))}>编辑</Button>
                        <Button size="sm" variant="ghost" className="text-destructive" onClick={() => void removeTemplate(t)}>
                          <Trash2Icon className="size-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="rounded-md border p-4">
            <div className="mb-3 flex items-center justify-between">
              <span className="font-medium text-sm">{form.id > 0 ? `编辑模板 #${form.id}` : "新建模板"}</span>
              {form.id > 0 && (
                <Button size="sm" variant="ghost" onClick={() => setForm(emptyTemplate)}>取消编辑</Button>
              )}
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label>模板名称</Label>
                <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="客户 A · 渗透测试" />
              </div>
              <div className="space-y-1.5">
                <Label>公司名（中文）</Label>
                <Input value={form.company_name} onChange={(e) => setForm({ ...form, company_name: e.target.value })} />
              </div>
              <div className="space-y-1.5">
                <Label>公司名（英文）</Label>
                <Input value={form.company_name_en} onChange={(e) => setForm({ ...form, company_name_en: e.target.value })} />
              </div>
              <div className="space-y-1.5">
                <Label>Logo 路径</Label>
                <div className="flex gap-2">
                  <Input value={form.logo_path} onChange={(e) => setForm({ ...form, logo_path: e.target.value })} placeholder="assets/<sha>.png" />
                  <label className="inline-flex cursor-pointer items-center gap-1 rounded-md border px-2 text-sm">
                    <UploadIcon className="size-4" />
                    <span>上传</span>
                    <input type="file" className="hidden" accept="image/png,image/jpeg,image/gif"
                      disabled={uploading}
                      onChange={(e) => { const f = e.target.files?.[0]; if (f) void uploadLogo("template", f); e.target.value = ""; }} />
                  </label>
                </div>
              </div>
              <div className="space-y-1.5">
                <Label>CVSS 版本</Label>
                <Input value={form.cvss_version} onChange={(e) => setForm({ ...form, cvss_version: e.target.value })} />
              </div>
              <div className="space-y-1.5">
                <Label>语言（locale）</Label>
                <Input value={form.locale} onChange={(e) => setForm({ ...form, locale: e.target.value })} placeholder="zh-CN" />
              </div>
              <div className="space-y-1.5 md:col-span-2">
                <Label>测试人员（JSON 数组）</Label>
                <textarea
                  className="min-h-[90px] w-full rounded-md border bg-background p-3 font-mono text-xs"
                  value={form.personnel_text}
                  onChange={(e) => setForm({ ...form, personnel_text: e.target.value })}
                  placeholder='[{"name":"张三","role":"渗透测试工程师","contact":"..."}]'
                />
              </div>
              <div className="md:col-span-2">
                <Button onClick={saveTemplate}>{form.id > 0 ? "保存模板" : "创建模板"}</Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>已生成报告</CardTitle>
          <CardDescription>共 {reports.length} 份。生成入口在任务详情「报告」Tab（可选择报告模板）。</CardDescription>
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
