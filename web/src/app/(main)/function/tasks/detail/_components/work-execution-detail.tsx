"use client";

import * as React from "react";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowLeftIcon,
  BotIcon,
  CheckCircle2Icon,
  Clock3Icon,
  ExternalLinkIcon,
  Loader2Icon,
  PlayIcon,
  RefreshCwIcon,
  RotateCwIcon,
  XCircleIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api, HttpError } from "@/lib/api";
import type { WorkTask } from "@/lib/types";
import { emitWorkChanged } from "@/lib/work-events";

import { automationStatusLabel } from "../../../../work/_components/work-automation";
import { WorkTaskActivity } from "../../../../work/_components/work-task-activity";

const terminalAutomation = new Set<WorkTask["automation_status"]>(["done", "failed", "canceled"]);

function statusTone(status: WorkTask["automation_status"]) {
  switch (status) {
    case "running":
      return "border-blue-500/30 bg-blue-500/10 text-blue-700";
    case "done":
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-700";
    case "failed":
      return "border-red-500/30 bg-red-500/10 text-red-700";
    case "canceled":
      return "border-border bg-muted text-muted-foreground";
    default:
      return "border-amber-500/30 bg-amber-500/10 text-amber-700";
  }
}

function formatDate(value?: string | null) {
  if (!value) return "未设置";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "时间未知";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function WorkExecutionDetail({
  workTaskID,
  artexTaskID,
}: {
  workTaskID: number;
  artexTaskID?: string;
}) {
  const [task, setTask] = React.useState<WorkTask | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [notFound, setNotFound] = React.useState(false);
  const [busy, setBusy] = React.useState<"run" | "cancel" | null>(null);
  const [reloadKey, setReloadKey] = React.useState(0);
  const requestSequence = React.useRef(0);

  const load = React.useCallback(
    async (quiet = false) => {
      const requestID = ++requestSequence.current;
      if (!quiet) setLoading(true);
      try {
        const fresh = await api.workTask(workTaskID);
        if (requestID !== requestSequence.current) return;
        setTask(fresh);
        setError(undefined);
        setNotFound(false);
      } catch (cause) {
        if (requestID !== requestSequence.current) return;
        const message = (cause as Error).message || "任务读取失败";
        setError(message);
        setNotFound(cause instanceof HttpError && (cause.status === 404 || cause.status === 410));
      } finally {
        if (requestID === requestSequence.current) setLoading(false);
      }
    },
    [workTaskID],
  );

  React.useEffect(() => {
    void load();
  }, [load, reloadKey]);

  React.useEffect(() => {
    if (!task || terminalAutomation.has(task.automation_status)) return;
    const timer = window.setInterval(() => void load(true), 4000);
    return () => window.clearInterval(timer);
  }, [load, task]);

  React.useEffect(() => {
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") void load(true);
    };
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => document.removeEventListener("visibilitychange", refreshWhenVisible);
  }, [load]);

  async function run() {
    setBusy("run");
    try {
      const updated = await api.runWorkTask(workTaskID);
      setTask(updated);
      emitWorkChanged();
      toast.success(updated.automation_status === "running" ? "已启动多 Agent 执行" : "已加入执行队列");
    } catch (cause) {
      toast.error(`启动失败：${(cause as Error).message}`);
    } finally {
      setBusy(null);
    }
  }

  async function cancel() {
    setBusy("cancel");
    try {
      const updated = await api.cancelWorkTaskAutomation(workTaskID);
      setTask(updated);
      emitWorkChanged();
      toast.success("已取消自动执行");
    } catch (cause) {
      toast.error(`取消失败：${(cause as Error).message}`);
    } finally {
      setBusy(null);
    }
  }

  const linkedArtexDeleted = Boolean(task?.artex_task?.deleted);
  const linkedArtexID = task
    ? linkedArtexDeleted
      ? ""
      : task.artex_task_id || ""
    : artexTaskID;
  const workTaskHref = task?.work_task_url || `/work/inbox?task=${workTaskID}`;
  const isRootTask = task?.parent_task_id == null;
  const canRun =
    isRootTask &&
    (task?.automation_status === "pending" || task?.automation_status === "failed");
  const canCancel =
    isRootTask && (task?.cancel_available || task?.automation_status === "running");
  const finalResult =
    task?.automation_result?.trim() ||
    (task?.automation_status === "done"
      ? "执行已完成，但活动记录里没有可展示的文本结果。请展开执行过程查看 Agent 的完整活动。"
      : "");

  if (loading && !task) {
    return (
      <div className="space-y-4 p-4 lg:p-6">
        <Skeleton className="h-28 w-full rounded-2xl" />
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <Skeleton className="h-80 rounded-2xl" />
          <Skeleton className="h-80 rounded-2xl" />
        </div>
      </div>
    );
  }

  if (!task) {
    return (
      <div className="flex min-h-[60vh] flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
        <AlertTriangle className="size-9 text-destructive" />
        <div>
          <p className="font-semibold">{notFound ? "个人任务不存在或已删除" : "个人执行详情读取失败"}</p>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            {error || "请返回个人任务后重试。"}
          </p>
        </div>
        <div className="flex flex-wrap justify-center gap-2">
          {!notFound && (
            <Button variant="outline" onClick={() => setReloadKey((value) => value + 1)}>
              <RefreshCwIcon /> 重试
            </Button>
          )}
          <Button asChild variant="outline">
            <Link href="/work/inbox">
              <ArrowLeftIcon /> 返回个人任务
            </Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col gap-4 p-4 lg:p-6">
      <section className="overflow-hidden rounded-2xl border bg-gradient-to-br from-blue-500/[0.08] via-violet-500/[0.06] to-rose-400/[0.08]">
        <div className="flex flex-col gap-4 p-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Button asChild size="sm" variant="ghost" className="min-h-10 sm:min-h-8">
                <Link href={workTaskHref}>
                  <ArrowLeftIcon /> 返回个人任务
                </Link>
              </Button>
              <Badge variant="outline" className={statusTone(task.automation_status)}>
                {automationStatusLabel(task.automation_status)}
              </Badge>
              {task.artex_task?.paused && <Badge variant="outline">已暂停</Badge>}
            </div>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight">{task.title}</h1>
            <p className="mt-2 max-w-3xl whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">
              {task.description || "未填写任务说明"}
            </p>
            {task.parent_task_id != null && (
              <div className="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-blue-500/20 bg-blue-500/[0.06] px-3 py-2 text-sm">
                <span className="font-medium">目标步骤 · 随父任务执行</span>
                <Button asChild size="sm" variant="link" className="h-auto p-0">
                  <Link href={`/work/inbox?task=${encodeURIComponent(String(task.parent_task_id))}`}>
                    返回父任务查看执行
                  </Link>
                </Button>
              </div>
            )}
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            {canRun && (
              <Button onClick={() => void run()} disabled={busy !== null}>
                {busy === "run" ? <Loader2Icon className="animate-spin" /> : task.automation_status === "failed" ? <RotateCwIcon /> : <PlayIcon />}
                {task.automation_status === "failed" ? "重试执行" : "立即执行"}
              </Button>
            )}
            {canCancel && (
              <Button variant="outline" onClick={() => void cancel()} disabled={busy !== null}>
                {busy === "cancel" ? <Loader2Icon className="animate-spin" /> : <XCircleIcon />}
                取消执行
              </Button>
            )}
            <Button variant="outline" onClick={() => void load()} disabled={loading}>
              <RefreshCwIcon className={loading ? "animate-spin" : ""} />
              刷新状态
            </Button>
          </div>
        </div>
      </section>

      {error && task && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700">
          <span>状态刷新失败，当前展示上一次成功读取的数据：{error}</span>
          <Button size="sm" variant="outline" className="min-h-10 sm:min-h-7" onClick={() => void load()}>
            重试
          </Button>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <BotIcon className="size-4 text-primary" /> 多 Agent 执行过程
              </CardTitle>
            </CardHeader>
            <CardContent>
              {linkedArtexID ? (
                <WorkTaskActivity
                  taskId={linkedArtexID}
                  live={task.automation_status === "running"}
                  defaultOpen
                />
              ) : linkedArtexDeleted ? (
                <div className="rounded-xl border border-amber-500/25 bg-amber-500/[0.07] px-4 py-6 text-center">
                  <AlertTriangle className="mx-auto size-5 text-amber-600" />
                  <p className="mt-2 text-sm font-medium">关联执行任务已删除</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    实时活动无法恢复；下方保留的个人任务最终结果仍然可用。
                  </p>
                </div>
              ) : (
                <div className="rounded-xl border border-dashed px-4 py-8 text-center">
                  <Loader2Icon className="mx-auto size-5 animate-spin text-primary" />
                  <p className="mt-2 text-sm font-medium">正在创建多 Agent 执行任务</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    调度器完成关联后会显示 goals、planner 与 worker 的实时活动。
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          {(task.automation_status === "done" || task.automation_status === "failed") && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  {task.automation_status === "failed" ? (
                    <AlertTriangle className="size-4 text-destructive" />
                  ) : (
                    <CheckCircle2Icon className="size-4 text-emerald-600" />
                  )}
                  {task.automation_status === "failed" ? "失败前结果" : "最终结果"}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="whitespace-pre-wrap break-words rounded-xl border bg-muted/30 p-4 text-sm leading-7">
                  {finalResult || "本次执行没有留下可展示的文本结果，请查看上方多 Agent 执行过程与错误信息。"}
                </div>
              </CardContent>
            </Card>
          )}
        </div>

        <aside className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">任务信息</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <InfoRow label="项目" value={task.project_name || "未归属项目"} />
              <InfoRow label="优先级" value={priorityLabel(task.priority)} />
              <InfoRow label="开始时间" value={formatDate(task.start_at)} />
              <InfoRow label="截止时间" value={formatDate(task.due_at)} />
              <InfoRow label="启动执行" value={formatDate(task.automation_launched_at)} />
              <InfoRow label="结束执行" value={formatDate(task.automation_finished_at)} />
              {task.tags.length > 0 && (
                <div>
                  <p className="text-xs text-muted-foreground">标签</p>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {task.tags.map((tag) => (
                      <Badge key={tag.id} variant="outline">
                        {tag.name}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          {(task.automation_error || task.artex_task_id) && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">关联信息</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {task.automation_error && (
                  <p className="whitespace-pre-wrap break-words rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                    {task.automation_error}
                  </p>
                )}
                {task.artex_task_id && (
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <p className="flex items-center gap-2 text-sm font-medium">
                      <Clock3Icon className="size-4 text-primary" /> 执行任务 #{task.artex_task_id}
                    </p>
                    {task.artex_task?.status && (
                      <p className="mt-1 text-xs text-muted-foreground">运行状态：{task.artex_task.status}</p>
                    )}
                    {linkedArtexDeleted && (
                      <p className="mt-1 text-xs text-amber-700">执行任务已删除，最终结果已保留</p>
                    )}
                    <Button asChild variant="ghost" size="sm" className="mt-1 min-h-10 px-0 sm:min-h-8">
                      <Link href={workTaskHref}>
                        <ExternalLinkIcon /> 打开个人任务详情
                      </Link>
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </aside>
      </div>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="text-right">{value}</span>
    </div>
  );
}

function priorityLabel(priority: WorkTask["priority"]) {
  return {
    low: "低",
    medium: "中",
    high: "高",
    urgent: "紧急",
  }[priority];
}
