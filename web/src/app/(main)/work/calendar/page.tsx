"use client";

import * as React from "react";
import dayGridPlugin from "@fullcalendar/react/daygrid";
import interactionPlugin from "@fullcalendar/react/interaction";
import timeGridPlugin from "@fullcalendar/react/timegrid";
import { CalendarPlus, Clock3, Loader2, RefreshCw, X } from "lucide-react";
import { toast } from "sonner";

import { EventCalendarViews } from "@/components/calendar/event-calendar-views";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { NotificationChannel, WorkCalendarEvent, WorkProject, WorkTag, WorkTask } from "@/lib/types";
import { WORK_CHANGED_EVENT, emitWorkChanged } from "@/lib/work-events";

import { TaskDetailDialog, type WorkTaskDraft } from "../_components/task-detail-dialog";
import { WorkErrorState, WorkPageHeader } from "../_components/work-shell";

type CalendarRange = {
  from: Date;
  to: Date;
};

type ChangedEventInfo = {
  event: {
    id: string | number;
    start: Date | null;
    end: Date | null;
    allDay: boolean;
  };
  revert: () => void;
};

export default function WorkCalendarPage() {
  const [events, setEvents] = React.useState<WorkCalendarEvent[]>([]);
  const [todayEvents, setTodayEvents] = React.useState<WorkCalendarEvent[]>([]);
  const [unscheduledTasks, setUnscheduledTasks] = React.useState<WorkTask[]>([]);
  const [projects, setProjects] = React.useState<WorkProject[]>([]);
  const [tags, setTags] = React.useState<WorkTag[]>([]);
  const [channels, setChannels] = React.useState<NotificationChannel[]>([]);
  const [range, setRange] = React.useState<CalendarRange>(() => defaultRange());
  const [detailTask, setDetailTask] = React.useState<WorkTask | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [createDraft, setCreateDraft] = React.useState<WorkTaskDraft>();
  const [loading, setLoading] = React.useState(true);
  const [loadedOnce, setLoadedOnce] = React.useState(false);
  const [optionsError, setOptionsError] = React.useState<string>();
  const [optionsLoading, setOptionsLoading] = React.useState(true);
  const [schedulingTaskID, setSchedulingTaskID] = React.useState<number>();
  const [error, setError] = React.useState<string>();
  const [todayLoading, setTodayLoading] = React.useState(true);
  const [todayError, setTodayError] = React.useState<string>();
  const [unscheduledError, setUnscheduledError] = React.useState<string>();
  const [notice, setNotice] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const [calendarHeight, setCalendarHeight] = React.useState(640);
  const requestSequence = React.useRef(0);
  const todayRequestSequence = React.useRef(0);
  const unscheduledRequestSequence = React.useRef(0);
  const optionsRequestSequence = React.useRef(0);

  const loadEvents = React.useCallback(async (nextRange: CalendarRange) => {
    const requestID = ++requestSequence.current;
    setLoading(true);
    try {
      const result = await api.workCalendarEvents(nextRange.from.toISOString(), nextRange.to.toISOString());
      if (requestID !== requestSequence.current) return;
      setEvents(result);
      setError(undefined);
    } catch (e) {
      if (requestID === requestSequence.current) setError((e as Error).message);
    } finally {
      if (requestID === requestSequence.current) {
        setLoading(false);
        setLoadedOnce(true);
      }
    }
  }, []);

  const loadUnscheduled = React.useCallback(async () => {
    const requestID = ++unscheduledRequestSequence.current;
    try {
      const tasks = await api.workTasks({
        status: "todo,doing",
        parent_only: "1",
      });
      if (requestID !== unscheduledRequestSequence.current) return;
      setUnscheduledTasks(tasks.filter((task) => !task.start_at && !task.due_at));
      setUnscheduledError(undefined);
    } catch (e) {
      if (requestID === unscheduledRequestSequence.current) {
        setUnscheduledError((e as Error).message);
      }
    }
  }, []);

  const loadToday = React.useCallback(async (quiet = false) => {
    const requestID = ++todayRequestSequence.current;
    if (!quiet) setTodayLoading(true);
    const from = new Date();
    from.setHours(0, 0, 0, 0);
    const to = new Date(from);
    to.setDate(to.getDate() + 1);
    try {
      const result = await api.workCalendarEvents(from.toISOString(), to.toISOString());
      if (requestID !== todayRequestSequence.current) return;
      setTodayEvents(
        result.filter(
          (event) => event.source === "work" && !["done", "canceled"].includes(event.status),
        ),
      );
      setTodayError(undefined);
    } catch (cause) {
      if (requestID === todayRequestSequence.current) setTodayError((cause as Error).message);
    } finally {
      if (requestID === todayRequestSequence.current) setTodayLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void loadEvents(range);
  }, [loadEvents, range, reloadKey]);

  React.useEffect(() => {
    void loadToday();
  }, [loadToday, reloadKey]);

  React.useEffect(() => {
    void loadUnscheduled();
  }, [loadUnscheduled, reloadKey]);

  React.useEffect(() => {
    const media = window.matchMedia("(max-width: 767px)");
    const sync = () => setCalendarHeight(media.matches ? 540 : 640);
    media.addEventListener("change", sync);
    sync();
    return () => media.removeEventListener("change", sync);
  }, []);

  const loadOptions = React.useCallback(async () => {
    const requestID = ++optionsRequestSequence.current;
    setOptionsLoading(true);
    try {
      const [projectResult, tagResult, channelResult] = await Promise.allSettled([
        api.workProjects(),
        api.workTags(),
        api.notifyChannels(),
      ]);
      if (requestID !== optionsRequestSequence.current) return;
      if (projectResult.status === "fulfilled") setProjects(projectResult.value);
      if (tagResult.status === "fulfilled") setTags(tagResult.value);
      if (channelResult.status === "fulfilled") setChannels(channelResult.value);
      setOptionsError(
        [projectResult, tagResult, channelResult].some((result) => result.status === "rejected")
          ? "部分项目、标签或通知渠道加载失败，日程仍可查看和改期。"
          : undefined,
      );
    } finally {
      if (requestID === optionsRequestSequence.current) setOptionsLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void loadOptions();
    return () => {
      optionsRequestSequence.current += 1;
    };
  }, [loadOptions]);

  React.useEffect(() => {
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") setReloadKey((value) => value + 1);
    };
    const refreshOnWorkChange = () => setReloadKey((value) => value + 1);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    window.addEventListener(WORK_CHANGED_EVENT, refreshOnWorkChange);
    return () => {
      document.removeEventListener("visibilitychange", refreshWhenVisible);
      window.removeEventListener(WORK_CHANGED_EVENT, refreshOnWorkChange);
    };
  }, []);

  function handleDatesSet(info: { start: Date; end: Date }) {
    setRange((current) =>
      current.from.getTime() === info.start.getTime() && current.to.getTime() === info.end.getTime()
        ? current
        : { from: new Date(info.start), to: new Date(info.end) },
    );
  }

  function openCreateAt(date: Date, allDay: boolean) {
    const start = new Date(date);
    if (allDay) start.setHours(9, 0, 0, 0);
    const due = new Date(start.getTime() + 30 * 60_000);
    setCreateDraft({
      start_at: start.toISOString(),
      due_at: due.toISOString(),
      tag_ids: [],
    });
    setCreateOpen(true);
  }

  async function openEvent(eventID: string | number, source?: string) {
    if (source !== "work") {
      toast.info("该日程不支持在此编辑");
      return;
    }
    await openWorkTask(Number(eventID));
  }

  async function openWorkTask(taskID: number) {
    try {
      setDetailTask(await api.workTask(taskID));
    } catch (e) {
      toast.error("读取任务详情失败", { description: (e as Error).message });
    }
  }

  async function handleTimeChange(info: ChangedEventInfo) {
    const event = events.find((item) => String(item.id) === String(info.event.id) && item.resourceId === "work");
    if (!event || !info.event.start) {
      info.revert();
      return;
    }
    const start = new Date(info.event.start);
    const rawDuration =
      info.event.end && info.event.start
        ? new Date(info.event.end).getTime() - new Date(info.event.start).getTime()
        : 0;
    const duration = Math.max(rawDuration, 30 * 60_000);
    const due = new Date(start.getTime() + duration);
    const previous = events;
    const previousToday = todayEvents;
    setEvents((items) =>
      items.map((item) =>
        item.id === event.id
          ? { ...item, start: start.toISOString(), end: due.toISOString() }
          : item,
      ),
    );
    setTodayEvents((items) =>
      items.map((item) =>
        item.id === event.id
          ? { ...item, start: start.toISOString(), end: due.toISOString() }
          : item,
      ),
    );
    try {
      const updated = await api.updateWorkTask(event.id, {
        start_at: start.toISOString(),
        due_at: due.toISOString(),
      });
      setEvents((items) =>
        items.map((item) =>
          item.id === event.id
            ? { ...item, start: updated.start_at ?? start.toISOString(), end: updated.due_at ?? due.toISOString() }
            : item,
        ),
      );
      setTodayEvents((items) =>
        items.map((item) =>
          item.id === event.id
            ? { ...item, start: updated.start_at ?? start.toISOString(), end: updated.due_at ?? due.toISOString() }
            : item,
        ),
      );
      emitWorkChanged();
      setNotice(`已更新「${event.title}」的时间`);
    } catch (e) {
      setEvents(previous);
      setTodayEvents(previousToday);
      info.revert();
      toast.error("调整日程失败", { description: (e as Error).message });
    }
  }

  async function scheduleForToday(task: WorkTask) {
    const start = new Date();
    start.setHours(9, 0, 0, 0);
    const due = new Date(start);
    due.setHours(18, 0, 0, 0);
    setSchedulingTaskID(task.id);
    setUnscheduledTasks((items) => items.filter((item) => item.id !== task.id));
    try {
      await api.updateWorkTask(task.id, {
        start_at: start.toISOString(),
        due_at: due.toISOString(),
      });
      setNotice(`已将「${task.title}」安排到今天`);
      setReloadKey((value) => value + 1);
      void loadToday();
      emitWorkChanged();
    } catch (e) {
      setUnscheduledTasks((items) => [task, ...items].sort((a, b) => a.id - b.id));
      toast.error("安排任务失败", { description: (e as Error).message });
    } finally {
      setSchedulingTaskID(undefined);
    }
  }

  const scheduledToday = React.useMemo(() => {
    const seen = new Set<number>();
    return todayEvents
      .filter((event) => {
        if (event.source !== "work" || ["done", "canceled"].includes(event.status)) return false;
        if (seen.has(event.id)) return false;
        seen.add(event.id);
        return true;
      })
      .sort((left, right) => new Date(left.start).getTime() - new Date(right.start).getTime());
  }, [todayEvents]);
  const todayTaskCount = scheduledToday.length + unscheduledTasks.length;
  const visibleUnscheduled = unscheduledTasks.slice(0, 3);

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <WorkPageHeader title="日历" description="点击日期新建任务，点击日程查看详情，拖拽即可改期。">
        <Button
          variant="outline"
          size="sm"
          className="min-h-10 sm:min-h-7"
          onClick={() => setReloadKey((value) => value + 1)}
          disabled={loading || todayLoading}
        >
          <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
          刷新
        </Button>
      </WorkPageHeader>

      <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
        <Badge variant="secondary">个人待办</Badge>
        <span>月 / 周 / 日视图切换；新建、编辑和删除均在同一页面完成。</span>
        {loading && loadedOnce && (
          <span className="inline-flex items-center gap-1 text-xs">
            <Loader2 className="size-3.5 animate-spin" /> 正在加载当前视图
          </span>
        )}
      </div>
      <div className="rounded-xl border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
        日历只绘制已设置开始或截止时间的任务。未排期待办会显示在日历下方，可直接安排到今天。
      </div>

      {optionsError && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">
          <span>{optionsError}</span>
          <Button
            size="sm"
            className="min-h-10 sm:min-h-7"
            variant="outline"
            disabled={optionsLoading}
            onClick={() => void loadOptions()}
          >
            <RefreshCw className={optionsLoading ? "size-4 animate-spin" : "size-4"} />
            {optionsLoading ? "重试中" : "重试加载选项"}
          </Button>
        </div>
      )}

      {notice && (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-primary/30 bg-primary/10 px-3 py-2 text-sm text-primary">
          <span>{notice}</span>
          <Button
            size="icon-sm"
            className="size-10 sm:size-7"
            variant="ghost"
            onClick={() => setNotice(undefined)}
            aria-label="关闭提示"
          >
            <X className="size-4" />
          </Button>
        </div>
      )}

      <Card>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="flex items-center gap-2 font-semibold">
                <Clock3 className="size-4 text-primary" />
                今日待办
                <Badge variant="secondary">{todayTaskCount}</Badge>
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                汇总今天已排期的任务和没有设置时间的待办。点击任一项即可查看详情或开始安排。
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="min-h-10 sm:min-h-7"
              onClick={() => {
                setReloadKey((value) => value + 1);
                void loadUnscheduled();
              }}
              disabled={loading || todayLoading}
            >
              <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
              刷新待办
            </Button>
          </div>

          {todayTaskCount === 0 ? (
            <p className="rounded-xl border border-dashed px-3 py-5 text-sm text-muted-foreground">
              今天没有待办。可以点击日历中的日期，或在页面上方创建新任务。
            </p>
          ) : (
            <div className="grid gap-2 md:grid-cols-2">
              {scheduledToday.map((event) => (
                <button
                  key={`scheduled-${event.id}`}
                  type="button"
                  className="flex min-h-14 min-w-0 items-center gap-3 rounded-xl border bg-background px-3 py-2 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  onClick={() => void openEvent(event.id, "work")}
                  aria-label={`打开今日任务 ${event.title}`}
                >
                  <span className="h-9 w-1 shrink-0 rounded-full bg-primary" aria-hidden="true" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{event.title}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {formatTodayTime(event.start, event.end)}
                    </span>
                  </span>
                  <Badge variant="outline" className="shrink-0">
                    已排期
                  </Badge>
                </button>
              ))}
              {visibleUnscheduled.map((task) => (
                <div
                  key={`unscheduled-${task.id}`}
                  className="flex min-h-14 min-w-0 items-center gap-2 rounded-xl border border-dashed px-3 py-2"
                >
                  <button
                    type="button"
                    className="min-w-0 flex-1 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => setDetailTask(task)}
                    aria-label={`打开待安排任务 ${task.title}`}
                  >
                    <span className="block truncate text-sm font-medium">{task.title}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {task.project_name || "未归属项目"} · 未设置时间
                    </span>
                  </button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="min-h-10 shrink-0 sm:min-h-7"
                    disabled={schedulingTaskID === task.id}
                    onClick={() => void scheduleForToday(task)}
                  >
                    {schedulingTaskID === task.id ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <CalendarPlus className="size-4" />
                    )}
                    安排今天
                  </Button>
                </div>
              ))}
            </div>
          )}
          {todayError && (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700">
              <span>今日已排期任务刷新失败：{todayError}</span>
              <Button
                size="sm"
                variant="outline"
                className="min-h-10 sm:min-h-7"
                disabled={todayLoading}
                onClick={() => void loadToday()}
              >
                <RefreshCw className={todayLoading ? "size-4 animate-spin" : "size-4"} />
                重试
              </Button>
            </div>
          )}
          {unscheduledTasks.length > visibleUnscheduled.length && (
            <p className="text-xs text-muted-foreground">
              还有 {unscheduledTasks.length - visibleUnscheduled.length} 项未排期待办，可在下方完整列表继续处理。
            </p>
          )}
        </CardContent>
      </Card>

      {error && <WorkErrorState message={error} onRetry={() => setReloadKey((value) => value + 1)} />}

      {!loadedOnce && loading ? (
        <Skeleton className="h-[540px] w-full sm:h-[640px]" />
      ) : (
        <Card>
          <CardContent className="px-1 py-2 sm:px-4">
            <EventCalendarViews
              plugins={[dayGridPlugin, timeGridPlugin, interactionPlugin]}
              initialView="dayGridMonth"
              headerToolbar={{
                left: "prev,next today",
                center: "title",
                right: "dayGridMonth,timeGridWeek,timeGridDay",
              }}
              height={calendarHeight}
              events={events.map((event) => ({ ...event, id: String(event.id) }))}
              editable
              selectable
              datesSet={handleDatesSet}
              dateClick={(info) => openCreateAt(info.date, info.allDay)}
              eventClick={(info) => {
                void openEvent(info.event.id, info.event.extendedProps.source as string | undefined);
              }}
              eventDrop={(info) => void handleTimeChange(info as unknown as ChangedEventInfo)}
              eventResize={(info) => void handleTimeChange(info as unknown as ChangedEventInfo)}
              popoverCloseContent={() => <X className="size-3.5" />}
              eventClass={(arg: { event: { extendedProps: { source?: string } } }) =>
                arg.event.extendedProps.source === "artex" ? "opacity-90" : ""
              }
              noEventsContent="当前时间范围内没有日程"
            />
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <p className="flex items-center gap-2 font-medium">
                <Clock3 className="size-4 text-primary" />
                未排期任务
                <Badge variant="secondary">{unscheduledTasks.length}</Badge>
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                这些待办还没有开始或截止时间，因此不会出现在日历网格中。
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="min-h-10 sm:min-h-7"
              onClick={() => void loadUnscheduled()}
            >
              <RefreshCw className="size-4" />
              刷新未排期
            </Button>
          </div>

          {unscheduledError ? (
            <WorkErrorState message={unscheduledError} onRetry={() => void loadUnscheduled()} />
          ) : unscheduledTasks.length === 0 ? (
            <p className="rounded-lg border border-dashed px-3 py-4 text-sm text-muted-foreground">
              当前没有未排期任务。
            </p>
          ) : (
            <div className="grid gap-2 md:grid-cols-2">
              {unscheduledTasks.map((task) => (
                <div
                  key={task.id}
                  className="flex min-w-0 flex-col gap-2 rounded-xl border px-3 py-3 sm:flex-row sm:items-center"
                >
                  <button
                    type="button"
                    className="min-w-0 flex-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => setDetailTask(task)}
                    aria-label={`打开未排期任务 ${task.title}`}
                  >
                    <span className="block truncate text-sm font-medium">{task.title}</span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {task.project_name || "未归属项目"} · 打开可补充时间与提醒
                    </span>
                  </button>
                  <Button
                    size="sm"
                    className="min-h-10 sm:min-h-7"
                    variant="outline"
                    disabled={schedulingTaskID === task.id}
                    onClick={() => void scheduleForToday(task)}
                  >
                    {schedulingTaskID === task.id ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <CalendarPlus className="size-4" />
                    )}
                    安排今天
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

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
          setReloadKey((value) => value + 1);
        }}
        onDeleted={() => {
          setDetailTask(null);
          setReloadKey((value) => value + 1);
        }}
        onSubtasksChange={() => setReloadKey((value) => value + 1)}
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
          setNotice("任务已创建并加入自动执行队列");
          setReloadKey((value) => value + 1);
        }}
        onSubtasksChange={() => undefined}
      />
    </div>
  );
}

function defaultRange(): CalendarRange {
  const now = new Date();
  return {
    from: new Date(now.getFullYear(), now.getMonth(), 1),
    to: new Date(now.getFullYear(), now.getMonth() + 1, 1),
  };
}

function formatTodayTime(start: string, end: string) {
  const startDate = new Date(start);
  const endDate = new Date(end);
  if (Number.isNaN(startDate.getTime())) return "时间待确认";
  const startText = startDate.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
  if (Number.isNaN(endDate.getTime())) return startText;
  const endText = endDate.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
  return `${startText} - ${endText}`;
}
