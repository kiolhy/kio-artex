"use client";

import * as React from "react";
import { Inbox, Loader2, Plus, RefreshCw, Trash2 } from "lucide-react";
import Link from "next/link";
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
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { NotificationChannel, WorkProject, WorkTag, WorkTask } from "@/lib/types";
import { cn } from "@/lib/utils";
import { WORK_CHANGED_EVENT, emitWorkChanged } from "@/lib/work-events";
import { deleteWorkTaskSafely, workTaskIsAutomating } from "@/lib/work-task-actions";
import { parseWorkQuickCapture } from "@/lib/work-quick-capture";

import { TaskDetailDialog, type WorkTaskDraft } from "../_components/task-detail-dialog";
import { WorkAutomationBadge } from "../_components/work-automation";
import { WorkEmptyState, WorkErrorState, WorkPageHeader } from "../_components/work-shell";

const priorityClass: Record<WorkTask["priority"], string> = {
  urgent: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-400",
  high: "border-orange-500/30 bg-orange-500/10 text-orange-700 dark:text-orange-400",
  medium: "border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-400",
  low: "border-muted-foreground/30 bg-muted text-muted-foreground",
};

export default function WorkInboxPage() {
  const [tasks, setTasks] = React.useState<WorkTask[]>([]);
  const [recentTasks, setRecentTasks] = React.useState<WorkTask[]>([]);
  const [projects, setProjects] = React.useState<WorkProject[]>([]);
  const [tags, setTags] = React.useState<WorkTag[]>([]);
  const [channels, setChannels] = React.useState<NotificationChannel[]>([]);
  const [selected, setSelected] = React.useState<number[]>([]);
  const [detailTask, setDetailTask] = React.useState<WorkTask | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [createDraft, setCreateDraft] = React.useState<WorkTaskDraft>();
  const [deleteTargets, setDeleteTargets] = React.useState<WorkTask[]>([]);
  const [query, setQuery] = React.useState("");
  const [debouncedQuery, setDebouncedQuery] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const [optionsLoading, setOptionsLoading] = React.useState(true);
  const [creating, setCreating] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);
  const [error, setError] = React.useState<string>();
  const [optionsError, setOptionsError] = React.useState<string>();
  const [title, setTitle] = React.useState("");
  const [projectId, setProjectId] = React.useState("");
  const requestSequence = React.useRef(0);
  const recentRequestSequence = React.useRef(0);

  React.useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [query]);

  const loadTasks = React.useCallback(async (quiet = false) => {
    const requestID = ++requestSequence.current;
    if (!quiet) setLoading(true);
    try {
      const list = await api.workTasks({ q: debouncedQuery, status: "todo,doing" });
      if (requestID !== requestSequence.current) return;
      setTasks(list);
      setSelected((ids) => ids.filter((id) => list.some((task) => task.id === id)));
      setError(undefined);
    } catch (e) {
      if (requestID === requestSequence.current) setError((e as Error).message);
    } finally {
      if (requestID === requestSequence.current) setLoading(false);
    }
  }, [debouncedQuery]);

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
      // The active list remains usable when the recent-result summary is unavailable.
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
    setOptionsError(failures.length > 0 ? "部分项目、标签或通知渠道加载失败，任务列表仍可使用。" : undefined);
    setOptionsLoading(false);
  }, []);

  React.useEffect(() => {
    void loadTasks();
    void loadRecent();
  }, [loadRecent, loadTasks]);

  React.useEffect(() => {
    void loadOptions();
  }, [loadOptions]);

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

  React.useEffect(() => {
    const id = Number(new URLSearchParams(window.location.search).get("task"));
    if (!Number.isSafeInteger(id) || id <= 0) return;
    let alive = true;
    void api
      .workTask(id)
      .then((task) => {
        if (alive) setDetailTask(task);
      })
      .catch((cause) => {
        if (alive) toast.error("无法打开指定任务", { description: (cause as Error).message });
      });
    return () => {
      alive = false;
    };
  }, []);

  function openCreate(withProject = false) {
    setCreateDraft({ project_id: withProject && projectId ? Number(projectId) : null, tag_ids: [] });
    setCreateOpen(true);
  }

  async function createTask(event: React.FormEvent) {
    event.preventDefault();
    if (!title.trim()) return;
    setCreating(true);
    try {
      const input = parseWorkQuickCapture(title.trim(), projects);
      const created = await api.createWorkTask({
        ...input,
        project_id: projectId ? Number(projectId) : (input.project_id ?? null),
      });
      requestSequence.current += 1;
      setTasks((items) => [created, ...items.filter((item) => item.id !== created.id)]);
      setTitle("");
      toast.success("待办已加入自动化队列");
      emitWorkChanged();
      void loadTasks(true);
      void loadRecent();
    } catch (e) {
      setError((e as Error).message);
      toast.error("创建失败", { description: (e as Error).message });
    } finally {
      setCreating(false);
    }
  }

  async function batchUpdate(patch: Parameters<typeof api.updateWorkTask>[1]) {
    const previous = tasks;
    setTasks((items) => items.map((item) => (selected.includes(item.id) ? { ...item, ...patch } : item)));
    try {
      await Promise.all(selected.map((id) => api.updateWorkTask(id, patch)));
      setSelected([]);
      emitWorkChanged();
      toast.success("任务已更新");
    } catch (e) {
      setTasks(previous);
      setError((e as Error).message);
      toast.error("批量更新失败", { description: (e as Error).message });
    }
  }

  function toggleSelected(id: number) {
    setSelected((ids) => (ids.includes(id) ? ids.filter((item) => item !== id) : [...ids, id]));
  }

  async function complete(id: number) {
    const previous = tasks;
    setTasks((items) => items.filter((task) => task.id !== id));
    try {
      await api.completeWorkTask(id);
      emitWorkChanged();
      toast.success("任务已完成");
    } catch (e) {
      setTasks(previous);
      setError((e as Error).message);
      toast.error("完成任务失败", { description: (e as Error).message });
    }
  }

  async function removeTargets() {
    if (deleteTargets.length === 0) return;
    setDeleting(true);
    try {
      for (const task of deleteTargets) {
        await deleteWorkTaskSafely(task);
      }
      const deletedIDs = new Set(deleteTargets.map((task) => task.id));
      setTasks((items) => items.filter((task) => !deletedIDs.has(task.id)));
      setSelected((ids) => ids.filter((id) => !deletedIDs.has(id)));
      setDetailTask((current) => (current && deletedIDs.has(current.id) ? null : current));
      setDeleteTargets([]);
      toast.success(deleteTargets.length === 1 ? "任务已删除" : `已删除 ${deleteTargets.length} 个任务`);
      emitWorkChanged();
    } catch (e) {
      toast.error("删除失败", { description: (e as Error).message });
      emitWorkChanged();
    } finally {
      setDeleting(false);
    }
  }

  const selectedTasks = tasks.filter((task) => selected.includes(task.id));

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <WorkPageHeader title="收件箱" description="所有未完成的个人待办，支持快速收集、编辑和删除。">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="搜索待办"
          className="w-full sm:w-64"
          aria-label="搜索待办"
        />
        <Button size="sm" className="min-h-10 sm:min-h-7" onClick={() => openCreate()}>
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

      <Card>
        <CardHeader className="pb-0">
          <p className="text-sm font-medium">快速收集</p>
          <p className="text-xs text-muted-foreground">输入一行内容，回车即可创建待办。</p>
        </CardHeader>
        <CardContent>
          <form onSubmit={createTask} className="flex flex-col gap-2 sm:flex-row">
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="例如：整理本周项目资料"
              aria-label="快速收集待办内容"
              className="min-h-10"
            />
            <select
              value={projectId}
              onChange={(e) => setProjectId(e.target.value)}
              className="h-10 rounded-lg border border-input bg-background px-3 text-sm"
              aria-label="任务项目"
            >
              <option value="">不归属项目</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
            <Button
              type="submit"
              className="min-h-10"
              disabled={!title.trim() || creating}
            >
              {creating && <Loader2 className="size-4 animate-spin" />}
              添加
            </Button>
            <Button
              type="button"
              variant="outline"
              className="min-h-10"
              onClick={() => openCreate(true)}
            >
              详细设置
            </Button>
          </form>
          <p className="text-xs text-muted-foreground">
            支持 /高 /紧急 /低 指定优先级，或以“明天 9:00”开头设置截止时间。
          </p>
        </CardContent>
      </Card>

      {selected.length > 0 && (
        <Card size="sm">
          <CardContent className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium">已选择 {selected.length} 项</span>
            <Button
              size="sm"
              className="min-h-10 sm:min-h-7"
              variant="outline"
              onClick={() => void batchUpdate({ status: "doing" })}
            >
              标记进行中
            </Button>
            <Button
              size="sm"
              className="min-h-10 sm:min-h-7"
              variant="outline"
              onClick={() => void batchUpdate({ priority: "high" })}
            >
              设为高优先级
            </Button>
            <Button
              size="sm"
              className="min-h-10 sm:min-h-7"
              variant="destructive"
              onClick={() => setDeleteTargets(selectedTasks)}
            >
              <Trash2 className="size-4" />
              删除所选
            </Button>
            <Button
              size="sm"
              className="min-h-10 sm:min-h-7"
              variant="ghost"
              onClick={() => setSelected([])}
            >
              取消选择
            </Button>
          </CardContent>
        </Card>
      )}

      {recentTasks.length > 0 && (
        <Card size="sm">
          <CardHeader className="pb-2">
            <div className="flex items-center justify-between gap-2">
              <div>
                <p className="text-sm font-medium">最近自动结果</p>
                <p className="text-xs text-muted-foreground">任务执行较快时可能已离开未完成列表，仍可在这里找到。</p>
              </div>
              <Button variant="ghost" size="xs" asChild>
                <Link href="/dashboard/">查看自动化卡片</Link>
              </Button>
            </div>
          </CardHeader>
          <CardContent className="grid gap-2 sm:grid-cols-2">
            {recentTasks.map((task) => (
              <button
                key={task.id}
                type="button"
                className="flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2 text-left hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => setDetailTask(task)}
              >
                <span
                  className={cn(
                    "size-2 shrink-0 rounded-full",
                    task.automation_status === "failed" ? "bg-red-500" : "bg-emerald-500",
                  )}
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
          </CardContent>
        </Card>
      )}

      {loading && tasks.length === 0 ? (
        <div className="space-y-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : error ? (
        <WorkErrorState message={error} onRetry={() => void loadTasks()} />
      ) : tasks.length === 0 ? (
        <WorkEmptyState
          title={debouncedQuery ? "没有匹配的待办" : "收件箱是空的"}
          description={debouncedQuery ? "尝试更换关键词。" : "从上方快速收集一条待办开始。"}
        >
          <Button size="sm" onClick={() => openCreate()}>
            <Plus className="size-4" />
            新建任务
          </Button>
        </WorkEmptyState>
      ) : (
        <div className="space-y-2">
          {tasks.map((task) => (
            <Card key={task.id} size="sm">
              <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <Checkbox
                  checked={selected.includes(task.id)}
                  onCheckedChange={() => toggleSelected(task.id)}
                  aria-label={`选择 ${task.title}`}
                />
                <button
                  type="button"
                  className="min-w-0 flex-1 text-left"
                  onClick={() => setDetailTask(task)}
                  aria-label={`打开任务 ${task.title}`}
                >
                  <p className="truncate text-sm font-medium">{task.title}</p>
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <Badge variant="outline" className={priorityClass[task.priority]}>
                      {task.priority}
                    </Badge>
                    {task.project_name && <span>{task.project_name}</span>}
                    {task.due_at && <span>截止 {new Date(task.due_at).toLocaleString()}</span>}
                    {task.tags.map((tag) => (
                      <Badge key={tag.id} variant="secondary">
                        {tag.name}
                      </Badge>
                    ))}
                    <WorkAutomationBadge task={task} />
                  </div>
                </button>
                <div className="flex items-center gap-1 self-end sm:self-auto">
                  <Button
                    variant="outline"
                    size="sm"
                    className="min-h-10 sm:min-h-7"
                    onClick={() => void complete(task.id)}
                  >
                    完成
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="min-h-10 sm:min-h-7"
                    onClick={() => setDetailTask(task)}
                  >
                    详情
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="size-10 sm:size-7"
                    onClick={() => setDeleteTargets([task])}
                    aria-label={`删除任务 ${task.title}`}
                  >
                    <Trash2 className="size-4 text-destructive" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <div className={cn("flex items-center gap-2 text-xs text-muted-foreground")}>
        <Inbox className="size-3.5" /> 共 {tasks.length} 条未完成待办
        {debouncedQuery && query !== debouncedQuery ? " · 正在搜索…" : ""}
      </div>

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

      <AlertDialog open={deleteTargets.length > 0} onOpenChange={(open) => !open && setDeleteTargets([])}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {deleteTargets.length === 1 ? `删除「${deleteTargets[0]?.title || "未命名任务"}」？` : `删除所选 ${deleteTargets.length} 个任务？`}
            </AlertDialogTitle>
            <AlertDialogDescription>
              此操作不可撤销，并会永久删除这些任务的全部子任务和提醒。
              {deleteTargets.some(workTaskIsAutomating)
                ? " 其中包含自动执行中的任务，确认后会先取消关联执行，任一取消失败时停止后续删除。"
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
                void removeTargets();
              }}
            >
              {deleting && <Loader2 className="size-4 animate-spin" />}
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
