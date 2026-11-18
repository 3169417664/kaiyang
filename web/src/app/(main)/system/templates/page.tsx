"use client";

import * as React from "react";

import { PlusIcon, Trash2Icon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";

type Kind = "planner" | "worker";
type Tpl = { id: number; name: string; nkey: string; body: string; builtin: boolean; is_default: boolean };

export default function TemplatesPage() {
  return (
    <div className="space-y-6 p-4 lg:p-6">
      <div>
        <h1 className="font-semibold text-xl">提示词模板</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          planner / worker 的任务级模板库。四级回退：任务快照 &gt; 全局 Agent 覆盖 &gt; 本库默认 &gt; 内置默认。
          模板正文支持 <code>{"{{.Goal}}"}</code>、<code>{"{{.DataDir}}"}</code>、<code>{"{{.Now}}"}</code>。
        </p>
      </div>
      <Tabs defaultValue="planner">
        <TabsList>
          <TabsTrigger value="planner">Planner 模板</TabsTrigger>
          <TabsTrigger value="worker">Worker 模板</TabsTrigger>
        </TabsList>
        <TabsContent value="planner" className="mt-4">
          <TemplateList kind="planner" />
        </TabsContent>
        <TabsContent value="worker" className="mt-4">
          <TemplateList kind="worker" />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function TemplateList({ kind }: { kind: Kind }) {
  const [items, setItems] = React.useState<Tpl[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [editing, setEditing] = React.useState<Tpl | null>(null);
  const [creating, setCreating] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      setItems(await api.promptTemplates(kind));
    } catch (e) {
      toast.error((e as Error).message || "加载模板失败");
    } finally {
      setLoading(false);
    }
  }, [kind]);

  React.useEffect(() => { void load(); }, [load]);

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>{kind === "planner" ? "Planner" : "Worker"} 模板</CardTitle>
          <CardDescription>内置模板可编辑正文但不可删除；自建模板可删除。</CardDescription>
        </div>
        <Button onClick={() => setCreating(true)}>
          <PlusIcon className="size-4" /> 新建模板
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <p className="py-8 text-center text-muted-foreground text-sm">加载中…</p>
        ) : items.length === 0 ? (
          <p className="py-8 text-center text-muted-foreground text-sm">暂无模板</p>
        ) : (
          items.map((t) => (
            <div key={t.id} className="rounded-lg border p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{t.name}</span>
                  <Badge variant="outline">{t.nkey}</Badge>
                  {t.builtin && <Badge variant="secondary">内置</Badge>}
                  {t.is_default && <Badge>默认</Badge>}
                </div>
                <div className="flex gap-2">
                  <Button size="sm" variant="outline" onClick={() => setEditing(t)}>编辑</Button>
                  {!t.is_default && (
                    <Button size="sm" variant="outline" onClick={async () => {
                      try {
                        await api.setDefaultPromptTemplate(kind, t.id);
                        toast.success("已设为默认");
                        await load();
                      } catch (e) { toast.error((e as Error).message || "设置失败"); }
                    }}>设为默认</Button>
                  )}
                  {!t.builtin && (
                    <Button size="sm" variant="ghost" className="text-destructive" onClick={async () => {
                      if (!window.confirm(`删除模板 ${t.name}？`)) return;
                      try {
                        await api.deletePromptTemplate(kind, t.id);
                        toast.success("已删除");
                        await load();
                      } catch (e) { toast.error((e as Error).message || "删除失败"); }
                    }}>
                      <Trash2Icon className="size-4" />
                    </Button>
                  )}
                </div>
              </div>
              <pre className="mt-2 line-clamp-3 whitespace-pre-wrap text-muted-foreground text-xs">{t.body.slice(0, 300)}</pre>
            </div>
          ))
        )}
      </CardContent>
      {editing && (
        <EditDialog kind={kind} tpl={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await load(); }} />
      )}
      {creating && (
        <CreateDialog kind={kind} onClose={() => setCreating(false)} onCreated={async () => { setCreating(false); await load(); }} />
      )}
    </Card>
  );
}

function EditDialog({ kind, tpl, onClose, onSaved }: { kind: Kind; tpl: Tpl; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = React.useState(tpl.name);
  const [body, setBody] = React.useState(tpl.body);
  const [busy, setBusy] = React.useState(false);
  async function submit() {
    setBusy(true);
    try {
      await api.updatePromptTemplate(kind, tpl.id, { name, body });
      toast.success("已保存");
      onSaved();
    } catch (e) { toast.error((e as Error).message || "保存失败"); } finally { setBusy(false); }
  }
  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader><DialogTitle>编辑模板：{tpl.name}</DialogTitle></DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>名称</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} disabled={tpl.builtin} />
          </div>
          <div className="space-y-1.5">
            <Label>正文</Label>
            <textarea className="min-h-[360px] w-full rounded-md border bg-background p-3 font-mono text-xs" value={body} onChange={(e) => setBody(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? "保存中…" : "保存"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CreateDialog({ kind, onClose, onCreated }: { kind: Kind; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = React.useState("");
  const [nkey, setNKey] = React.useState("");
  const [body, setBody] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  async function submit() {
    if (!name.trim() || !nkey.trim() || !body.trim()) { toast.error("名称、nkey、正文必填"); return; }
    setBusy(true);
    try {
      await api.createPromptTemplate(kind, { name: name.trim(), nkey: nkey.trim(), body });
      toast.success("已创建");
      onCreated();
    } catch (e) { toast.error((e as Error).message || "创建失败"); } finally { setBusy(false); }
  }
  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader><DialogTitle>新建 {kind === "planner" ? "Planner" : "Worker"} 模板</DialogTitle></DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5"><Label>名称</Label><Input value={name} onChange={(e) => setName(e.target.value)} /></div>
          <div className="space-y-1.5"><Label>nkey（唯一）</Label><Input value={nkey} onChange={(e) => setNKey(e.target.value)} placeholder="my-scene" /></div>
          <div className="space-y-1.5"><Label>正文</Label><textarea className="min-h-[300px] w-full rounded-md border bg-background p-3 font-mono text-xs" value={body} onChange={(e) => setBody(e.target.value)} /></div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? "创建中…" : "创建"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
