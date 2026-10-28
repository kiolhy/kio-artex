"use client";

import * as React from "react";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

import {
  ArchiveIcon,
  ArrowLeftIcon,
  BrainIcon,
  CheckIcon,
  CircleAlertIcon,
  PauseIcon,
  PlayIcon,
  RefreshCwIcon,
} from "lucide-react";
import { toast } from "sonner";

import { StatusBadge } from "@/components/status-badge";
import { TaskLLMProfileChain } from "@/components/task-llm-profile-chain";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Spinner } from "@/components/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { api, HttpError } from "@/lib/api";
import type { LLMProfile, Task } from "@/lib/types";

import { BroadcastTab } from "./_tabs/broadcast-tab";
import { GraphTab } from "./_tabs/graph-tab";
import { InterceptTab } from "./_tabs/intercept-tab";
import { OverviewTab } from "./_tabs/overview-tab";
import { SessionsTab } from "./_tabs/sessions-tab";
import { WorkExecutionDetail } from "./_components/work-execution-detail";

const TABS = [
  { value: "sessions", label: "会话" },
  { value: "overview", label: "总览" },
  { value: "graph", label: "执行活动" },
  { value: "broadcast", label: "播报板" },
  { value: "intercept", label: "待确认操作" },
];

function taskProfileIDs(task: Task): string[] {
  if (task.llm_profile_ids && task.llm_profile_ids.length > 0) {
    return task.llm_profile_ids.map(String);
  }
  return task.llm_profile_id ? [String(task.llm_profile_id)] : [];
}

