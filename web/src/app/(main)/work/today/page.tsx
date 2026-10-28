"use client";

import * as React from "react";
import { CalendarClock, CheckCircle2, Plus, RefreshCw, Sun, Trash2 } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { NotificationChannel, WorkProject, WorkTag, WorkTask } from "@/lib/types";
import { WORK_CHANGED_EVENT, emitWorkChanged } from "@/lib/work-events";
import { deleteWorkTaskSafely, workTaskIsAutomating } from "@/lib/work-task-actions";

import { TaskDetailDialog, type WorkTaskDraft } from "../_components/task-detail-dialog";
import { WorkAutomationBadge } from "../_components/work-automation";
import { WorkEmptyState, WorkErrorState, WorkPageHeader } from "../_components/work-shell";

type TaskGroup = "today" | "overdue" | "upcoming";

export default function WorkTodayPage() {
  const [today, setToday] = React.useState<WorkTask[]>([]);
  const [overdue, setOverdue] = React.useState<WorkTask[]>([]);
  const [upcoming, setUpcoming] = React.useState<WorkTask[]>([]);
  const [recentTasks, setRecentTasks] = React.useState<WorkTask[]>([]);
  const [projects, setProjects] = React.useState<WorkProject[]>([]);
  const [tags, setTags] = React.useState<WorkTag[]>([]);
  const [channels, setChannels] = React.useState<NotificationChannel[]>([]);
  const [detailTask, setDetailTask] = React.useState<WorkTask | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [createDraft, setCreateDraft] = React.useState<WorkTaskDraft>();
  const [deleteTarget, setDeleteTarget] = React.useState<WorkTask | null>(null);
  const [deleting, setDeleting] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [optionsLoading, setOptionsLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [optionsError, setOptionsError] = React.useState<string>();
  const requestSequence = React.useRef(0);
  const recentRequestSequence = React.useRef(0);

  const loadTasks = React.useCallback(async (quiet = false) => {
    const requestID = ++requestSequence.current;
    if (!quiet) setLoading(true);
    try {
      const [todayTasks, overdueTasks, upcomingTasks] = await Promise.all([
        api.workTasks({ view: "today" }),
        api.workTasks({ view: "overdue" }),
        api.workTasks({ view: "upcoming" }),
      ]);
      if (requestID !== requestSequence.current) return;
      setToday(todayTasks);
      setOverdue(overdueTasks);
      setUpcoming(upcomingTasks.slice(0, 8));
      setError(undefined);
    } catch (e) {
      if (requestID === requestSequence.current) setError((e as Error).message);
    } finally {
      if (requestID === requestSequence.current) setLoading(false);
    }
  }, []);

  const loadRecent = React.useCallback(async () => {
    const requestID = ++recentRequestSequence.current;
    try {
      const overview = await api.workAutomationOverview();
      if (requestID !== recentRequestSequence.current) return;
      setRecentTasks(
        overview.recent
          .filter((task) => task.automation_status === "done" || task.automation_status === "failed")
          .slice(0, 5),
      );
    } catch {
      // The main today view remains usable if this secondary summary fails.
    }
  }, []);

  const loadOptions = React.useCallback(async () => {
    setOptionsLoading(true);
    const [projectResult, tagResult, channelResult] = await Promise.allSettled([
      api.workProjects(),
      api.workTags(),
      api.notifyChannels(),
    ]);
    if (projectResult.status === "fulfilled") setProjects(projectResult.value);
    if (tagResult.status === "fulfilled") setTags(tagResult.value);
    if (channelResult.status === "fulfilled") setChannels(channelResult.value);
    const failures = [projectResult, tagResult, channelResult].filter((result) => result.status === "rejected");
    setOptionsError(failures.length > 0 ? "项目、标签或通知渠道加载失败，任务仍可正常查看和编辑。" : undefined);
    setOptionsLoading(false);
  }, []);

  React.useEffect(() => {
    void loadTasks();
    void loadRecent();
    void loadOptions();
  }, [loadOptions, loadRecent, loadTasks]);

  React.useEffect(() => {
    const refreshWhenVisible = () => {
      if (document.visibilityState !== "visible") return;
      void loadTasks(true);
      void loadRecent();
      void loadOptions();
    };
    const refreshAfterChange = () => {
      void loadTasks(true);
      void loadRecent();
    };
    document.addEventListener("visibilitychange", refreshWhenVisible);
    window.addEventListener(WORK_CHANGED_EVENT, refreshAfterChange);
    return () => {
      document.removeEventListener("visibilitychange", refreshWhenVisible);
      window.removeEventListener(WORK_CHANGED_EVENT, refreshAfterChange);
    };
  }, [loadOptions, loadRecent, loadTasks]);

  function openCreate() {
    const due = new Date();
    due.setHours(18, 0, 0, 0);
    setCreateDraft({ due_at: due.toISOString(), tag_ids: [] });
    setCreateOpen(true);
  }

  async function complete(task: WorkTask, group: TaskGroup) {
    const setter = group === "today" ? setToday : group === "overdue" ? setOverdue : setUpcoming;
    const previous = group === "today" ? today : group === "overdue" ? overdue : upcoming;
    setter((items) => items.filter((item) => item.id !== task.id));
    try {
      await api.completeWorkTask(task.id);
      toast.success("任务已完成");
      emitWorkChanged();
    } catch (e) {
      setter(previous);
      setError((e as Error).message);
      toast.error("完成任务失败", { description: (e as Error).message });
    }
  }

  async function removeTask() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteWorkTaskSafely(deleteTarget);
      toast.success("任务已删除");
      setDeleteTarget(null);
      setDetailTask((current) => (current?.id === deleteTarget.id ? null : current));
      emitWorkChanged();
    } catch (e) {
      toast.error("删除失败", { description: (e as Error).message });
    } finally {
      setDeleting(false);
    }
  }

  const groups = [
    {
      title: "逾期",
      icon: CalendarClock,
      tasks: overdue,
      key: "overdue" as const,
      tone: "text-red-700 dark:text-red-400",
    },
    {
      title: "今日",
      icon: Sun,
      tasks: today,
      key: "today" as const,
      tone: "text-amber-700 dark:text-amber-400",
    },
    {
      title: "即将到期",
      icon: CheckCircle2,
      tasks: upcoming,
      key: "upcoming" as const,
      tone: "text-blue-700 dark:text-blue-400",
    },
  ];
  const taskCount = today.length + overdue.length + upcoming.length;

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <WorkPageHeader title="今日" description="按逾期、今日、即将到期组织今天的个人工作。">
        <Button size="sm" className="min-h-10 sm:min-h-7" onClick={openCreate}>
          <Plus className="size-4" />
          新建任务
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="min-h-10 sm:min-h-7"
          onClick={() => void loadTasks()}
          disabled={loading}
        >
          <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
          刷新
        </Button>
      </WorkPageHeader>

      {optionsError && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
          {optionsError}
        </div>
      )}

      {recentTasks.length > 0 && (
        <Card size="sm">
          <CardContent className="space-y-1.5">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">最近自动结果</p>
              <span className="text-xs text-muted-foreground">快速完成的任务仍会保留在这里</span>
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {recentTasks.map((task) => (
                <button
                  key={task.id}
                  type="button"
                  className="flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2 text-left hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  onClick={() => setDetailTask(task)}
                >
                  <span
                    className={`size-2 shrink-0 rounded-full ${
                      task.automation_status === "failed" ? "bg-red-500" : "bg-emerald-500"
                    }`}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{task.title}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {task.automation_error || (task.automation_status === "failed" ? "执行失败" : "自动执行已完成")}
                    </span>
                  </span>
                  <WorkAutomationBadge task={task} />
                </button>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {loading && taskCount === 0 ? (
        <div className="space-y-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      ) : error ? (
        <WorkErrorState message={error} onRetry={() => void loadTasks()} />
      ) : taskCount === 0 ? (
        <WorkEmptyState title="今天没有待办" description="直接在当前页面新建任务，或到收件箱收集想法。">
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新建今日任务
          </Button>
        </WorkEmptyState>
      ) : (
        <div className="space-y-5">
          {groups.map((group) => (
            <section key={group.key} className="space-y-2">
              <div className={`flex items-center gap-2 text-sm font-semibold ${group.tone}`}>
                <group.icon className="size-4" />
                {group.title}
                <span className="text-xs font-normal text-muted-foreground">{group.tasks.length}</span>
              </div>
              {group.tasks.length === 0 ? (
                <p className="rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground">
                  暂无{group.title}任务
                </p>
              ) : (
                group.tasks.map((task) => (
                  <Card key={task.id} size="sm">
                    <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                      <button
                        type="button"
                        className="min-w-0 flex-1 text-left"
                        onClick={() => setDetailTask(task)}
                        aria-label={`打开任务 ${task.title}`}
                      >
                        <p className="truncate text-sm font-medium">{task.title}</p>
                        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <Badge variant="outline">{task.priority}</Badge>
                          {task.project_name && <span>{task.project_name}</span>}
                          {task.due_at && <span>{new Date(task.due_at).toLocaleString()}</span>}
                          <WorkAutomationBadge task={task} />
                        </div>
                      </button>
                      <div className="flex items-center gap-1 self-end sm:self-auto">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="min-h-10 sm:min-h-7"
                          onClick={() => setDetailTask(task)}
                        >
                          详情
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="min-h-10 sm:min-h-7"
                          onClick={() => void complete(task, group.key)}
                        >
                          完成
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="size-10 sm:size-7"
                          onClick={() => setDeleteTarget(task)}
                          aria-label={`删除任务 ${task.title}`}
                        >
                          <Trash2 className="size-4 text-destructive" />
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                ))
              )}
            </section>
          ))}
        </div>
      )}

      <TaskDetailDialog
        task={detailTask}
        projects={projects}
        tags={tags}
        channels={channels}
        onClose={() => setDetailTask(null)}
        onSaved={(saved, options) => {
          if (options?.keepOpen) {
            setDetailTask((current) => (current?.id === saved.id ? saved : current));
          } else {
            setDetailTask(null);
          }
          emitWorkChanged();
        }}
        onDeleted={() => {
          setDetailTask(null);
          emitWorkChanged();
        }}
        onSubtasksChange={() => emitWorkChanged()}
      />

      <TaskDetailDialog
        task={null}
        createMode={createOpen}
        draft={createDraft}
        projects={projects}
        tags={tags}
        channels={channels}
        onClose={() => setCreateOpen(false)}
        onSaved={() => {
          setCreateOpen(false);
          toast.success("任务已创建并加入自动化队列");
          emitWorkChanged();
        }}
        onSubtasksChange={() => undefined}
      />

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除「{deleteTarget?.title || "未命名任务"}」？</AlertDialogTitle>
            <AlertDialogDescription>
              此操作不可撤销，并会永久删除该任务的全部子任务和提醒。
              {deleteTarget && workTaskIsAutomating(deleteTarget)
                ? " 任务正在自动执行，确认后会先取消关联执行，取消失败时不会删除任务。"
                : ""}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleting}
              onClick={(event) => {
                event.preventDefault();
                void removeTask();
              }}
            >
              {deleting ? "删除中" : "确认删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {optionsLoading && (
        <span className="sr-only" role="status">
          正在加载项目、标签和提醒渠道
        </span>
      )}
    </div>
  );
}
