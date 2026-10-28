"use client";

import * as React from "react";
import {
  AlertTriangle,
  ArrowDown,
  ChevronDown,
  ChevronRight,
  CircleDot,
  Clock3,
  Loader2,
  RefreshCw,
  Terminal,
  Wifi,
  WifiOff,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { api, sseUrl } from "@/lib/api";
import type { Activity } from "@/lib/types";
import { cn } from "@/lib/utils";

type ConnectionState = "idle" | "connecting" | "live" | "retrying" | "offline";

type ActivityDetailState = {
  detail?: string;
  error?: string;
  loading: boolean;
};

const HISTORY_LIMIT = 200;
const MAX_ACTIVITY_ITEMS = 500;
const MAX_RECONNECT_ATTEMPTS = 3;

export function WorkTaskActivity({
  taskId,
  live,
  defaultOpen = false,
}: {
  taskId: string;
  live: boolean;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = React.useState(defaultOpen);
  const [activities, setActivities] = React.useState<Activity[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState<string>();
  const [connection, setConnection] = React.useState<ConnectionState>("idle");
  const [reloadKey, setReloadKey] = React.useState(0);
  const [expandedSeq, setExpandedSeq] = React.useState<number | null>(null);
  const [details, setDetails] = React.useState<Record<number, ActivityDetailState>>({});
  const [showJumpToBottom, setShowJumpToBottom] = React.useState(false);
  const cursorRef = React.useRef(0);
  const viewportRef = React.useRef<HTMLDivElement | null>(null);
  const atBottomRef = React.useRef(true);
  const activityKeyRef = React.useRef(taskId);
  const userToggledRef = React.useRef(false);

  React.useEffect(() => {
    const taskChanged = activityKeyRef.current !== taskId;
    if (taskChanged) {
      activityKeyRef.current = taskId;
      userToggledRef.current = false;
      setOpen(defaultOpen);
      return;
    }
    if (!userToggledRef.current) setOpen(defaultOpen);
  }, [defaultOpen, taskId]);

  React.useEffect(() => {
    setActivities([]);
    setDetails({});
    setExpandedSeq(null);
    setError(undefined);
    setConnection("idle");
    cursorRef.current = 0;
    atBottomRef.current = true;
    setShowJumpToBottom(false);
  }, [taskId]);

  React.useEffect(() => {
    if (!open || !taskId) return;

    let alive = true;
    let source: EventSource | null = null;
    let retryTimer: number | undefined;
    let reconnectAttempts = 0;

    const closeSource = () => {
      source?.close();
      source = null;
    };

    const connect = (since: number) => {
      if (!alive || !live) return;
      closeSource();
      setConnection(reconnectAttempts > 0 ? "retrying" : "connecting");
      const query = new URLSearchParams({ task: taskId, since: String(since) });
      source = new EventSource(sseUrl(`/api/exploration/activity/stream?${query.toString()}`));
      source.onopen = () => {
        reconnectAttempts = 0;
        if (alive) setConnection("live");
      };
      source.onmessage = (event) => {
        const activity = parseActivity(event.data);
        if (!activity) return;
        cursorRef.current = Math.max(cursorRef.current, activity.seq);
        setActivities((current) => mergeActivities(current, [activity]));
      };
      source.onerror = () => {
        if (!alive) return;
        closeSource();
        if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
          setConnection("offline");
          setError("实时执行流已断开，历史记录仍可查看。可手动重连。");
          return;
        }
        const delay = 750 * 2 ** reconnectAttempts;
        reconnectAttempts += 1;
        setConnection("retrying");
        retryTimer = window.setTimeout(() => connect(cursorRef.current), delay);
      };
    };

    setLoading(true);
    setError(undefined);
    setConnection(live ? "connecting" : "offline");
    void api
      .activity(taskId, { limit: HISTORY_LIMIT })
      .then((result) => {
        if (!alive) return;
        setActivities((current) => {
          const merged = mergeActivities(current, result.items);
          cursorRef.current = Math.max(
            result.cursor,
            merged.length > 0 ? merged[merged.length - 1].seq : 0,
          );
          return merged;
        });
        if (live) connect(cursorRef.current);
        else setConnection("offline");
      })
      .catch((cause: Error) => {
        if (!alive) return;
        setError(cause.message || "执行过程加载失败");
        setConnection("offline");
      })
      .finally(() => {
        if (alive) setLoading(false);
      });

    return () => {
      alive = false;
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
      closeSource();
    };
  }, [live, open, reloadKey, taskId]);

  React.useEffect(() => {
    if (!open || !atBottomRef.current) return;
    const viewport = viewportRef.current;
    if (!viewport) return;
    const frame = window.requestAnimationFrame(() => {
      viewport.scrollTop = viewport.scrollHeight;
    });
    return () => window.cancelAnimationFrame(frame);
  }, [activities, open]);

  function handleScroll() {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const distance = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight;
    const atBottom = distance < 32;
    atBottomRef.current = atBottom;
    setShowJumpToBottom(!atBottom);
  }

  function jumpToBottom() {
    const viewport = viewportRef.current;
    if (!viewport) return;
    atBottomRef.current = true;
    setShowJumpToBottom(false);
    viewport.scrollTo({ top: viewport.scrollHeight, behavior: "smooth" });
  }

  function toggleDetail(activity: Activity) {
    if (expandedSeq === activity.seq) {
      setExpandedSeq(null);
      return;
    }
    setExpandedSeq(activity.seq);
    if (details[activity.seq]?.detail || details[activity.seq]?.loading) return;
    setDetails((current) => ({
      ...current,
      [activity.seq]: { loading: true },
    }));
    void api
      .activityDetail(activity.seq, taskId)
      .then((result) => {
        setDetails((current) => ({
          ...current,
          [activity.seq]: { detail: result.detail, loading: false },
        }));
      })
      .catch((cause: Error) => {
        setDetails((current) => ({
          ...current,
          [activity.seq]: { error: cause.message || "详情加载失败", loading: false },
        }));
      });
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={(next) => {
        userToggledRef.current = true;
        setOpen(next);
      }}
    >
      <section className="rounded-lg border">
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex w-full items-center gap-2 rounded-lg px-3 py-3 text-left hover:bg-muted/50"
            aria-label={open ? "收起多 Agent 执行过程" : "展开多 Agent 执行过程"}
          >
            {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
            <Terminal className="size-4 text-primary" />
            <span className="min-w-0 flex-1 text-sm font-medium">执行过程</span>
            <ConnectionBadge state={connection} live={live} />
            {activities.length > 0 && (
              <Badge variant="secondary" className="whitespace-nowrap">
                {activities.length} 条
              </Badge>
            )}
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="space-y-3 border-t px-3 py-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs text-muted-foreground">
                goals 拆解目标，planner 制定计划，worker 并行执行；这里展示关联执行任务的实时活动。
              </p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setReloadKey((value) => value + 1)}
                disabled={loading}
              >
                <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
                重连
              </Button>
            </div>

            {error && (
              <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="relative">
              <div
                ref={viewportRef}
                onScroll={handleScroll}
                className="max-h-80 min-h-24 overflow-y-auto overscroll-contain rounded-md border bg-muted/20 p-2"
              >
                {loading && activities.length === 0 ? (
                  <div className="flex min-h-20 items-center justify-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" />
                    正在读取执行历史...
                  </div>
                ) : activities.length === 0 ? (
                  <div className="flex min-h-20 items-center justify-center px-4 text-center text-sm text-muted-foreground">
                    {live ? "执行任务已启动，等待 planner 或 worker 产出第一条活动。" : "该执行任务还没有可展示的活动。"}
                  </div>
                ) : (
                  <ol className="space-y-2" aria-label="多 Agent 执行时间线">
                    {activities.map((activity) => {
                      const expanded = expandedSeq === activity.seq;
                      const detail = details[activity.seq];
                      return (
                        <li key={`${taskId}:${activity.seq}`}>
                          <button
                            type="button"
                            onClick={() => toggleDetail(activity)}
                            className={cn(
                              "w-full rounded-md border bg-background/80 px-3 py-2 text-left transition-colors hover:bg-muted/60",
                              activity.is_error && "border-destructive/30",
                            )}
                            aria-expanded={expanded}
                          >
                            <div className="flex min-w-0 items-start gap-2">
                              <CircleDot
                                className={cn(
                                  "mt-0.5 size-3.5 shrink-0",
                                  activity.is_error ? "text-destructive" : "text-primary",
                                )}
                              />
                              <div className="min-w-0 flex-1 space-y-1">
                                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                                  <span className="font-medium text-foreground">{activity.worker || "system"}</span>
                                  <Badge variant="outline" className="px-1.5 text-xs">
                                    {activity.kind}
                                  </Badge>
                                  {activity.tool && <span className="font-mono">{activity.tool}</span>}
                                  <span className="inline-flex items-center gap-1">
                                    <Clock3 className="size-3" />
                                    {formatActivityTime(activity.ts)}
                                  </span>
                                  <span>#{activity.seq}</span>
                                </div>
                                <p
                                  className={cn(
                                    "break-words text-sm",
                                    activity.is_error && "text-destructive",
                                  )}
                                >
                                  {activity.summary || "无摘要"}
                                </p>
                                <UsageBadges activity={activity} />
                              </div>
                              {expanded ? (
                                <ChevronDown className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                              ) : (
                                <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                              )}
                            </div>
                          </button>
                          {expanded && (
                            <div className="mx-2 rounded-b-md border border-t-0 bg-background px-3 py-2">
                              {detail?.loading ? (
                                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                                  <Loader2 className="size-3.5 animate-spin" />
                                  正在加载完整活动详情...
                                </div>
                              ) : detail?.error ? (
                                <p className="text-xs text-destructive">{detail.error}</p>
                              ) : (
                                <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">
                                  {detail?.detail || activity.detail || activity.summary || "没有更多详情。"}
                                </pre>
                              )}
                            </div>
                          )}
                        </li>
                      );
                    })}
                  </ol>
                )}
              </div>
              {showJumpToBottom && (
                <Button
                  type="button"
                  size="sm"
                  className="absolute right-3 bottom-3 shadow-md"
                  onClick={jumpToBottom}
                >
                  <ArrowDown className="size-4" />
                  回到最新
                </Button>
              )}
            </div>
          </div>
        </CollapsibleContent>
      </section>
    </Collapsible>
  );
}

function ConnectionBadge({ state, live }: { state: ConnectionState; live: boolean }) {
  if (!live) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
        <WifiOff className="size-3.5" />
        仅历史
      </span>
    );
  }
  const meta: Record<Exclude<ConnectionState, "offline">, { label: string; className: string }> = {
    idle: { label: "待连接", className: "text-muted-foreground" },
    connecting: { label: "连接中", className: "text-amber-600" },
    live: { label: "实时", className: "text-emerald-600" },
    retrying: { label: "重连中", className: "text-amber-600" },
  };
  if (state === "offline") {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-destructive">
        <WifiOff className="size-3.5" />
        已断开
      </span>
    );
  }
  const item = meta[state];
  return (
    <span className={cn("inline-flex items-center gap-1 text-xs", item.className)}>
      {state === "connecting" || state === "retrying" ? (
        <Loader2 className="size-3.5 animate-spin" />
      ) : (
        <Wifi className="size-3.5" />
      )}
      {item.label}
    </span>
  );
}

function UsageBadges({ activity }: { activity: Activity }) {
  const usage = [
    activity.input_tokens !== undefined ? `输入 ${formatNumber(activity.input_tokens)}` : "",
    activity.output_tokens !== undefined ? `输出 ${formatNumber(activity.output_tokens)}` : "",
    activity.cache_read_tokens !== undefined ? `缓存读 ${formatNumber(activity.cache_read_tokens)}` : "",
    activity.cache_write_tokens !== undefined ? `缓存写 ${formatNumber(activity.cache_write_tokens)}` : "",
  ].filter(Boolean);
  if (usage.length === 0) return null;
  return <p className="text-xs text-muted-foreground">{usage.join(" · ")} Token</p>;
}

function parseActivity(data: string): Activity | null {
  try {
    const value = JSON.parse(data) as Partial<Activity>;
    if (
      typeof value !== "object" ||
      value === null ||
      typeof value.seq !== "number" ||
      typeof value.worker !== "string" ||
      typeof value.kind !== "string" ||
      typeof value.summary !== "string"
    ) {
      return null;
    }
    return value as Activity;
  } catch {
    return null;
  }
}

function mergeActivities(current: Activity[], incoming: Activity[]) {
  const bySeq = new Map<number, Activity>();
  for (const item of current) bySeq.set(item.seq, item);
  for (const item of incoming) bySeq.set(item.seq, item);
  return Array.from(bySeq.values())
    .sort((left, right) => left.seq - right.seq)
    .slice(-MAX_ACTIVITY_ITEMS);
}

function formatActivityTime(timestamp: string) {
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return "时间未知";
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}
