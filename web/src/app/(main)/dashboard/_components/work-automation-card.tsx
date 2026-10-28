"use client";

import * as React from "react";

import {
  AlertCircle,
  ArrowRight,
  ExternalLink,
  Loader2,
  Play,
  RefreshCw,
  RotateCw,
  XCircle,
} from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { WorkAutomationOverview, WorkTask } from "@/lib/types";
import { WORK_CHANGED_EVENT, emitWorkChanged } from "@/lib/work-events";
import { cn } from "@/lib/utils";

const statusMeta: Record<
  WorkTask["automation_status"],
  { label: string; dot: string; badge: string }
> = {
  pending: {
    label: "自动待执行",
    dot: "bg-amber-500",
    badge: "border-amber-500/30 bg-amber-500/10 text-amber-700",
  },
  running: {
    label: "自动执行中",
    dot: "bg-blue-500",
    badge: "border-blue-500/30 bg-blue-500/10 text-blue-700",
  },
  done: {
    label: "自动已完成",
    dot: "bg-emerald-500",
    badge: "border-emerald-500/30 bg-emerald-500/10 text-emerald-700",
  },
  failed: {
    label: "自动执行失败",
    dot: "bg-red-500",
    badge: "border-red-500/30 bg-red-500/10 text-red-700",
  },
  canceled: {
    label: "自动已取消",
    dot: "bg-muted-foreground",
    badge: "border-border bg-muted text-muted-foreground",
  },
};

function taskHref(task: WorkTask) {
  return task.work_task_url || `/work/inbox?task=${task.id}`;
}

function artexHref(task: WorkTask) {
  if (task.artex_task?.deleted) return "";
  if (task.artex_task_url) return task.artex_task_url;
  if (!task.artex_task_id || !task.artex_task) return "";
  return `/function/tasks/detail?id=${encodeURIComponent(task.artex_task_id)}&work_task=${encodeURIComponent(String(task.id))}`;
}

function AutomationTaskRow({
  task,
  busy,
  onRun,
  onCancel,
}: {
  task: WorkTask;
  busy: boolean;
  onRun: (task: WorkTask) => void;
  onCancel: (task: WorkTask) => void;
}) {
  const meta = statusMeta[task.automation_status] ?? statusMeta.pending;
  const executionHref = artexHref(task);
  const isRootTask = task.parent_task_id == null;
  const canRun =
    isRootTask && (task.automation_status === "pending" || task.automation_status === "failed");
  const canCancel =
    isRootTask && (task.cancel_available || task.automation_status === "running");
  const resultPreview = task.automation_result?.trim().split(/\r?\n/, 1)[0] || "";

  return (
    <div className="rounded-xl border bg-background/80 p-2.5">
      <div className="flex items-start gap-2.5">
        <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", meta.dot)} />
        <Link
          href={taskHref(task)}
          className="group min-w-0 flex-1 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={`查看个人任务：${task.title}`}
        >
          <span className="flex items-start gap-1">
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium group-hover:underline">{task.title}</span>
              <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                {task.automation_error ||
                  (task.artex_task?.deleted
                    ? "关联执行任务已删除，最终结果已保留"
                    : task.artex_task?.paused
                    ? "关联执行已暂停"
                    : task.automation_status === "done"
                      ? resultPreview || "自动执行已完成，打开任务查看执行结果"
                    : task.artex_task_id
                      ? `执行任务 #${task.artex_task_id}`
                        : task.automation_status === "canceled"
                          ? "自动执行已取消"
                          : "等待调度器拾取")}
              </span>
            </span>
            <ArrowRight className="mt-0.5 size-3.5 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
          </span>
        </Link>
        <Badge variant="outline" className={cn("shrink-0 whitespace-nowrap", meta.badge)}>
          {task.automation_status === "running" && task.artex_task?.paused
            ? "执行中 / 已暂停"
            : meta.label}
        </Badge>
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-end gap-2 sm:gap-1.5">
        {executionHref && (
          <Button variant="outline" size="xs" className="h-10 px-3 sm:h-6 sm:px-2" asChild>
            <Link href={executionHref} aria-label={`查看「${task.title}」的多 Agent 执行过程`}>
              <ExternalLink className="size-3" />
              查看多 Agent 执行过程
            </Link>
          </Button>
        )}
        {canRun && (
          <Button
            size="xs"
            className="h-10 px-3 sm:h-6 sm:px-2"
            disabled={busy}
            onClick={() => onRun(task)}
          >
            {busy ? (
              <Loader2 className="size-3 animate-spin" />
            ) : task.automation_status === "failed" ? (
              <RotateCw className="size-3" />
            ) : (
              <Play className="size-3" />
            )}
            {task.automation_status === "failed" ? "重试" : "立即执行"}
          </Button>
        )}
        {canCancel && (
          <Button
            variant="ghost"
            size="xs"
            className="h-10 px-3 sm:h-6 sm:px-2"
            disabled={busy}
            onClick={() => onCancel(task)}
          >
            {busy ? <Loader2 className="size-3 animate-spin" /> : <XCircle className="size-3" />}
            取消
          </Button>
        )}
      </div>
    </div>
  );
}

