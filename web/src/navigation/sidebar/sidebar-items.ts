import {
  Activity,
  Ban,
  BellRing,
  BookOpen,
  Bot,
  Brain,
  Bug,
  ClipboardList,
  FileClock,
  FileArchive,
  FileCode2,
  FileText,
  FolderOpen,
  KeyRound,
  FolderSync,
  LayoutDashboard,
  type LucideIcon,
  MessageSquare,
  MonitorCog,
  Network,
  Plug,
  Radio,
  RadioTower,
  ScrollText,
  Settings2,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Target,
  Terminal,
  UsersRound,
  Wrench,
} from "lucide-react";

export type NavBadge = "new" | "soon";

export interface NavSubItem {
  id: string;
  title: string;
  url: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
  /** 需要的权限键；空=不受权限控制。 */
  perm?: string;
}

interface NavItemBase {
  id: string;
  title: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
  perm?: string;
}

export interface NavMainLinkItem extends NavItemBase {
  url: string;
  subItems?: never;
}

export interface NavMainParentItem extends NavItemBase {
  subItems: NavSubItem[];
}

export type NavMainItem = NavMainLinkItem | NavMainParentItem;

export interface NavGroup {
  id: number;
  label?: string;
  items: NavMainItem[];
}

export const sidebarItems: NavGroup[] = [
  {
    id: 1,
    label: "功能",
    items: [
      { id: "dashboard", title: "仪表盘", url: "/dashboard", icon: LayoutDashboard, perm: "dashboard:read" },
      { id: "chat", title: "对话", url: "/chat", icon: MessageSquare, perm: "task:read" },
      { id: "tasks", title: "任务", url: "/function/tasks", icon: Target, perm: "task:read" },
      { id: "findings", title: "发现", url: "/function/findings", icon: Bug, perm: "finding:read" },
      { id: "traffic", title: "流量", url: "/function/traffic", icon: Activity, perm: "traffic:read" },
      { id: "commands", title: "工具执行", url: "/function/commands", icon: Terminal, perm: "task:read" },
      { id: "llm-records", title: "LLM 录制", url: "/function/llm-records", icon: Radio, perm: "llm:read" },
      { id: "assets", title: "资产", url: "/function/assets", icon: Network, perm: "asset:read" },
      { id: "sync", title: "资产同步", url: "/function/sync", icon: FolderSync, perm: "asset:read" },
      { id: "workspace", title: "工作空间", url: "/function/workspace", icon: FolderOpen, perm: "workspace:read" },
    ],
  },
  {
    id: 2,
    label: "系统",
    items: [
      { id: "llm", title: "LLM", url: "/system/llm", icon: Brain, perm: "llm:read" },
      { id: "agents", title: "Agent", url: "/system/agents", icon: Bot, perm: "agent:read" },
      { id: "templates", title: "提示词模板", url: "/system/templates", icon: FileCode2, perm: "task:read" },
      { id: "reports", title: "报告管理", url: "/system/reports", icon: FileText, perm: "report:read" },
      { id: "knowledge", title: "知识库", url: "/system/knowledge", icon: BookOpen, perm: "knowledge:read" },
      { id: "credentials", title: "内网凭据", url: "/system/credentials", icon: KeyRound, perm: "credential:read" },
      { id: "mcp", title: "MCP", url: "/system/mcp", icon: Plug, perm: "mcp:read" },
      { id: "skills", title: "Skill", url: "/system/skills", icon: Sparkles, perm: "skill:read" },
      { id: "tools", title: "工具", url: "/system/tools", icon: Wrench, perm: "tool:read" },
      { id: "notify", title: "通知推送", url: "/system/notify", icon: BellRing, perm: "notify:read" },
      { id: "vulnintel", title: "漏洞情报", url: "/system/vulnintel", icon: ShieldCheck, perm: "vulnintel:read" },
      { id: "proxypool", title: "代理池", url: "/system/proxypool", icon: Network, perm: "proxy:read" },
      { id: "egress", title: "SSH 跳板", url: "/system/egress", icon: Network, perm: "config:read" },
      { id: "hostenv", title: "宿主环境", url: "/system/hostenv", icon: MonitorCog, perm: "hostenv:read" },
      { id: "c2", title: "监听器 / C2", url: "/system/c2", icon: RadioTower, perm: "c2:read" },
      { id: "intercept", title: "拦截规则", url: "/system/intercept", icon: ShieldAlert, perm: "intercept:read" },
      { id: "asset-intercept", title: "资产拦截", url: "/system/intercept/assets", icon: Ban, perm: "intercept:read" },
      { id: "approvals", title: "拦截审批", url: "/system/intercept/approvals", icon: ClipboardList, perm: "intercept:read" },
      { id: "rbac", title: "用户与角色", url: "/system/rbac", icon: UsersRound, perm: "rbac:read" },
      { id: "audit", title: "审计日志", url: "/system/audit", icon: FileClock, perm: "log:read" },
      { id: "logs", title: "日志", url: "/system/logs", icon: ScrollText, perm: "log:read" },
      { id: "settings", title: "系统配置", url: "/system/settings", icon: Settings2, perm: "config:read" },
      { id: "diagnostics", title: "诊断包", url: "/system/diagnostics", icon: FileArchive, perm: "config:read" },
    ],
  },
];

// filterSidebarByPermissions 根据 /rbac/me 返回的权限集合过滤导航。
// permissions=null（加载中）时原样返回，避免首屏闪空。
export function filterSidebarByPermissions(groups: NavGroup[], permissions: Set<string> | null): NavGroup[] {
  if (!permissions) return groups;
  return groups
    .map((g) => ({
      ...g,
      items: g.items
        .map((item) => {
          if (item.subItems) {
            const subItems = item.subItems.filter((s) => !s.perm || permissions.has(s.perm));
            if (subItems.length === 0) return null;
            return { ...item, subItems } as NavMainItem;
          }
          if (item.perm && !permissions.has(item.perm)) return null;
          return item;
        })
        .filter((x): x is NavMainItem => x !== null),
    }))
    .filter((g) => g.items.length > 0);
}