function TaskLLMControl({ task, profiles, onUpdated }: { task: Task; profiles: LLMProfile[]; onUpdated: () => void }) {
  const [open, setOpen] = React.useState(false);
  const [profileIDs, setProfileIDs] = React.useState<string[]>(() => taskProfileIDs(task));
  const [activeProfileID, setActiveProfileID] = React.useState(
    task.active_llm_profile_id ? String(task.active_llm_profile_id) : (taskProfileIDs(task)[0] ?? ""),
  );
  const [saving, setSaving] = React.useState(false);
  const popoverContentRef = React.useRef<HTMLDivElement>(null);

  const chain = taskProfileIDs(task);
  const exhausted = task.llm_failover_state === "chain_exhausted";
  // 任何状态都可以改链,终态也不例外:任务结束后主 Agent 对话仍走这条链,
  // 链上模型出问题时必须能换掉,否则已完成任务就没法继续交互。
  const terminal = ["done", "failed", "timeout"].includes(task.status);
  // A null active profile on an exhausted, non-empty chain is a persisted end
  // cursor. Keep the status display honest; choosing the first profile is only
  // the editor's reset draft and does not mean it is currently active.
  let activeID = chain[0] ?? "";
  if (exhausted) activeID = "";
  if (task.active_llm_profile_id) activeID = String(task.active_llm_profile_id);
  const activeProfile = profiles.find((profile) => profile.id === activeID);
  const currentLabel = exhausted
    ? "配置链已耗尽"
    : (activeProfile?.name ?? (activeID ? `配置 #${activeID}` : "跟随默认配置"));
  const activeIndex = chain.indexOf(activeID);
  const backupCount = activeIndex >= 0 ? Math.max(0, chain.length - activeIndex - 1) : 0;
  const currentTitle = [currentLabel, activeProfile?.model, backupCount > 0 ? `${backupCount} 个备用` : ""]
    .filter(Boolean)
    .join(" · ");
  let editorDescription = "调整顺序或当前配置后，将从下一次 LLM 调用开始生效。";
  if (terminal) editorDescription = "任务已结束，改动只影响后续的主 Agent 对话。";
  let saveLabel = "保存";
  if (exhausted) saveLabel = "保存并重置";
  if (saving) saveLabel = "保存中";

  const syncDraft = React.useCallback(() => {
    const next = taskProfileIDs(task);
    setProfileIDs(next);
    setActiveProfileID(task.active_llm_profile_id ? String(task.active_llm_profile_id) : (next[0] ?? ""));
  }, [task]);

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (next) syncDraft();
  };

  const handleProfileIDsChange = (next: string[]) => {
    setProfileIDs(next);
    setActiveProfileID((current) => (next.includes(current) ? current : (next[0] ?? "")));
  };

  const save = async () => {
    setSaving(true);
    try {
      const result = await api.updateTaskLLMProfiles(
        task.id,
        profileIDs.map(Number),
        activeProfileID ? Number(activeProfileID) : undefined,
      );
      if (result.switch_event) {
        toast.info(result.switch_event.summary, { id: `task-${task.id}-llm-${result.switch_event.seq}` });
      } else {
        toast.success(
          result.reopened_intents > 0
            ? `LLM 配置已更新，并恢复 ${result.reopened_intents} 条额度阻塞意图`
            : "LLM 配置已更新",
        );
      }
      setOpen(false);
      onUpdated();
    } catch (error) {
      toast.error("更新失败：" + (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          size="sm"
          variant={exhausted ? "destructive" : "outline"}
          aria-label="查看或切换任务 LLM 配置"
          title={currentTitle}
        >
          <BrainIcon data-icon="inline-start" />
          <span className="hidden max-w-36 truncate lg:inline">{currentLabel}</span>
          {backupCount > 0 && <span className="hidden text-muted-foreground xl:inline">+{backupCount}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent ref={popoverContentRef} align="start" className="w-[min(28rem,calc(100vw-2rem))] gap-4 p-4">
        <PopoverHeader>
          <PopoverTitle>任务 LLM 配置链</PopoverTitle>
          <PopoverDescription>{editorDescription}</PopoverDescription>
        </PopoverHeader>

        {exhausted && (
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>配置链额度已耗尽</AlertTitle>
            <AlertDescription>
              {task.llm_failover_reason ?? "所有已选配置均被判定为额度不足。保存配置链可重置故障状态。"}
            </AlertDescription>
          </Alert>
        )}

        <TaskLLMProfileChain
          profiles={profiles}
          value={profileIDs}
          onValueChange={handleProfileIDsChange}
          activeProfileId={activeProfileID}
          onActiveProfileChange={setActiveProfileID}
          inputId="task-llm-profiles"
          disabled={saving}
          portalContainer={popoverContentRef}
        />

        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => setOpen(false)}>
            关闭
          </Button>
          <Button type="button" size="sm" onClick={save} disabled={saving}>
            {saving && <Spinner data-icon="inline-start" />}
            {saveLabel}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

function TaskDetailInner({ id }: { id: string }) {
  const router = useRouter();
  const [task, setTask] = React.useState<Task | null>(null);
  const [paused, setPaused] = React.useState(false);
  const [loaded, setLoaded] = React.useState(false);
  const [loadError, setLoadError] = React.useState("");
  const [taskNotFound, setTaskNotFound] = React.useState(false);
  const [statsError, setStatsError] = React.useState("");
  const [tab, setTab] = React.useState("sessions");
  const [interceptPendingCount, setInterceptPendingCount] = React.useState(0);
  const [profiles, setProfiles] = React.useState<LLMProfile[]>([]);
  const [profilesError, setProfilesError] = React.useState("");
  const [archiving, setArchiving] = React.useState(false);

  const reloadProfiles = React.useCallback(() => {
    setProfilesError("");
    api
      .llmProfiles()
      .then((rows) => {
        setProfiles(rows);
        setProfilesError("");
      })
      .catch((error) => {
        setProfilesError((error as Error).message || "LLM 配置列表读取失败");
      });
  }, []);

  React.useEffect(() => {
    reloadProfiles();
  }, [reloadProfiles]);

  React.useEffect(() => {
    let alive = true;
    const load = () =>
      api
        .interceptTask(id)
        .then((rows) => {
          if (alive) setInterceptPendingCount(rows.filter((r) => r.status === "pending").length);
        })
        .catch(() => {
          // Polling is best-effort; the next interval retries automatically.
        });
    void load();
    const t = setInterval(load, 5000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [id]);

  const taskLoadInFlight = React.useRef<string | null>(null);
  const load = React.useCallback(() => {
    if (taskLoadInFlight.current === id) return;
    taskLoadInFlight.current = id;
    const taskRequest = api
      .task(id)
      .then((next) => {
        if (taskLoadInFlight.current !== id) return;
        setTask(next);
        setPaused(next.paused ?? false);
        setLoadError("");
        setTaskNotFound(false);
      })
      .catch((error) => {
        if (taskLoadInFlight.current !== id) return;
        const message = (error as Error).message || "任务读取失败";
        setLoadError(message);
        setTaskNotFound(error instanceof HttpError && (error.status === 404 || error.status === 410));
      })
      .finally(() => {
        if (taskLoadInFlight.current === id) {
          taskLoadInFlight.current = null;
          setLoaded(true);
        }
      });
    const statsRequest = api
      .stats(id)
      .then((stats) => {
        const at = stats.active_task;
        setStatsError("");
        setTask((base) => {
          if (!base) return base;
          const next = { ...base };
          if (at) {
            next.in_flight = at.in_flight;
            next.goals_total = at.goals_total;
            next.goals_met = at.goals_met;
            next.engine_mode = stats.engine_mode ?? at.engine_mode;
            next.paused = at.paused;
          }
          return next;
        });
        if (at && typeof at.paused === "boolean") setPaused(at.paused);
      })
      .catch((error) => {
        setStatsError((error as Error).message || "任务统计读取失败");
      });
    return Promise.allSettled([taskRequest, statsRequest]);
  }, [id]);
  React.useEffect(() => {
    load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [load]);

  async function togglePause() {
    if (task && ["done", "failed", "timeout"].includes(task.status)) return;
    const next = !paused;
    try {
      await api.controlTask(id, next ? "pause" : "resume");
      setPaused(next);
      toast.success(next ? "已暂停探索" : "已恢复探索");
    } catch (e) {
      toast.error("操作失败：" + (e as Error).message);
    }
  }

  async function archiveTask() {
    if (!task || archiving) return;
    setArchiving(true);
    try {
      await api.archiveTask(task.id);
      toast.success("任务已加入归档队列");
      router.push("/function/tasks");
    } catch (error) {
      toast.error(`归档失败：${(error as Error).message}`);
      setArchiving(false);
    }
  }

  if (!task) {
    if (!loaded) {
      return (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
          <p className="text-muted-foreground">加载中…</p>
        </div>
      );
    }
    if (!loadError) {
      return (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
          <p className="text-muted-foreground">任务不存在或当前不可访问</p>
          <Button asChild variant="outline">
            <Link href="/function/tasks">
              <ArrowLeftIcon /> 返回任务列表
            </Link>
          </Button>
        </div>
      );
    }
    if (!taskNotFound) {
      return (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
          <CircleAlertIcon className="size-8 text-destructive" />
          <div>
            <p className="font-medium">任务读取失败</p>
            <p className="text-muted-foreground mt-1 max-w-lg text-sm">{loadError}</p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => load()}>
              <RefreshCwIcon /> 重试
            </Button>
            <Button asChild variant="ghost">
              <Link href="/function/tasks">
                <ArrowLeftIcon /> 返回任务列表
              </Link>
            </Button>
          </div>
        </div>
      );
    }
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
        <p className="text-muted-foreground">任务 {id} 已被删除、归档或不存在</p>
        <Button asChild variant="outline">
          <Link href="/function/tasks">
            <ArrowLeftIcon /> 返回任务列表
          </Link>
        </Button>
      </div>
    );
  }

  const completed = task.status === "done";
  const terminal = ["done", "failed", "timeout"].includes(task.status);
  const archiveLifecycleEligible = terminal || paused || task.status === "paused";
  const canArchive = archiveLifecycleEligible && !task.archive_blocked_by_task_id;
  let archiveDisabledReason = task.queued ? "排队中的任务必须先暂停" : "运行中的任务必须先暂停";
  if (archiveLifecycleEligible && task.archive_blocked_by_task_id) {
    archiveDisabledReason = `任务被未归档任务 #${task.archive_blocked_by_task_id} 直接继承，请先归档依赖任务`;
  }
  const engineMode = paused ? "paused" : (task.engine_mode ?? "idle");
  let controlVariant: "default" | "secondary" | "outline" = "outline";
  let controlIcon = <PauseIcon data-icon="inline-start" />;
  let controlLabel = "暂停";
  if (terminal) {
    controlVariant = "secondary";
    controlIcon = <CheckIcon data-icon="inline-start" />;
    controlLabel = completed ? "已完成" : "已结束";
  } else if (paused) {
    controlVariant = "default";
    controlIcon = <PlayIcon data-icon="inline-start" />;
    controlLabel = "恢复";
  }
  const archiveTrigger = (
    <Button
      size="icon-sm"
      variant="ghost"
      disabled={!canArchive || archiving}
      aria-label={canArchive ? "归档任务" : archiveDisabledReason}
    >
      {archiving ? <Spinner /> : <ArchiveIcon />}
    </Button>
  );

  return (
    <Tabs value={tab} onValueChange={setTab} className="flex flex-1 flex-col gap-0">
      {/* Top fixed area */}
      <header className="sticky top-0 z-10 flex flex-col gap-2 border-b bg-background/95 px-4 py-2.5 backdrop-blur lg:px-6">
        <div className="flex items-center gap-2">
          <SidebarTrigger className="-ml-1" />
          <Button asChild variant="ghost" size="icon" className="size-7">
            <Link href="/function/tasks">
              <ArrowLeftIcon />
            </Link>
          </Button>
          <h1 className="min-w-0 flex-1 truncate text-sm font-semibold" title={task.name || task.description}>
            {task.name || task.description}
          </h1>
          <code className="hidden rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground sm:inline">
            {task.id}
          </code>
          <Separator orientation="vertical" className="mx-1 hidden h-4 sm:block" />
          <TaskLLMControl task={task} profiles={profiles} onUpdated={load} />
          <StatusBadge domain={terminal ? "task" : "engine"} value={terminal ? task.status : engineMode} dot />
          {canArchive ? (
            <AlertDialog>
              <AlertDialogTrigger asChild>{archiveTrigger}</AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>归档任务 #{task.id}？</AlertDialogTitle>
                  <AlertDialogDescription>
                    执行活动、关联记录、工作文件、资料和 LLM
                    历史将压缩到冷存储。归档完成后可在任务列表的“已归档”页还原。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>取消</AlertDialogCancel>
                  <AlertDialogAction onClick={() => void archiveTask()}>确认归档</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex">{archiveTrigger}</span>
              </TooltipTrigger>
              <TooltipContent>{archiveDisabledReason}</TooltipContent>
            </Tooltip>
          )}
          <Button size="sm" variant={controlVariant} onClick={togglePause} disabled={terminal}>
            {controlIcon}
            {controlLabel}
          </Button>
        </div>
        <p className="truncate text-xs text-muted-foreground">{task.goal}</p>
        {/* Tabs */}
        <div className="no-scrollbar min-w-0 overflow-x-auto">
          <TabsList variant="default" className="min-w-max">
            {TABS.map((t) => (
              <TabsTrigger key={t.value} value={t.value}>
                {t.label}
                {t.value === "intercept" && interceptPendingCount > 0 && (
                  <span className="ml-1.5 inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-amber-500 px-1 text-xs font-semibold leading-none text-white">
                    {interceptPendingCount > 99 ? "99+" : interceptPendingCount}
                  </span>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
      </header>

      {/* Tab content */}
      <div className="flex-1 p-4 lg:p-6">
        {loadError && (
          <Alert className="mb-4" variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>任务信息刷新失败</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
              <span>当前展示的是上一次成功读取的数据：{loadError}</span>
              <Button size="sm" variant="outline" onClick={() => void load()}>
                <RefreshCwIcon /> 重试
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {statsError && (
          <Alert className="mb-4" variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>任务统计暂不可用</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
              <span>任务基础信息仍可查看：{statsError}</span>
              <Button size="sm" variant="outline" onClick={() => void load()}>
                <RefreshCwIcon /> 重试
              </Button>
            </AlertDescription>
          </Alert>
        )}
        {profilesError && (
          <Alert className="mb-4" variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>LLM 配置列表读取失败</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
              <span>任务内容不受影响，但暂时无法切换模型配置：{profilesError}</span>
              <Button size="sm" variant="outline" onClick={reloadProfiles}>
                <RefreshCwIcon /> 重试
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <TabsContent value="sessions" className="mt-0">
          <SessionsTab taskId={id} />
        </TabsContent>
        <TabsContent value="overview" className="mt-0">
          <OverviewTab taskId={id} />
        </TabsContent>
        <TabsContent value="graph" className="mt-0">
          <GraphTab taskId={id} />
        </TabsContent>
        <TabsContent value="broadcast" className="mt-0">
          <BroadcastTab taskId={id} />
        </TabsContent>
        <TabsContent value="intercept" className="mt-0">
          <InterceptTab taskId={id} />
        </TabsContent>
      </div>
    </Tabs>
  );
}

// useSearchParams must sit under a Suspense boundary for static export.
export default function TaskDetailPage() {
  return (
    <React.Suspense fallback={null}>
      <TaskDetailRoute />
    </React.Suspense>
  );
}

function TaskDetailRoute() {
  const searchParams = useSearchParams();
  const workTaskID = Number(searchParams.get("work_task") || 0);
  if (Number.isSafeInteger(workTaskID) && workTaskID > 0) {
    return (
      <WorkExecutionDetail
        workTaskID={workTaskID}
        artexTaskID={searchParams.get("id") || undefined}
      />
    );
  }
  return <TaskDetailInner id={searchParams.get("id") ?? ""} />;
}