export function WorkAutomationCard({ refreshKey = 0 }: { refreshKey?: number }) {
  const [data, setData] = React.useState<WorkAutomationOverview | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [actionError, setActionError] = React.useState<string>();
  const [busyTaskID, setBusyTaskID] = React.useState<number>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const requestSequence = React.useRef(0);

  const load = React.useCallback(async (quiet = false) => {
    const requestID = ++requestSequence.current;
    if (!quiet) setLoading(true);
    try {
      const result = await api.workAutomationOverview();
      if (requestID !== requestSequence.current) return;
      setData(result);
      setError(undefined);
    } catch (cause) {
      if (requestID === requestSequence.current) setError((cause as Error).message);
    } finally {
      if (requestID === requestSequence.current) setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load, reloadKey, refreshKey]);

  React.useEffect(() => {
    const reload = () => void load(true);
    window.addEventListener(WORK_CHANGED_EVENT, reload);
    return () => window.removeEventListener(WORK_CHANGED_EVENT, reload);
  }, [load]);

  async function runTask(task: WorkTask) {
    setBusyTaskID(task.id);
    setActionError(undefined);
    try {
      const updated = await api.runWorkTask(task.id);
      setData((current) =>
        current
          ? {
              ...current,
              tasks: current.tasks.map((item) => (item.id === updated.id ? updated : item)),
            }
          : current,
      );
      emitWorkChanged();
      await load(true);
    } catch (cause) {
      setActionError(`启动「${task.title}」失败：${(cause as Error).message}`);
    } finally {
      setBusyTaskID(undefined);
    }
  }

  async function cancelTask(task: WorkTask) {
    setBusyTaskID(task.id);
    setActionError(undefined);
    try {
      const updated = await api.cancelWorkTaskAutomation(task.id);
      setData((current) =>
        current
          ? {
              ...current,
              tasks: current.tasks.map((item) => (item.id === updated.id ? updated : item)),
            }
          : current,
      );
      emitWorkChanged();
      await load(true);
    } catch (cause) {
      setActionError(`取消「${task.title}」失败：${(cause as Error).message}`);
    } finally {
      setBusyTaskID(undefined);
    }
  }

  const recent = React.useMemo(() => {
    const activeIDs = new Set(data?.tasks.map((task) => task.id) ?? []);
    return (data?.recent ?? []).filter((task) => !activeIDs.has(task.id)).slice(0, 4);
  }, [data]);

  return (
    <Card className="h-full">
      <CardHeader className="pb-0">
        <div className="flex items-center justify-between gap-2">
          <div>
            <p className="text-sm font-semibold">个人自动化执行</p>
            <p className="text-xs text-muted-foreground">个人任务自动派发给多 Agent 执行</p>
          </div>
          <div className="flex items-center gap-2 sm:gap-1">
            <Button
              variant="ghost"
              size="icon-xs"
              className="size-10 sm:size-6"
              onClick={() => void load()}
              disabled={loading}
              aria-label="刷新自动化任务"
            >
              <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
            </Button>
            <Button variant="ghost" size="xs" className="h-10 px-3 sm:h-6 sm:px-2" asChild>
              <Link href="/work/inbox">查看</Link>
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading && !data ? (
          <div className="space-y-2">
            <Skeleton className="h-16" />
            <Skeleton className="h-20" />
            <Skeleton className="h-20" />
          </div>
        ) : error && !data ? (
          <div className="flex flex-col items-center gap-2 py-4 text-sm text-destructive">
            <span>{error}</span>
            <Button
              variant="outline"
              size="xs"
              className="h-10 px-3 sm:h-6 sm:px-2"
              onClick={() => setReloadKey((key) => key + 1)}
            >
              重试
            </Button>
          </div>
        ) : (
          <>
            <div className="grid grid-cols-3 gap-2 text-center">
              <div className="rounded-xl border bg-amber-500/10 py-2">
                <p className="text-lg font-semibold text-amber-700">{data?.summary.pending ?? 0}</p>
                <p className="text-xs text-muted-foreground">待执行</p>
              </div>
              <div className="rounded-xl border bg-blue-500/10 py-2">
                <p className="text-lg font-semibold text-blue-700">{data?.summary.running ?? 0}</p>
                <p className="text-xs text-muted-foreground">执行中</p>
              </div>
              <div className="rounded-xl border bg-red-500/10 py-2">
                <p className="text-lg font-semibold text-red-700">{data?.summary.failed ?? 0}</p>
                <p className="text-xs text-muted-foreground">失败</p>
              </div>
            </div>

            {(error || actionError) && (
              <div
                className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"
                role="alert"
              >
                <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
                <span className="min-w-0 flex-1">{actionError || error}</span>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="size-10 sm:size-6"
                  onClick={() => {
                    setActionError(undefined);
                    setError(undefined);
                  }}
                  aria-label="关闭错误提示"
                >
                  <XCircle className="size-3.5" />
                </Button>
              </div>
            )}

            {data && data.tasks.length > 0 ? (
              <div className="space-y-2">
                {data.tasks.slice(0, 6).map((task) => (
                  <AutomationTaskRow
                    key={task.id}
                    task={task}
                    busy={busyTaskID === task.id}
                    onRun={(item) => void runTask(item)}
                    onCancel={(item) => void cancelTask(item)}
                  />
                ))}
              </div>
            ) : (
              <div className="rounded-xl border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
                暂无进行中的自动化任务。新建个人任务后会自动进入队列。
              </div>
            )}

            {recent.length > 0 && (
              <section className="space-y-2 border-t pt-3">
                <div className="flex items-center justify-between">
                  <p className="text-xs font-medium">最近自动结果</p>
                  <span className="text-xs text-muted-foreground">近 7 天</span>
                </div>
                {recent.map((task) => (
                  <AutomationTaskRow
                    key={`recent-${task.id}`}
                    task={task}
                    busy={busyTaskID === task.id}
                    onRun={(item) => void runTask(item)}
                    onCancel={(item) => void cancelTask(item)}
                  />
                ))}
              </section>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
