import {
  BellRing,
  Bot,
  Brain,
  CalendarDays,
  ClipboardList,
  FolderKanban,
  FolderOpen,
  Inbox,
  LibraryBig,
  LayoutDashboard,
  type LucideIcon,
  MessageSquare,
  NotebookPen,
  Plug,
  Radio,
  ScrollText,
  Settings2,
  ShieldAlert,
  Sparkles,
  Sun,
  Target,
  Terminal,
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
}

interface NavItemBase {
  id: string;
  title: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
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
  collapsedByDefault?: boolean;
  items: NavMainItem[];
}

export const sidebarItems: NavGroup[] = [
  {
    id: 1,
    label: "工作台",
    items: [
      { id: "dashboard", title: "工作台", url: "/dashboard", icon: LayoutDashboard },
    ],
  },
  {
    id: 2,
    label: "个人工作",
    items: [
      { id: "work-inbox", title: "收件箱", url: "/work/inbox", icon: Inbox },
      { id: "work-today", title: "今日", url: "/work/today", icon: Sun },
      { id: "work-projects", title: "项目", url: "/work/projects", icon: FolderKanban },
      { id: "work-calendar", title: "日历", url: "/work/calendar", icon: CalendarDays },
      { id: "work-notes", title: "笔记", url: "/work/notes", icon: NotebookPen },
      { id: "work-assets", title: "资料资产", url: "/work/assets", icon: LibraryBig },
    ],
  },
  {
    id: 3,
    label: "智能体",
    items: [
      { id: "chat", title: "对话", url: "/chat", icon: MessageSquare },
      { id: "tasks", title: "任务", url: "/function/tasks", icon: Target },
      { id: "commands", title: "工具执行", url: "/function/commands", icon: Terminal },
      { id: "llm-records", title: "LLM 录制", url: "/function/llm-records", icon: Radio },
      { id: "workspace", title: "工作空间", url: "/function/workspace", icon: FolderOpen },
    ],
  },
  {
    id: 4,
    label: "系统",
    collapsedByDefault: true,
    items: [
      { id: "llm", title: "LLM", url: "/system/llm", icon: Brain },
      { id: "agents", title: "Agent", url: "/system/agents", icon: Bot },
      { id: "mcp", title: "MCP", url: "/system/mcp", icon: Plug },
      { id: "skills", title: "Skill", url: "/system/skills", icon: Sparkles },
      { id: "tools", title: "工具", url: "/system/tools", icon: Wrench },
      { id: "notify", title: "通知推送", url: "/system/notify", icon: BellRing },
      { id: "intercept", title: "执行审批", url: "/system/intercept", icon: ShieldAlert },
      { id: "approvals", title: "审批记录", url: "/system/intercept/approvals", icon: ClipboardList },
      { id: "logs", title: "日志", url: "/system/logs", icon: ScrollText },
      { id: "settings", title: "系统配置", url: "/system/settings", icon: Settings2 },
    ],
  },
];
