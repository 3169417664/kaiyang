"use client";

import * as React from "react";

import { RefreshCwIcon, SearchIcon, Trash2Icon, UploadIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { KnowledgeConfig, KnowledgeDoc, KnowledgeHit } from "@/lib/types";

export default function KnowledgePage() {
  const [docs, setDocs] = React.useState<KnowledgeDoc[]>([]);
  const [scope, setScope] = React.useState<"global" | "task">("global");
  const [taskId, setTaskId] = React.useState("");
  const [category, setCategory] = React.useState("");
  const [title, setTitle] = React.useState("");
  const [content, setContent] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [hits, setHits] = React.useState<KnowledgeHit[]>([]);
  const [mode, setMode] = React.useState("");
  const [cfg, setCfg] = React.useState<KnowledgeConfig>({ enabled: false, base_url: "", model: "", dim: 1024 });
  const [busy, setBusy] = React.useState(false);
  const fileRef = React.useRef<HTMLInputElement>(null);

  const load = React.useCallback(async () => {
    try {
      const [ds, c] = await Promise.all([api.knowledgeDocs(scope, taskId.trim() || undefined), api.knowledgeConfig()]);
      setDocs(ds); setCfg(c);
    } catch (e) { toast.error((e as Error).message || "加载知识库失败"); }
  }, [scope, taskId]);
  React.useEffect(() => { void load(); }, [load]);

  async function addText() {
    if (!title.trim() || !content.trim()) { toast.error("标题和正文必填"); return; }
    setBusy(true);
    try {
      await api.createKnowledgeDoc({ scope, task_id: scope === "task" ? Number(taskId) : undefined, category: category.trim(), title: title.trim(), content });
      toast.success("已添加"); setTitle(""); setContent(""); await load();
    } catch (e) { toast.error((e as Error).message || "添加失败"); } finally { setBusy(false); }
  }

  async function upload() {
    const f = fileRef.current?.files?.[0];
    if (!f) { toast.error("请选择文件"); return; }
    setBusy(true);
    try {
      await api.uploadKnowledgeDoc(f, scope, scope === "task" ? Number(taskId) : 0, category.trim());
      toast.success("已上传并索引"); if (fileRef.current) fileRef.current.value = ""; await load();
    } catch (e) { toast.error((e as Error).message || "上传失败"); } finally { setBusy(false); }
  }

  async function search() {
    if (!query.trim()) return;
    setBusy(true);
    try {
      const r = await api.searchKnowledge(query, 0, 8);
      setHits(r.hits ?? []); setMode(r.mode || "");
    } catch (e) { toast.error((e as Error).message || "检索失败"); } finally { setBusy(false); }
  }

  async function saveCfg() {
    try { await api.saveKnowledgeConfig(cfg); toast.success("embedder 配置已保存"); }
    catch (e) { toast.error((e as Error).message || "保存失败"); }
  }

  return (
    <div className="space-y-6 p-4 lg:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">知识库</h1>
          <p className="mt-1 text-muted-foreground text-sm">全局 + 任务双层 RAG；pgvector/embedder 缺失时自动降级关键词检索。</p>
        </div>
        <div className="flex items-end gap-2">
          <select className="h-9 rounded-md border bg-background px-3 text-sm" value={scope} onChange={(e) => setScope(e.target.value as "global" | "task")}>
            <option value="global">全局知识库</option>
            <option value="task">任务知识库</option>
          </select>
          {scope === "task" && <Input className="w-36" value={taskId} onChange={(e) => setTaskId(e.target.value.replace(/[^0-9]/g, ""))} placeholder="任务 ID" />}
          <Button variant="outline" onClick={load}><RefreshCwIcon className="size-4" /> 刷新</Button>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{scope === "global" ? "新增全局知识" : "新增任务知识"}</CardTitle>
            <CardDescription>漏洞库、方法论、检查项、客户背景资料等。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid gap-2 sm:grid-cols-2">
              <div className="space-y-1.5"><Label>分类</Label><Input value={category} onChange={(e) => setCategory(e.target.value)} placeholder="web / ad / cloud" /></div>
              <div className="space-y-1.5"><Label>标题</Label><Input value={title} onChange={(e) => setTitle(e.target.value)} /></div>
            </div>
            <textarea className="min-h-[160px] w-full rounded-md border bg-background p-3 text-sm" value={content} onChange={(e) => setContent(e.target.value)} placeholder="知识正文…" />
            <div className="flex flex-wrap items-center gap-2">
              <Button onClick={addText} disabled={busy}>添加文本</Button>
              <input ref={fileRef} type="file" className="hidden" onChange={upload} />
              <Button variant="outline" onClick={() => fileRef.current?.click()} disabled={busy}><UploadIcon className="size-4" /> 上传文档</Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>检索预览</CardTitle>
            <CardDescription>{mode ? `当前模式：${mode}` : "输入问题测试知识命中。"}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex gap-2">
              <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="例如：SQL 注入绕过 WAF 的手法" />
              <Button onClick={search} disabled={busy}><SearchIcon className="size-4" /> 检索</Button>
            </div>
            <div className="max-h-[360px] space-y-2 overflow-auto">
              {hits.length === 0 ? <p className="py-6 text-center text-muted-foreground text-sm">暂无结果</p> : hits.map((h, i) => (
                <div key={i} className="rounded border p-2 text-sm">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">{h.scope || "global"}</Badge>
                    <span className="font-medium">{h.title || "未命名"}</span>
                    <span className="ml-auto text-muted-foreground text-xs">{(h.score * 100).toFixed(0)}%</span>
                  </div>
                  <p className="mt-1 whitespace-pre-wrap text-muted-foreground text-xs">{h.content.slice(0, 400)}</p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle>embedder 配置</CardTitle><CardDescription>OpenAI 兼容 /embeddings 端点；缺失时自动走关键词。</CardDescription></CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2">
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={cfg.enabled} onChange={(e) => setCfg({ ...cfg, enabled: e.target.checked })} /> 启用语义检索</label>
          <div className="space-y-1.5"><Label>Base URL</Label><Input value={cfg.base_url} onChange={(e) => setCfg({ ...cfg, base_url: e.target.value })} /></div>
          <div className="space-y-1.5"><Label>Model</Label><Input value={cfg.model} onChange={(e) => setCfg({ ...cfg, model: e.target.value })} /></div>
          <div className="space-y-1.5"><Label>API Key</Label><Input type="password" value={cfg.api_key ?? ""} onChange={(e) => setCfg({ ...cfg, api_key: e.target.value })} placeholder={cfg.has_key ? "已配置，留空不修改" : ""} /></div>
          <div className="space-y-1.5"><Label>维度</Label><Input value={cfg.dim} onChange={(e) => setCfg({ ...cfg, dim: Number(e.target.value) || 0 })} /></div>
          <div><Button onClick={saveCfg}>保存配置</Button></div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{scope === "global" ? "全局文档" : `任务 #${taskId || "?"} 文档`}</CardTitle><CardDescription>共 {docs.length} 篇。</CardDescription></CardHeader>
        <CardContent className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="border-b text-muted-foreground"><tr className="text-left">
              <th className="py-2 pr-4 font-medium">标题</th><th className="py-2 pr-4 font-medium">分类</th><th className="py-2 pr-4 font-medium">来源</th><th className="py-2 pr-4 font-medium">索引</th><th className="py-2 pr-4 font-medium">操作</th>
            </tr></thead>
            <tbody>
              {docs.length === 0 ? <tr><td colSpan={5} className="py-8 text-center text-muted-foreground">暂无文档</td></tr> : docs.map((d) => (
                <tr key={d.id} className="border-b last:border-0">
                  <td className="py-2 pr-4">{d.title}</td>
                  <td className="py-2 pr-4">{d.category || "—"}</td>
                  <td className="py-2 pr-4">{d.source}{d.file_name ? ` · ${d.file_name}` : ""}</td>
                  <td className="py-2 pr-4"><Badge variant={d.indexed ? "secondary" : "outline"}>{d.indexed ? "已索引" : "未索引"}</Badge></td>
                  <td className="py-2 pr-4">
                    <Button size="sm" variant="ghost" className="text-destructive" onClick={async () => {
                      if (!window.confirm(`删除文档「${d.title}」？`)) return;
                      try { await api.deleteKnowledgeDoc(d.id); toast.success("已删除"); await load(); }
                      catch (e) { toast.error((e as Error).message || "删除失败"); }
                    }}><Trash2Icon className="size-4" /></Button>
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
