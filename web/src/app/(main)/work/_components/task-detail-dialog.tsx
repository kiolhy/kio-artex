"use client";

import * as React from "react";

import {
  AlertTriangle,
  CheckCircle2,
  ExternalLink,
  Loader2,
  Play,
  Plus,
  RotateCw,
  Trash2,
  XCircle,
} from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { NotificationChannel, WorkProject, WorkTask } from "@/lib/types";
import { cn } from "@/lib/utils";
import { deleteWorkTaskSafely, workTaskIsAutomating } from "@/lib/work-task-actions";
import { emitWorkChanged } from "@/lib/work-events";
import { automationStatusLabel } from "./work-automation";
import { WorkTaskActivity } from "./work-task-activity";
import { WorkTaskAssetsPanel } from "./work-task-assets";

type ReminderForm = {
  dueEnabled: boolean;
  dueMinutes: number;
  overdueEnabled: boolean;
  dueChannelIDs: number[];
  overdueChannelIDs: number[];
};

const emptyReminders: ReminderForm = {
  dueEnabled: true,
  dueMinutes: 10,
  overdueEnabled: true,
  dueChannelIDs: [],
  overdueChannelIDs: [],
};

export type WorkTaskDraft = {
  title?: string;
  description?: string;
  priority?: WorkTask["priority"];
  project_id?: number | null;
  start_at?: string | null;
  due_at?: string | null;
  repeat_rule?: WorkTask["repeat_rule"];
  tag_ids?: number[];
  pinned_asset_ids?: number[];
  excluded_asset_ids?: number[];
};

