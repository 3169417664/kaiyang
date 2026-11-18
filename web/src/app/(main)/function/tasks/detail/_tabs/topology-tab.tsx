"use client";
"use no memo";

import * as React from "react";

import {
  Background,
  BackgroundVariant,
  type Connection,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  type NodeProps,
  Panel,
  Position,
  ReactFlow,
  ReactFlowProvider,
  type Edge as RFEdge,
  type Node as RFNode,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import dagre from "@dagrejs/dagre";
import {
  DownloadIcon,
  LinkIcon,
  PlusIcon,
  RefreshCwIcon,
  SaveIcon,
  StickyNoteIcon,
  Trash2Icon,
  UploadIcon,
  UsersIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { TopologyData, TopologyEdge, TopologyGroup, TopologyNode, TopologyNote } from "@/lib/types";
import { cn } from "@/lib/utils";

type DeviceNodeData = {
  ip: string;
  hostname: string;
  label: string;
  notes: string;
  groupName?: string;
  groupColor?: string;
};
type NoteNodeData = { noteId: number; content: string };
type DeviceRFNode = RFNode<DeviceNodeData, "topo">;
type NoteRFNode = RFNode<NoteNodeData, "note">;
type TopoRFNode = DeviceRFNode | NoteRFNode;

const DEFAULT_GROUP_COLOR = "#3b82f6";

// dagreAutoLayout 为「所有节点坐标都为 0」的新拓扑计算分层坐标。
// 已有坐标（用户拖拽过/手工布局）一律以数据库为准。
function dagreAutoLayout(nodes: TopologyNode[], edges: TopologyEdge[]): Map<string, { x: number; y: number }> {
	const g = new dagre.graphlib.Graph();
	g.setGraph({ rankdir: "LR", nodesep: 48, ranksep: 96, marginx: 32, marginy: 32 });
	g.setDefaultEdgeLabel(() => ({}));
	for (const n of nodes) g.setNode(n.ip, { width: 210, height: 76 });
	for (const e of edges) {
		if (g.hasNode(e.src_ip) && g.hasNode(e.dst_ip)) g.setEdge(e.src_ip, e.dst_ip);
	}
	dagre.layout(g);
	const out = new Map<string, { x: number; y: number }>();
	for (const n of nodes) {
		const p = g.node(n.ip);
		if (p) out.set(n.ip, { x: Math.round(p.x - 105), y: Math.round(p.y - 38) });
	}
	return out;
}

function DeviceNode({ data, selected }: NodeProps<DeviceRFNode>) {
  return (
    <div
      className={cn(
        "w-[210px] cursor-pointer rounded-xl border-2 bg-card px-3 py-2 shadow-sm transition-shadow hover:shadow-md",
        selected ? "ring-2 ring-blue-500/60" : "",
      )}
      style={{ borderColor: data.groupColor || "#cbd5e1" }}
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!size-2.5 !border-2 !border-neutral-300 !bg-white dark:!border-neutral-600 dark:!bg-neutral-800"
      />
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono font-semibold text-sm">{data.ip}</span>
        {data.groupName ? (
          <span
            className="max-w-24 shrink-0 truncate rounded px-1.5 py-0.5 font-medium text-[10px]"
            style={{
              backgroundColor: `${data.groupColor || DEFAULT_GROUP_COLOR}22`,
              color: data.groupColor || DEFAULT_GROUP_COLOR,
            }}
            title={data.groupName}
          >
            {data.groupName}
          </span>
        ) : null}
      </div>
      <div className="mt-0.5 truncate text-muted-foreground text-xs" title={data.hostname}>
        {data.hostname || "—"}
      </div>
      {data.label ? (
        <div className="mt-1.5 truncate rounded bg-muted px-1.5 py-0.5 text-[11px]" title={data.label}>
          {data.label}
        </div>
      ) : null}
      {data.notes ? (
        <div className="mt-1 truncate text-[10px] text-muted-foreground" title={data.notes}>
          {data.notes}
        </div>
      ) : null}
      <Handle
        type="source"
        position={Position.Right}
        className="!size-2.5 !border-2 !border-blue-400 !bg-white dark:!bg-neutral-800"
      />
    </div>
  );
}

function NoteNode({ data, selected }: NodeProps<NoteRFNode>) {
  return (
    <div
      className={cn(
        "w-[200px] cursor-move rounded-lg border border-amber-300 bg-amber-50 p-2.5 text-amber-950 shadow-sm dark:border-amber-500/40 dark:bg-amber-950/50 dark:text-amber-100",
        selected ? "ring-2 ring-amber-500/60" : "",
      )}
    >
      <div className="mb-1 flex items-center gap-1 font-medium text-[10px] text-amber-700 dark:text-amber-300">
        <StickyNoteIcon className="size-3" /> 便签
      </div>
      <div className="whitespace-pre-wrap break-words text-xs leading-snug">{data.content}</div>
    </div>
  );
}

const nodeTypes = { topo: DeviceNode, note: NoteNode };

function nodePosition(index: number): { x: number; y: number } {
  return { x: 60 + (index % 4) * 250, y: 60 + Math.floor(index / 4) * 150 };
}

function downloadJSON(name: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

export function TopologyTab({ taskId, categoryName }: { taskId: string; categoryName?: string }) {
  const [data, setData] = React.useState<TopologyData>({ nodes: [], edges: [], groups: [], notes: [] });
  const [busy, setBusy] = React.useState(false);
  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<TopoRFNode>([]);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<RFEdge>([]);
  const [selected, setSelected] = React.useState<
    { kind: "node"; ip: string } | { kind: "note"; id: number } | { kind: "edge"; id: number } | null
  >(null);
  const [noteDraft, setNoteDraft] = React.useState("");
  const [groupDraft, setGroupDraft] = React.useState({ name: "", color: DEFAULT_GROUP_COLOR });
  const fileRef = React.useRef<HTMLInputElement | null>(null);
  const allowed = !categoryName || categoryName === "内网渗透";

  const load = React.useCallback(async () => {
    try {
      const d = await api.topology(taskId);
      setData({
        nodes: d.nodes ?? [],
        edges: d.edges ?? [],
        groups: d.groups ?? [],
        notes: d.notes ?? [],
      });
    } catch (e) {
      toast.error((e as Error).message || "加载拓扑失败");
    }
  }, [taskId]);
  React.useEffect(() => {
    if (allowed) void load();
  }, [allowed, load]);

  const groupsById = React.useMemo(() => new Map(data.groups.map((g) => [g.id, g])), [data.groups]);

  React.useEffect(() => {
    const needsLayout = data.nodes.length > 1 && data.nodes.every((n) => n.x === 0 && n.y === 0);
    const auto = needsLayout ? dagreAutoLayout(data.nodes, data.edges) : null;
    setRfNodes([
      ...data.nodes.map((n) => {
        const g = n.group_id ? groupsById.get(n.group_id) : undefined;
        return {
          id: n.ip,
          type: "topo" as const,
          position: auto?.get(n.ip) ?? { x: n.x, y: n.y },
          data: {
            ip: n.ip,
            hostname: n.hostname,
            label: n.display_label,
            notes: n.notes,
            groupName: g?.name,
            groupColor: g?.color,
          },
        };
      }),
      ...data.notes.map((n) => ({
        id: `note-${n.id}`,
        type: "note" as const,
        position: { x: n.x, y: n.y },
        data: { noteId: n.id, content: n.content },
        selectable: true,
      })),
    ]);
    setRfEdges(
      data.edges.map((e) => ({
        id: `e-${e.id}`,
        source: e.src_ip,
        target: e.dst_ip,
        label: e.label || undefined,
        type: "default",
        animated: false,
        labelShowBg: true,
        labelBgPadding: [4, 2] as [number, number],
        labelStyle: { fontSize: 10, fontWeight: 600 },
        style: { stroke: "#64748b", strokeWidth: 1.6 },
        markerEnd: { type: MarkerType.ArrowClosed, color: "#64748b", width: 14, height: 14 },
      })),
    );
  }, [data, groupsById, setRfNodes, setRfEdges]);

  async function refresh() {
    setBusy(true);
    try {
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function connect(c: Connection) {
    if (!c.source || !c.target) return;
    if (c.source === c.target) {
      toast.error("不能连接节点自身");
      return;
    }
    try {
      await api.addTopologyEdge(taskId, { src_ip: c.source, dst_ip: c.target, label: "" });
      toast.success("已连线");
      await load();
    } catch (e) {
      toast.error((e as Error).message || "连线失败");
    }
  }

  async function onNodeDragStop(_: MouseEvent | TouchEvent, node: TopoRFNode) {
    const x = Math.round(node.position.x);
    const y = Math.round(node.position.y);
    try {
      if (node.type === "note") {
        await api.patchTopologyNote(taskId, Number(node.id.replace("note-", "")), { x, y });
      } else {
        await api.patchTopologyNode(taskId, node.id, { x, y });
      }
    } catch (e) {
      toast.error((e as Error).message || "保存坐标失败");
    }
  }

  const selectedNode = selected?.kind === "node" ? data.nodes.find((n) => n.ip === selected.ip) : undefined;
  const selectedNote = selected?.kind === "note" ? data.notes.find((n) => n.id === selected.id) : undefined;
  const selectedEdge = selected?.kind === "edge" ? data.edges.find((e) => e.id === selected.id) : undefined;

  async function addNote() {
    const content = noteDraft.trim();
    if (!content) {
      toast.error("便签内容不能为空");
      return;
    }
    try {
      const pos = nodePosition(data.nodes.length + data.notes.length);
      await api.addTopologyNote(taskId, { content, x: pos.x, y: pos.y + 40 });
      setNoteDraft("");
      toast.success("便签已添加");
      await load();
    } catch (e) {
      toast.error((e as Error).message || "添加便签失败");
    }
  }

  async function addGroup() {
    const name = groupDraft.name.trim();
    if (!name) {
      toast.error("分组名称不能为空");
      return;
    }
    try {
      await api.addTopologyGroup(taskId, { name, color: groupDraft.color });
      setGroupDraft({ name: "", color: DEFAULT_GROUP_COLOR });
      toast.success("分组已创建");
      await load();
    } catch (e) {
      toast.error((e as Error).message || "创建分组失败");
    }
  }

  async function renameGroup(g: TopologyGroup) {
    const name = window.prompt("分组名称", g.name);
    if (name === null || !name.trim() || name.trim() === g.name) return;
    try {
      await api.patchTopologyGroup(taskId, g.id, { name: name.trim() });
      await load();
    } catch (e) {
      toast.error((e as Error).message || "重命名失败");
    }
  }

  async function removeGroup(g: TopologyGroup) {
    if (!window.confirm(`删除分组「${g.name}」？组内节点会保留，仅解除分组。`)) return;
    try {
      await api.deleteTopologyGroup(taskId, g.id);
      toast.success("分组已删除");
      await load();
    } catch (e) {
      toast.error((e as Error).message || "删除分组失败");
    }
  }

  async function importJSON(file: File) {
    setBusy(true);
    try {
      const raw = JSON.parse(await file.text()) as Partial<TopologyData>;
      const groups = Array.isArray(raw.groups) ? raw.groups : [];
      const nodes = Array.isArray(raw.nodes) ? raw.nodes : [];
      const edges = Array.isArray(raw.edges) ? raw.edges : [];
      const notes = Array.isArray(raw.notes) ? raw.notes : [];
      const groupMap = new Map<number, number>();
      for (const g of groups) {
        if (!g?.name?.trim()) continue;
        const created = await api.addTopologyGroup(taskId, {
          name: g.name.trim(),
          color: g.color || DEFAULT_GROUP_COLOR,
        });
        groupMap.set(g.id, created.id);
      }
      const importedIPs = new Set<string>();
      for (const n of nodes) {
        if (!n?.ip?.trim()) continue;
        const ip = n.ip.trim();
        await api.upsertTopologyNode(taskId, {
          ip,
          hostname: n.hostname || "",
          display_label: n.display_label || "",
          notes: n.notes || "",
          x: Number.isFinite(n.x) ? n.x : 0,
          y: Number.isFinite(n.y) ? n.y : 0,
          group_id: n.group_id ? (groupMap.get(n.group_id) ?? null) : null,
        });
        importedIPs.add(ip);
      }
      const existingIPs = new Set(data.nodes.map((n) => n.ip));
      const skippedEdges: string[] = [];
      for (const e of edges) {
        if (!e?.src_ip || !e?.dst_ip) continue;
        if (
          (!importedIPs.has(e.src_ip) && !existingIPs.has(e.src_ip)) ||
          (!importedIPs.has(e.dst_ip) && !existingIPs.has(e.dst_ip))
        ) {
          skippedEdges.push(`${e.src_ip}->${e.dst_ip}`);
          continue;
        }
        await api.addTopologyEdge(taskId, { src_ip: e.src_ip, dst_ip: e.dst_ip, label: e.label || "" });
      }
      for (const n of notes) {
        if (!n?.content?.trim()) continue;
        await api.addTopologyNote(taskId, {
          content: n.content,
          x: Number.isFinite(n.x) ? n.x : 0,
          y: Number.isFinite(n.y) ? n.y : 0,
        });
      }
      toast.success(
        skippedEdges.length
          ? `导入完成，跳过 ${skippedEdges.length} 条悬空边：${skippedEdges.slice(0, 3).join("、")}${skippedEdges.length > 3 ? "…" : ""}`
          : "导入完成",
      );
      await load();
    } catch (e) {
      toast.error((e as Error).message || "导入失败：JSON 格式不正确");
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  const clearSelection = () => {
    setSelected(null);
    void load();
  };
  let selectedTitle = "点击画布中的节点、便签或连线进行编辑。";
  let selectedBody: React.ReactNode = <p className="text-muted-foreground text-sm">暂无选中项。</p>;
  if (selectedNode) {
    selectedTitle = `节点 ${selectedNode.ip}`;
    selectedBody = (
      <NodeEditor
        key={selectedNode.ip}
        taskId={taskId}
        node={selectedNode}
        groups={data.groups}
        onDone={load}
        onDeleted={clearSelection}
      />
    );
  } else if (selectedEdge) {
    selectedTitle = `连线 #${selectedEdge.id}`;
    selectedBody = (
      <EdgeEditor key={selectedEdge.id} taskId={taskId} edge={selectedEdge} onDone={load} onDeleted={clearSelection} />
    );
  } else if (selectedNote) {
    selectedTitle = `便签 #${selectedNote.id}`;
    selectedBody = (
      <NoteEditor key={selectedNote.id} taskId={taskId} note={selectedNote} onDone={load} onDeleted={clearSelection} />
    );
  }

  if (!allowed) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>内网拓扑</CardTitle>
          <CardDescription>拓扑画布仅对内网渗透分类的任务开放，当前任务分类为「{categoryName}」。</CardDescription>
        </CardHeader>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3">
          <div>
            <CardTitle>内网拓扑</CardTitle>
            <CardDescription>
              拖拽节点保存坐标，拖动节点右侧圆点连线；支持分组、便签、换行编辑与 JSON 导入导出。
              {categoryName ? `（当前分类：${categoryName}）` : ""}
            </CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={refresh} disabled={busy}>
              <RefreshCwIcon className="size-4" /> 刷新
            </Button>
            <Button variant="outline" size="sm" onClick={() => downloadJSON(`topology-${taskId}.json`, data)}>
              <DownloadIcon className="size-4" /> 导出
            </Button>
            <Button variant="outline" size="sm" onClick={() => fileRef.current?.click()} disabled={busy}>
              <UploadIcon className="size-4" /> 导入
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) void importJSON(f);
              }}
            />
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
            <div className="h-[65vh] min-h-[520px] overflow-hidden rounded-xl border bg-muted/20">
              <ReactFlowProvider>
                <ReactFlow
                  nodes={rfNodes}
                  edges={rfEdges}
                  onNodesChange={onNodesChange}
                  onEdgesChange={onEdgesChange}
                  onConnect={(c) => void connect(c)}
                  onNodeDragStop={(e, n) => void onNodeDragStop(e, n)}
                  onNodeClick={(_, n) => {
                    if (n.type === "note") {
                      setSelected({ kind: "note", id: Number(n.id.replace("note-", "")) });
                    } else {
                      setSelected({ kind: "node", ip: n.id });
                    }
                  }}
                  onEdgeClick={(_, e) => setSelected({ kind: "edge", id: Number(e.id.replace("e-", "")) })}
                  onPaneClick={() => setSelected(null)}
                  nodeTypes={nodeTypes}
                  deleteKeyCode={null}
                  fitView
                  fitViewOptions={{ padding: 0.2 }}
                  proOptions={{ hideAttribution: true }}
                  minZoom={0.1}
                  className="!bg-[#f0f2f7] dark:!bg-neutral-950"
                >
                  <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
                  <Controls showInteractive={false} />
                  <MiniMap pannable zoomable nodeStrokeWidth={0} />
                  <Panel position="top-left">
                    <Badge variant="outline" className="bg-card/90 backdrop-blur">
                      {data.nodes.length} 节点 · {data.edges.length} 边 · {data.groups.length} 分组 ·{" "}
                      {data.notes.length} 便签
                    </Badge>
                  </Panel>
                </ReactFlow>
              </ReactFlowProvider>
            </div>

            <div className="space-y-4">
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">选中项</CardTitle>
                  <CardDescription>{selectedTitle}</CardDescription>
                </CardHeader>
                <CardContent>{selectedBody}</CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-base">
                    <UsersIcon className="size-4" /> 分组
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex items-end gap-2">
                    <div className="min-w-0 flex-1 space-y-1">
                      <Label htmlFor="topo-group-name">名称</Label>
                      <Input
                        id="topo-group-name"
                        value={groupDraft.name}
                        placeholder="如 DMZ / 核心区"
                        onChange={(e) => setGroupDraft((s) => ({ ...s, name: e.target.value }))}
                      />
                    </div>
                    <input
                      type="color"
                      aria-label="分组颜色"
                      className="h-9 w-10 cursor-pointer rounded border bg-background"
                      value={groupDraft.color}
                      onChange={(e) => setGroupDraft((s) => ({ ...s, color: e.target.value }))}
                    />
                    <Button size="sm" onClick={() => void addGroup()}>
                      <PlusIcon className="size-4" /> 新建
                    </Button>
                  </div>
                  <div className="space-y-1.5">
                    {data.groups.length === 0 ? (
                      <p className="text-muted-foreground text-sm">暂无分组。</p>
                    ) : (
                      data.groups.map((g) => {
                        const count = data.nodes.filter((n) => n.group_id === g.id).length;
                        return (
                          <div key={g.id} className="flex items-center gap-2 rounded-md border px-2 py-1.5 text-sm">
                            <span
                              className="size-3 shrink-0 rounded-full"
                              style={{ backgroundColor: g.color || "#94a3b8" }}
                            />
                            <span className="min-w-0 grow truncate">{g.name}</span>
                            <Badge variant="outline" className="shrink-0 text-[10px]">
                              {count}
                            </Badge>
                            <Button
                              size="icon"
                              variant="ghost"
                              className="size-7"
                              title="重命名"
                              onClick={() => void renameGroup(g)}
                            >
                              <SaveIcon className="size-3.5" />
                            </Button>
                            <Button
                              size="icon"
                              variant="ghost"
                              className="size-7 text-destructive"
                              title="删除分组"
                              onClick={() => void removeGroup(g)}
                            >
                              <Trash2Icon className="size-3.5" />
                            </Button>
                          </div>
                        );
                      })
                    )}
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-base">
                    <StickyNoteIcon className="size-4" /> 便签
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-2">
                  <Textarea
                    value={noteDraft}
                    placeholder="记录判断、凭据线索、下一步动作…"
                    rows={3}
                    onChange={(e) => setNoteDraft(e.target.value)}
                  />
                  <Button size="sm" className="w-full" onClick={() => void addNote()}>
                    <PlusIcon className="size-4" /> 添加便签
                  </Button>
                </CardContent>
              </Card>
            </div>
          </div>
        </CardContent>
      </Card>

      <AddNodeCard
        taskId={taskId}
        groups={data.groups}
        nextIndex={data.nodes.length + data.notes.length}
        onDone={load}
      />
    </div>
  );
}

function NodeEditor({
  taskId,
  node,
  groups,
  onDone,
  onDeleted,
}: {
  taskId: string;
  node: TopologyNode;
  groups: TopologyGroup[];
  onDone: () => Promise<void> | void;
  onDeleted: () => void;
}) {
  const [hostname, setHostname] = React.useState(node.hostname);
  const [label, setLabel] = React.useState(node.display_label);
  const [notes, setNotes] = React.useState(node.notes);
  const [groupId, setGroupId] = React.useState(node.group_id ? String(node.group_id) : "");
  const [busy, setBusy] = React.useState(false);

  async function save() {
    setBusy(true);
    try {
      await api.patchTopologyNode(taskId, node.ip, {
        hostname,
        display_label: label,
        notes,
        group_id: groupId ? Number(groupId) : null,
      });
      toast.success("节点已保存");
      await onDone();
    } catch (e) {
      toast.error((e as Error).message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm(`删除节点 ${node.ip}？关联连线会一并删除。`)) return;
    try {
      await api.deleteTopologyNode(taskId, node.ip);
      toast.success("节点已删除");
      onDeleted();
    } catch (e) {
      toast.error((e as Error).message || "删除失败");
    }
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label>IP</Label>
        <Input value={node.ip} readOnly className="font-mono" />
      </div>
      <div className="space-y-1">
        <Label htmlFor="topo-node-hostname">主机名</Label>
        <Input id="topo-node-hostname" value={hostname} onChange={(e) => setHostname(e.target.value)} />
      </div>
      <div className="space-y-1">
        <Label htmlFor="topo-node-label">显示标签</Label>
        <Input id="topo-node-label" value={label} onChange={(e) => setLabel(e.target.value)} />
      </div>
      <div className="space-y-1">
        <Label htmlFor="topo-node-group">分组</Label>
        <select
          id="topo-node-group"
          className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          value={groupId}
          onChange={(e) => setGroupId(e.target.value)}
        >
          <option value="">未分组</option>
          {groups.map((g) => (
            <option key={g.id} value={g.id}>
              {g.name}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="topo-node-notes">备注</Label>
        <Textarea id="topo-node-notes" rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
      </div>
      <div className="flex gap-2">
        <Button size="sm" className="grow" onClick={() => void save()} disabled={busy}>
          <SaveIcon className="size-4" /> 保存
        </Button>
        <Button size="sm" variant="destructive" onClick={() => void remove()}>
          <Trash2Icon className="size-4" /> 删除
        </Button>
      </div>
    </div>
  );
}

function EdgeEditor({
  taskId,
  edge,
  onDone,
  onDeleted,
}: {
  taskId: string;
  edge: TopologyEdge;
  onDone: () => Promise<void> | void;
  onDeleted: () => void;
}) {
  const [label, setLabel] = React.useState(edge.label);
  const [busy, setBusy] = React.useState(false);

  async function save() {
    setBusy(true);
    try {
      await api.patchTopologyEdge(taskId, edge.id, { label });
      toast.success("连线已保存");
      await onDone();
    } catch (e) {
      toast.error((e as Error).message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    try {
      await api.deleteTopologyEdge(taskId, edge.id);
      toast.success("连线已删除");
      onDeleted();
    } catch (e) {
      toast.error((e as Error).message || "删除失败");
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-sm">
        <Badge variant="outline" className="font-mono">
          {edge.src_ip}
        </Badge>
        <LinkIcon className="size-3.5 text-muted-foreground" />
        <Badge variant="outline" className="font-mono">
          {edge.dst_ip}
        </Badge>
      </div>
      <div className="space-y-1">
        <Label htmlFor="topo-edge-label">连线标签</Label>
        <Input
          id="topo-edge-label"
          value={label}
          placeholder="如 3389 / 横向移动"
          onChange={(e) => setLabel(e.target.value)}
        />
      </div>
      <div className="flex gap-2">
        <Button size="sm" className="grow" onClick={() => void save()} disabled={busy}>
          <SaveIcon className="size-4" /> 保存
        </Button>
        <Button size="sm" variant="destructive" onClick={() => void remove()}>
          <Trash2Icon className="size-4" /> 删除
        </Button>
      </div>
    </div>
  );
}

function NoteEditor({
  taskId,
  note,
  onDone,
  onDeleted,
}: {
  taskId: string;
  note: TopologyNote;
  onDone: () => Promise<void> | void;
  onDeleted: () => void;
}) {
  const [content, setContent] = React.useState(note.content);
  const [busy, setBusy] = React.useState(false);

  async function save() {
    if (!content.trim()) {
      toast.error("便签内容不能为空");
      return;
    }
    setBusy(true);
    try {
      await api.patchTopologyNote(taskId, note.id, { content });
      toast.success("便签已保存");
      await onDone();
    } catch (e) {
      toast.error((e as Error).message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    try {
      await api.deleteTopologyNote(taskId, note.id);
      toast.success("便签已删除");
      onDeleted();
    } catch (e) {
      toast.error((e as Error).message || "删除失败");
    }
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="topo-note-content">内容</Label>
        <Textarea id="topo-note-content" rows={5} value={content} onChange={(e) => setContent(e.target.value)} />
      </div>
      <div className="flex gap-2">
        <Button size="sm" className="grow" onClick={() => void save()} disabled={busy}>
          <SaveIcon className="size-4" /> 保存
        </Button>
        <Button size="sm" variant="destructive" onClick={() => void remove()}>
          <Trash2Icon className="size-4" /> 删除
        </Button>
      </div>
    </div>
  );
}

function AddNodeCard({
  taskId,
  groups,
  nextIndex,
  onDone,
}: {
  taskId: string;
  groups: TopologyGroup[];
  nextIndex: number;
  onDone: () => Promise<void> | void;
}) {
  const [ip, setIP] = React.useState("");
  const [hostname, setHostname] = React.useState("");
  const [label, setLabel] = React.useState("");
  const [groupId, setGroupId] = React.useState("");
  const [busy, setBusy] = React.useState(false);

  async function add() {
    const value = ip.trim();
    if (!value) {
      toast.error("IP 必填");
      return;
    }
    setBusy(true);
    try {
      const pos = nodePosition(nextIndex);
      await api.upsertTopologyNode(taskId, {
        ip: value,
        hostname: hostname.trim(),
        display_label: label.trim(),
        group_id: groupId ? Number(groupId) : null,
        x: pos.x,
        y: pos.y,
      });
      toast.success("节点已保存");
      setIP("");
      setHostname("");
      setLabel("");
      setGroupId("");
      await onDone();
    } catch (e) {
      toast.error((e as Error).message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">添加节点</CardTitle>
        <CardDescription>节点以 IP 为唯一键；重复添加会更新已有节点。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 md:grid-cols-5">
        <div className="space-y-1.5">
          <Label htmlFor="topo-add-ip">IP / 主机</Label>
          <Input id="topo-add-ip" value={ip} onChange={(e) => setIP(e.target.value)} placeholder="10.0.0.5" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="topo-add-hostname">主机名</Label>
          <Input id="topo-add-hostname" value={hostname} onChange={(e) => setHostname(e.target.value)} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="topo-add-label">显示标签</Label>
          <Input id="topo-add-label" value={label} onChange={(e) => setLabel(e.target.value)} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="topo-add-group">分组</Label>
          <select
            id="topo-add-group"
            className="h-9 w-full rounded-md border bg-background px-3 text-sm"
            value={groupId}
            onChange={(e) => setGroupId(e.target.value)}
          >
            <option value="">未分组</option>
            {groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        </div>
        <div className="flex items-end">
          <Button className="w-full" onClick={() => void add()} disabled={busy}>
            <PlusIcon className="size-4" /> 添加节点
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
