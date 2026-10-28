"use client";

import { Badge } from "@/components/ui/badge";
import type { WorkTask } from "@/lib/types";
import { cn } from "@/lib/utils";

const automationMeta: Record<WorkTask["automation_status"], { label: string; className: string }> = {
  pending: { label: "自动待执行", className: "border-muted-foreground/30 bg-muted text-muted-foreground" },
  running: {
    label: "自动执行中",
    className: "border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-400",
  },
  done: {
    label: "自动已完成",
    className: "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  },
  failed: {
    label: "自动执行失败",
    className: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-400",
  },
  canceled: { label: "自动已取消", className: "border-muted-foreground/30 bg-muted text-muted-foreground" },
};

export function WorkAutomationBadge({ task, className }: { task: WorkTask; className?: string }) {
  if (task.parent_task_id) return null;
  const meta = automationMeta[task.automation_status] ?? automationMeta.pending;
  const paused = task.automation_status === "running" && task.artex_task?.paused;
  return (
    <Badge variant="outline" className={cn("whitespace-nowrap", meta.className, className)}>
      {paused ? "执行中 / 已暂停" : meta.label}
    </Badge>
  );
}

export function automationStatusLabel(status: WorkTask["automation_status"]) {
  return automationMeta[status]?.label ?? automationMeta.pending.label;
}