export function TaskDetailDialog({
  task,
  createMode = false,
  draft,
  projects,
  tags,
  channels,
  onClose,
  onSaved,
  onDeleted,
  onSubtasksChange,
}: {
  task: WorkTask | null;
  createMode?: boolean;
  draft?: WorkTaskDraft;
  projects: WorkProject[];
  tags: { id: number; name: string; color: string }[];
  channels: NotificationChannel[];
  onClose: () => void;
  onSaved: (task: WorkTask, options?: { keepOpen?: boolean }) => void;
  onDeleted?: (task: WorkTask) => void;
  onSubtasksChange: () => void;
}) {
  const [title, setTitle] = React.useState("");
  const [description, setDescription] = React.useState("");
  const [status, setStatus] = React.useState<WorkTask["status"]>("todo");
  const [priority, setPriority] = React.useState<WorkTask["priority"]>("medium");
  const [projectID, setProjectID] = React.useState("");
  const [repeatRule, setRepeatRule] = React.useState<WorkTask["repeat_rule"]>("");
  const [startAt, setStartAt] = React.useState("");
  const [dueAt, setDueAt] = React.useState("");
  const [selectedTags, setSelectedTags] = React.useState<number[]>([]);
  const [assetSelection, setAssetSelection] = React.useState<{
    pinned: number[];
    excluded: number[];
  }>({ pinned: [], excluded: [] });
  const [subtasks, setSubtasks] = React.useState<WorkTask[]>([]);
  const [subtaskTitle, setSubtaskTitle] = React.useState("");
  const [reminders, setReminders] = React.useState<ReminderForm>(emptyReminders);
  const [remindersLoading, setRemindersLoading] = React.useState(false);
  const [reminderLoadError, setReminderLoadError] = React.useState<string>();
  const [reminderReloadKey, setReminderReloadKey] = React.useState(0);
  const [automationBusy, setAutomationBusy] = React.useState(false);
  const [automation, setAutomation] = React.useState<WorkTask | null>(null);
  const [watchAutomation, setWatchAutomation] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [loadingDetail, setLoadingDetail] = React.useState(false);
  const [error, setError] = React.useState<string>();

  React.useEffect(() => {
    if (!task && !createMode) return;
    if (!task && createMode) {
      setTitle(draft?.title ?? "");
      setDescription(draft?.description ?? "");
      setStatus("todo");
      setPriority(draft?.priority ?? "medium");
      setProjectID(draft?.project_id ? String(draft.project_id) : "");
      setRepeatRule(draft?.repeat_rule ?? "");
      setStartAt(draft?.start_at ? toLocalInput(new Date(draft.start_at)) : "");
      setDueAt(draft?.due_at ? toLocalInput(new Date(draft.due_at)) : "");
      setSelectedTags(draft?.tag_ids ?? []);
      setAssetSelection({
        pinned: draft?.pinned_asset_ids ?? [],
        excluded: draft?.excluded_asset_ids ?? [],
      });
      setSubtasks([]);
      setReminders({ ...emptyReminders });
      setRemindersLoading(false);
      setReminderLoadError(undefined);
      setAutomation(null);
      setWatchAutomation(false);
      setError(undefined);
      setLoadingDetail(false);
      return;
    }
    if (!task) return;
    setTitle(task.title);
    setDescription(task.description);
    setStatus(task.status);
    setPriority(task.priority);
    setProjectID(task.project_id ? String(task.project_id) : "");
    setRepeatRule(task.repeat_rule);
    setStartAt(task.start_at ? toLocalInput(new Date(task.start_at)) : "");
    setDueAt(task.due_at ? toLocalInput(new Date(task.due_at)) : "");
    setAutomation(task);
    setWatchAutomation(
      task.automation_status === "pending" || task.automation_status === "running",
    );
    setSelectedTags(task.tags.map((tag) => tag.id));
    setAssetSelection({ pinned: [], excluded: [] });
    setError(undefined);
    setLoadingDetail(true);
    let cancelled = false;
    void (async () => {
      try {
        const children = await api.workTasks({ parent_task_id: String(task.id) });
        if (cancelled) return;
        setSubtasks(children);
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      } finally {
        if (!cancelled) setLoadingDetail(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [createMode, draft, task]);

  React.useEffect(() => {
    if (createMode || !task || !watchAutomation) return;
    let cancelled = false;
    const timer = window.setInterval(() => {
      void api
        .workTask(task.id)
        .then((fresh) => {
          if (cancelled) return;
          setAutomation(fresh);
          if (["done", "failed", "canceled"].includes(fresh.automation_status)) {
            setWatchAutomation(false);
            emitWorkChanged();
          }
        })
        .catch((cause: Error) => {
          if (!cancelled) setError(`自动执行状态同步失败：${cause.message}`);
        });
    }, 3000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [createMode, task, watchAutomation]);

  React.useEffect(() => {
    if (createMode || !task) {
      setRemindersLoading(false);
      setReminderLoadError(undefined);
      return;
    }
    let cancelled = false;
    setRemindersLoading(true);
    setReminderLoadError(undefined);
    void api
      .workTaskReminders(task.id)
      .then((reminderRows) => {
        if (cancelled) return;
        const form = { ...emptyReminders };
        for (const reminder of reminderRows) {
          if (reminder.kind === "due") {
            form.dueEnabled = true;
            form.dueMinutes = reminder.minutes || 10;
            form.dueChannelIDs = reminder.channel_ids;
          } else if (reminder.kind === "overdue") {
            form.overdueEnabled = true;
            form.overdueChannelIDs = reminder.channel_ids;
          }
        }
        setReminders(form);
      })
      .catch((e: Error) => {
        if (!cancelled) setReminderLoadError(e.message || "未知错误");
      })
      .finally(() => {
        if (!cancelled) setRemindersLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [createMode, reminderReloadKey, task]);

  async function runAutomation() {
    if (!task || task.parent_task_id != null) return;
    setAutomationBusy(true);
    try {
      const updated = await api.runWorkTask(task.id);
      setAutomation(updated);
      setWatchAutomation(true);
      emitWorkChanged();
      onSaved(updated, { keepOpen: true });
      toast.success("已启动自动执行");
    } catch (e) {
      setError((e as Error).message);
      toast.error(`启动失败：${(e as Error).message}`);
    } finally {
      setAutomationBusy(false);
    }
  }

  async function cancelAutomation() {
    if (!task || task.parent_task_id != null) return;
    setAutomationBusy(true);
    try {
      const updated = await api.cancelWorkTaskAutomation(task.id);
      setAutomation(updated);
      setWatchAutomation(false);
      emitWorkChanged();
      onSaved(updated, { keepOpen: true });
      toast.success("已取消自动执行");
    } catch (e) {
      setError((e as Error).message);
      toast.error(`取消失败：${(e as Error).message}`);
    } finally {
      setAutomationBusy(false);
    }
  }

  async function save() {
    if ((!task && !createMode) || !title.trim()) return;
    setSaving(true);
    setError(undefined);
    try {
      const completing = Boolean(task && status === "done" && task.status !== "done");
      const input = {
        title: title.trim(),
        description,
        // Completion owns repeat-instance generation, so route the transition
        // through the dedicated endpoint after saving the latest configuration.
        status: completing ? undefined : status,
        priority,
        project_id: projectID ? Number(projectID) : null,
        repeat_rule: repeatRule,
        start_at: startAt ? new Date(startAt).toISOString() : null,
        due_at: dueAt ? new Date(dueAt).toISOString() : null,
        tag_ids: selectedTags,
        pinned_asset_ids: assetSelection.pinned,
        excluded_asset_ids: assetSelection.excluded,
      };
      const updated = task ? await api.updateWorkTask(task.id, input) : await api.createWorkTask(input);
      emitWorkChanged();
      // A failed reminder read must never be saved back as the default reminder
      // configuration. Keep the successfully saved task fields and let the user
      // retry reminders explicitly.
      if (!createMode && (remindersLoading || reminderLoadError)) {
        onSaved(updated, { keepOpen: true });
        setError("任务已保存，但提醒配置尚未成功读取，因此没有改动原有提醒。请重试读取后再保存提醒。");
        return;
      }
      const reminderInput = [
        ...(reminders.dueEnabled ? [{ kind: "due" as const, minutes: reminders.dueMinutes, enabled: true }] : []),
        ...(reminders.overdueEnabled ? [{ kind: "overdue" as const, minutes: 0, enabled: true }] : []),
      ];
      try {
        await api.replaceWorkTaskReminders(updated.id, reminderInput);
        if (reminders.dueEnabled) {
          await api.replaceWorkTaskReminderChannels(updated.id, "due", reminders.dueChannelIDs);
        }
        if (reminders.overdueEnabled) {
          await api.replaceWorkTaskReminderChannels(updated.id, "overdue", reminders.overdueChannelIDs);
        }
      } catch (e) {
        onSaved(updated, { keepOpen: true });
        setError(`任务已保存，但提醒保存失败：${(e as Error).message}`);
        return;
      }
      onSaved(completing && task ? await api.completeWorkTask(task.id) : updated);
      emitWorkChanged();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function removeTask() {
    if (!task) return;
    setDeleting(true);
    try {
      await deleteWorkTaskSafely(task);
      emitWorkChanged();
      setDeleteOpen(false);
      toast.success("任务已删除");
      onDeleted?.(task);
      onClose();
    } catch (e) {
      setDeleteOpen(false);
      setError((e as Error).message);
      toast.error((e as Error).message);
    } finally {
      setDeleting(false);
    }
  }

  async function addSubtask(event: React.FormEvent) {
    event.preventDefault();
    if (!task || !subtaskTitle.trim()) return;
    const previous = subtasks;
    try {
      const created = await api.createWorkTask({
        title: subtaskTitle.trim(),
        parent_task_id: task.id,
      });
      setSubtasks((items) => [...items, created]);
      setSubtaskTitle("");
      emitWorkChanged();
      onSubtasksChange();
    } catch (e) {
      setSubtasks(previous);
      setError((e as Error).message);
    }
  }

  async function completeSubtask(id: number) {
    const previous = subtasks;
    setSubtasks((items) => items.filter((item) => item.id !== id));
    try {
      await api.completeWorkTask(id);
      emitWorkChanged();
      onSubtasksChange();
    } catch (e) {
      setSubtasks(previous);
      setError((e as Error).message);
    }
  }

  const open = task !== null || createMode;
  const executionAvailable = Boolean(
    automation?.artex_task_id && !automation.artex_task?.deleted,
  );

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[calc(100dvh-1rem)] sm:max-h-[90dvh] sm:max-w-3xl">
        <DialogHeader className="pr-10">
          <DialogTitle>{createMode ? "新建任务" : "任务详情"}</DialogTitle>
          <DialogDescription>
            {createMode ? "创建后会自动加入执行队列；可先设置时间、项目、标签和提醒。" : "编辑任务、子任务、自动执行与提醒配置。"}
          </DialogDescription>
        </DialogHeader>

        {!createMode && executionAvailable && automation?.artex_task_id && (
          <WorkTaskActivity
            taskId={automation.artex_task_id}
            live={automation.automation_status === "running"}
            defaultOpen={["running", "done", "failed"].includes(automation.automation_status)}
          />
        )}

        {loadingDetail ? (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Loader2 className="size-4 animate-spin" /> 正在读取任务数据...
          </div>
        ) : (
          <div className="grid gap-4 lg:grid-cols-2">
            <div className="space-y-3">
              <div className="grid gap-2">
                <Label htmlFor="task-title">标题</Label>
                <Input
                  id="task-title"
                  className="h-11 sm:h-8"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="task-description">描述</Label>
                <Textarea
                  id="task-description"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  className="min-h-24"
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="grid gap-2">
                  <Label htmlFor="task-status">状态</Label>
                  <NativeSelect
                    id="task-status"
                    value={status}
                    onChange={(e) => setStatus(e.target.value as WorkTask["status"])}
                    className="w-full [&>select]:h-11 sm:[&>select]:h-8"
                  >
                    <option value="todo">待办</option>
                    <option value="doing">进行中</option>
                    <option value="done">已完成</option>
                    <option value="canceled">已取消</option>
                  </NativeSelect>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="task-priority">优先级</Label>
                  <NativeSelect
                    id="task-priority"
                    value={priority}
                    onChange={(e) => setPriority(e.target.value as WorkTask["priority"])}
                    className="w-full [&>select]:h-11 sm:[&>select]:h-8"
                  >
                    <option value="low">低</option>
                    <option value="medium">中</option>
                    <option value="high">高</option>
                    <option value="urgent">紧急</option>
                  </NativeSelect>
                </div>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="task-project">项目</Label>
                <NativeSelect
                  id="task-project"
                  value={projectID}
                  onChange={(e) => setProjectID(e.target.value)}
                  className="w-full [&>select]:h-11 sm:[&>select]:h-8"
                >
                  <option value="">不归属项目</option>
                  {projects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.name}
                    </option>
                  ))}
                </NativeSelect>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="grid gap-2">
                  <Label htmlFor="task-start">开始时间</Label>
                  <Input
                    id="task-start"
                    type="datetime-local"
                    className="h-11 sm:h-8"
                    value={startAt}
                    onChange={(e) => setStartAt(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="task-due">截止时间</Label>
                  <Input
                    id="task-due"
                    type="datetime-local"
                    className="h-11 sm:h-8"
                    value={dueAt}
                    onChange={(e) => setDueAt(e.target.value)}
                  />
                </div>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="task-repeat">重复</Label>
                <NativeSelect
                  id="task-repeat"
                  value={repeatRule}
                  onChange={(e) => setRepeatRule(e.target.value as WorkTask["repeat_rule"])}
                  className="w-full [&>select]:h-11 sm:[&>select]:h-8"
                >
                  <option value="">不重复</option>
                  <option value="daily">每天</option>
                  <option value="weekly">每周</option>
                  <option value="monthly">每月</option>
                  <option value="weekdays">工作日</option>
                </NativeSelect>
              </div>
              <div className="grid gap-2">
                <Label>标签</Label>
                <div className="flex flex-wrap gap-2">
                  {tags.map((tag) => {
                    const checked = selectedTags.includes(tag.id);
                    const inputID = `task-tag-${tag.id}`;
                    return (
                      <label
                        key={tag.id}
                        htmlFor={inputID}
                        className="flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm sm:min-h-8 sm:px-2 sm:py-1"
                      >
                        <Checkbox
                          id={inputID}
                          checked={checked}
                          onCheckedChange={() =>
                            setSelectedTags((ids) =>
                              ids.includes(tag.id) ? ids.filter((id) => id !== tag.id) : [...ids, tag.id],
                            )
                          }
                        />
                        <span className={checked ? "" : "text-muted-foreground"}>{tag.name}</span>
                      </label>
                    );
                  })}
                  {tags.length === 0 && <span className="text-sm text-muted-foreground">暂无标签</span>}
                </div>
              </div>
            </div>

            <div className="space-y-4 max-lg:order-first">
              {!createMode && automation && (
                <section className="space-y-3 rounded-lg border p-3">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="flex items-center gap-2 text-sm font-medium">
                      <RotateCw className="size-4" />
                      自动执行
                    </h3>
                    <span className="text-xs text-muted-foreground">
                      {automation.parent_task_id != null
                        ? "由父任务统一执行"
                        : automationStatusLabel(automation.automation_status)}
                    </span>
                  </div>
                  {automation.automation_error ? (
                    <p className="rounded-md bg-destructive/10 px-2 py-1.5 text-xs text-destructive">
                      {automation.automation_error}
                    </p>
                  ) : null}
                  {automation.parent_task_id != null && (
                    <div className="rounded-xl border border-blue-500/20 bg-blue-500/[0.06] p-3">
                      <p className="text-sm font-medium">目标步骤 · 随父任务执行</p>
                      <p className="mt-1 text-xs leading-5 text-muted-foreground">
                        子任务会作为目标上下文交给同一组多 Agent 执行，不会单独创建执行任务。
                      </p>
                      <Button size="sm" variant="outline" className="mt-3 h-11 sm:h-8" asChild>
                        <Link href={`/work/inbox?task=${encodeURIComponent(String(automation.parent_task_id))}`}>
                          返回父任务查看执行
                          <ExternalLink className="size-3.5" />
                        </Link>
                      </Button>
                    </div>
                  )}
                  {automation.parent_task_id == null &&
                    (automation.automation_status === "done" || automation.automation_status === "failed") && (
                    <div
                      className={
                        automation.automation_status === "failed"
                          ? "space-y-2 rounded-xl border border-amber-500/25 bg-amber-500/[0.06] p-3"
                          : "space-y-2 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.06] p-3"
                      }
                    >
                      <div className="flex items-center gap-2">
                        {automation.automation_status === "failed" ? (
                          <AlertTriangle className="size-4 text-amber-600" />
                        ) : (
                          <CheckCircle2 className="size-4 text-emerald-600" />
                        )}
                        <span className="text-sm font-semibold">
                          {automation.automation_status === "failed" ? "失败前结果" : "执行结果"}
                        </span>
                      </div>
                      <p className="whitespace-pre-wrap break-words text-sm leading-6 text-foreground/85">
                        {automation.automation_result?.trim() ||
                          (automation.automation_status === "failed"
                            ? "本次执行没有留下可展示的文本结果，请查看错误信息和执行过程。"
                            : "执行已结束，但执行活动里没有可展示的最终结果。请展开执行过程查看 Agent 活动。")}
                      </p>
                      {automation.artex_task?.deleted && (
                        <p className="rounded-lg border border-amber-500/25 bg-amber-500/[0.08] px-2.5 py-2 text-xs leading-5 text-amber-800">
                          关联执行任务已删除，无法再打开实时执行过程；最终结果已保留在此任务中。
                        </p>
                      )}
                    </div>
                    )}
                  {automation.automation_status === "running" && !automation.artex_task_id && (
                    <div className="flex items-start gap-2 rounded-xl border border-blue-500/20 bg-blue-500/[0.06] p-3 text-sm">
                      <Loader2 className="mt-0.5 size-4 shrink-0 animate-spin text-blue-600" />
                      <div>
                        <p className="font-medium">正在启动多 Agent 执行</p>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">
                          goals 正在拆解目标，关联任务同步后会自动显示实时执行过程。
                        </p>
                      </div>
                    </div>
                  )}
                  {automation.artex_task_id ? (
                    <div className="space-y-2 rounded-md border bg-muted/30 px-3 py-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-xs font-medium">关联执行任务</span>
                        {automation.artex_task?.status && (
                          <span className="rounded border bg-background px-1.5 py-0.5 text-xs">
                            {automation.artex_task.status}
                          </span>
                        )}
                        {automation.artex_task?.paused && (
                          <span className="rounded border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-xs text-amber-700">
                            已暂停
                          </span>
                        )}
                        {automation.artex_task?.queued && (
                          <span className="rounded border px-1.5 py-0.5 text-xs text-muted-foreground">
                            排队中
                          </span>
                        )}
                        {automation.artex_task?.deleted && (
                          <span className="rounded border border-destructive/30 px-1.5 py-0.5 text-xs text-destructive">
                            已删除
                          </span>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="min-w-0 flex-1 truncate text-sm font-medium">
                          {automation.artex_task?.name ||
                            automation.artex_task?.description ||
                            automation.title}
                        </p>
                        {executionAvailable ? (
                          <Button size="sm" variant="outline" className="h-11 sm:h-7" asChild>
                            <Link
                              href={`/function/tasks/detail?id=${encodeURIComponent(automation.artex_task_id)}&work_task=${encodeURIComponent(String(automation.id))}`}
                            >
                              查看多 Agent 执行过程
                              <ExternalLink className="size-3.5" />
                            </Link>
                          </Button>
                        ) : (
                          <span className="text-xs text-muted-foreground">
                            执行任务已删除，最终结果仍可查看
                          </span>
                        )}
                      </div>
                    </div>
                  ) : null}
                  <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    {automation.automation_launched_at && (
                      <span>启动 {new Date(automation.automation_launched_at).toLocaleString()}</span>
                    )}
                    {automation.automation_finished_at && (
                      <span>结束 {new Date(automation.automation_finished_at).toLocaleString()}</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    {automation.parent_task_id == null &&
                      (automation.automation_status === "pending" ||
                        automation.automation_status === "failed") && (
                      <Button
                        size="sm"
                        className="h-11 sm:h-7"
                        disabled={automationBusy}
                        onClick={() => void runAutomation()}
                      >
                        {automationBusy ? <Loader2 className="size-4 animate-spin" /> : <Play className="size-4" />}
                        {automation.automation_status === "failed" ? "重试" : "立即执行"}
                      </Button>
                    )}
                    {automation.parent_task_id == null &&
                      (automation.automation_status === "pending" ||
                        automation.automation_status === "running") && (
                      <Button
                        size="sm"
                        variant="outline"
                        className="h-11 sm:h-7"
                        disabled={automationBusy}
                        onClick={() => void cancelAutomation()}
                      >
                        <XCircle className="size-4" />
                        取消自动执行
                      </Button>
                    )}
                  </div>
                </section>
              )}

              {!createMode && (
                <section className="space-y-2 rounded-lg border p-3">
                  <h3 className="text-sm font-medium">子任务</h3>
                  <div className="space-y-2">
                    {subtasks.map((item) => (
                      <div key={item.id} className="flex items-center justify-between gap-2 text-sm">
                        <span className="min-w-0 flex-1 truncate">{item.title}</span>
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-11 sm:h-7"
                          onClick={() => void completeSubtask(item.id)}
                        >
                          完成
                        </Button>
                      </div>
                    ))}
                    {subtasks.length === 0 && <p className="text-sm text-muted-foreground">暂无子任务。</p>}
                  </div>
                  <form onSubmit={addSubtask} className="flex gap-2">
                    <Input
                      className="h-11 sm:h-8"
                      value={subtaskTitle}
                      onChange={(e) => setSubtaskTitle(e.target.value)}
                      placeholder="添加子任务"
                    />
                    <Button
                      type="submit"
                      size="icon"
                      className="size-11 sm:size-8"
                      disabled={!subtaskTitle.trim()}
                      aria-label="添加子任务"
                    >
                      <Plus />
                    </Button>
                  </form>
                </section>
              )}

              <section className="space-y-3 rounded-lg border p-3">
                <h3 className="text-sm font-medium">提醒</h3>
                {remindersLoading ? (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" /> 正在读取提醒配置...
                  </div>
                ) : reminderLoadError ? (
                  <div className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-3">
                    <p className="text-sm text-destructive">提醒读取失败：{reminderLoadError}</p>
                    <p className="text-xs text-muted-foreground">
                      为避免把默认值覆盖到原配置，提醒已暂时锁定。任务其它字段仍可保存。
                    </p>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="h-11 sm:h-7"
                      onClick={() => setReminderReloadKey((value) => value + 1)}
                    >
                      <RotateCw className="size-4" />
                      重试读取
                    </Button>
                  </div>
                ) : (
                  <>
                    <div className="flex items-center justify-between gap-3 text-sm">
                      <span>到期前提醒</span>
                      <div className="flex items-center gap-2">
                        <Input
                          type="number"
                          min={0}
                          value={reminders.dueMinutes}
                          disabled={!reminders.dueEnabled}
                          onChange={(e) => setReminders((v) => ({ ...v, dueMinutes: Number(e.target.value) }))}
                          className="h-11 w-24 sm:h-8 sm:w-20"
                        />
                        <span className="text-muted-foreground">分钟</span>
                        <Checkbox
                          checked={reminders.dueEnabled}
                          onCheckedChange={(checked) =>
                            setReminders((v) => ({ ...v, dueEnabled: checked === true }))
                          }
                        />
                      </div>
                    </div>
                    <ChannelPicker
                      label="到期提醒渠道"
                      channels={channels}
                      disabled={!reminders.dueEnabled}
                      selected={reminders.dueChannelIDs}
                      onChange={(ids) => setReminders((value) => ({ ...value, dueChannelIDs: ids }))}
                    />
                    <div className="flex items-center justify-between gap-3 text-sm">
                      <span>逾期后提醒一次</span>
                      <Checkbox
                        checked={reminders.overdueEnabled}
                        onCheckedChange={(checked) =>
                          setReminders((v) => ({ ...v, overdueEnabled: checked === true }))
                        }
                      />
                    </div>
                    <ChannelPicker
                      label="逾期提醒渠道"
                      channels={channels}
                      disabled={!reminders.overdueEnabled}
                      selected={reminders.overdueChannelIDs}
                      onChange={(ids) => setReminders((value) => ({ ...value, overdueChannelIDs: ids }))}
                    />
                    <p className="text-xs text-muted-foreground">
                      未选择渠道时使用“通知推送”中的个人默认渠道；浏览器系统提醒需在工作台顶部单独授权。
                    </p>
                  </>
                )}
              </section>

              <WorkTaskAssetsPanel
                taskId={task?.id}
                initialPinned={assetSelection.pinned}
                initialExcluded={assetSelection.excluded}
                onSelectionChange={setAssetSelection}
              />
            </div>
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}
        <DialogFooter className="sticky bottom-0 z-10 sm:justify-between">
          <div>
            {!createMode && task && (
              <Button
                type="button"
                variant="destructive"
                className="h-11 sm:h-8"
                disabled={saving || deleting}
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 className="size-4" />
                删除任务
              </Button>
            )}
          </div>
          <div className="flex flex-col-reverse gap-2 sm:flex-row">
            <Button variant="outline" className="h-11 sm:h-8" onClick={onClose}>
              取消
            </Button>
            <Button
              className="h-11 sm:h-8"
              onClick={() => void save()}
              disabled={saving || !title.trim() || remindersLoading}
            >
              {saving && <Loader2 className="size-4 animate-spin" />}
              {createMode ? "创建任务" : "保存"}
            </Button>
          </div>
        </DialogFooter>

        <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>删除「{task?.title || "未命名任务"}」？</AlertDialogTitle>
              <AlertDialogDescription>
                此操作不可撤销，并会永久删除该任务的全部子任务和提醒。
                {task && workTaskIsAutomating(task)
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
                {deleting && <Loader2 className="size-4 animate-spin" />}
                {deleting ? "删除中" : "确认删除"}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </DialogContent>
    </Dialog>
  );
}

function toLocalInput(date: Date) {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function ChannelPicker({
  label,
  channels,
  selected,
  disabled,
  onChange,
}: {
  label: string;
  channels: NotificationChannel[];
  selected: number[];
  disabled: boolean;
  onChange: (ids: number[]) => void;
}) {
  const fieldID = React.useId();

  return (
    <fieldset className="grid gap-2" disabled={disabled}>
      <legend className="text-sm font-medium">{label}</legend>
      <div className={cn("flex flex-wrap gap-2", disabled && "opacity-50")}>
        {channels.map((channel) => {
          const checked = selected.includes(channel.id);
          const inputID = `${fieldID}-${channel.id}`;
          return (
            <label
              key={channel.id}
              htmlFor={inputID}
              className="flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm sm:min-h-8 sm:px-2 sm:py-1"
            >
              <Checkbox
                id={inputID}
                checked={checked}
                disabled={disabled}
                onCheckedChange={() =>
                  onChange(checked ? selected.filter((id) => id !== channel.id) : [...selected, channel.id])
                }
              />
              <span>{channel.name}</span>
            </label>
          );
        })}
        {channels.length === 0 && <span className="text-sm text-muted-foreground">暂无可用通知渠道</span>}
      </div>
    </fieldset>
  );
}
