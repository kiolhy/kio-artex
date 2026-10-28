"use client";

import * as React from "react";
import { AlertCircleIcon, Loader2Icon, RefreshCwIcon } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { sseUrl } from "@/lib/api";
import { MOCK } from "@/lib/mock/enabled";
import type { LogLine } from "@/lib/types";
import { cn } from "@/lib/utils";

// Mock demo：无后端 SSE，塞几行示例日志。
const MOCK_LOGS: LogLine[] = [
  { seq: 1, ts: "2026-07-26T03:55:00Z", level: "info", tag: "engine", text: "ARTEX v0.1.0 backend listening on :8787 (workers=3)" },
  { seq: 2, ts: "2026-07-26T03:55:01Z", level: "info", tag: "config", text: "LLM configured from DB: anthropic / claude-opus-4-8" },
  { seq: 3, ts: "2026-07-26T03:56:10Z", level: "info", tag: "planner", text: "task #12: 第 3 轮规划，生成执行意图 i-4" },
  { seq: 4, ts: "2026-07-26T03:57:00Z", level: "warn", tag: "policy", text: "block bash: 操作超出当前任务允许范围" },
  { seq: 5, ts: "2026-07-26T03:57:30Z", level: "info", tag: "work#1", text: "summary: 今日工作汇总已生成并写入通知" },
  { seq: 6, ts: "2026-07-26T03:58:20Z", level: "error", tag: "work#3", text: "approval: 高风险文件操作等待人工确认" },
];

const levelTone: Record<LogLine["level"], string> = {
  info: "text-foreground/80",
  warn: "text-amber-600 dark:text-amber-400",
  error: "text-red-600 dark:text-red-400",
};
const levelDot: Record<LogLine["level"], string> = {
  info: "bg-muted-foreground/40",
  warn: "bg-amber-500",
  error: "bg-red-500",
};

function fmtTime(ts: string) {
  const d = new Date(ts);
  if (isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

export default function LogsPage() {
  const [lines, setLines] = React.useState<LogLine[]>([]);
  const [q, setQ] = React.useState("");
  const [level, setLevel] = React.useState<"all" | LogLine["level"]>("all");
  const [paused, setPaused] = React.useState(false);
  const [loadingHistory, setLoadingHistory] = React.useState(false);
  const [historyError, setHistoryError] = React.useState("");
  const [hasMore, setHasMore] = React.useState(false);
  const [connection, setConnection] = React.useState<"connecting" | "connected" | "disconnected">(
    MOCK ? "connected" : "connecting",
  );
  const [streamKey, setStreamKey] = React.useState(0);
  const bottom = React.useRef<HTMLDivElement>(null);
  const stick = React.useRef(true);
  const pausedRef = React.useRef(false);
  pausedRef.current = paused;

  // Minimum db_id seen — used as the cursor for loading older history.
  const minDbId = React.useMemo(() => {
    let min = 0;
    for (const l of lines) {
      if (l.db_id && (min === 0 || l.db_id < min)) min = l.db_id;
    }
    return min;
  }, [lines]);

  // Live tail via SSE. on connect, replays whatever is in the ring (includes
  // DB-restored history from the last 100 rows written before restart).
  React.useEffect(() => {
    if (MOCK) {
      setLines(MOCK_LOGS);
      setConnection("connected");
      return;
    }
    setConnection("connecting");
    const es = new EventSource(sseUrl("/api/logs/stream?since=0"));
    es.onopen = () => setConnection("connected");
    es.onmessage = (e) => {
      setConnection("connected");
      if (pausedRef.current) return;
      try {
        const l = JSON.parse(e.data) as LogLine;
        setLines((prev) => (prev.some((x) => x.seq === l.seq) ? prev : [...prev, l].slice(-3000)));
      } catch {
        /* ignore malformed frame */
      }
    };
    es.onerror = () => setConnection("disconnected");
    return () => es.close();
  }, [streamKey]);

  // Mark "has more" once we have any db_id in view.
  React.useEffect(() => {
    if (minDbId > 1) setHasMore(true);
  }, [minDbId]);

  async function loadOlderHistory() {
    if (loadingHistory) return;
    setLoadingHistory(true);
    setHistoryError("");
    try {
      const params = minDbId > 0 ? `?before=${minDbId}&limit=200` : `?limit=200`;
      const res = await fetch(`/api/logs/history${params}`);
      if (!res.ok) {
        throw new Error(`读取历史日志失败（HTTP ${res.status}）`);
      }
      const data = await res.json() as { items: LogLine[]; has_more: boolean };
      if (data.items?.length) {
        // Assign synthetic seq numbers below current minimum to keep dedup working.
        setLines((prev) => {
          const minSeq = prev.reduce((m, l) => Math.min(m, l.seq ?? 0), 0);
          const older = data.items.map((l, i) => ({
            ...l,
            seq: minSeq - data.items.length + i,
          }));
          // Prepend, dedup by db_id, cap at 5000.
          const merged = [...older, ...prev];
          const seen = new Set<number>();
          const deduped = merged.filter((l) => {
            if (!l.db_id) return true;
            if (seen.has(l.db_id)) return false;
            seen.add(l.db_id);
            return true;
          });
          return deduped.slice(-5000);
        });
      }
      setHasMore(data.has_more ?? false);
    } catch (e) {
      setHistoryError((e as Error).message || "读取历史日志失败");
    } finally {
      setLoadingHistory(false);
    }
  }

  const filtered = React.useMemo(() => {
    const needle = q.trim().toLowerCase();
    return lines.filter(
      (l) =>
        (level === "all" || l.level === level) &&
        (!needle || l.text.toLowerCase().includes(needle) || l.tag.toLowerCase().includes(needle)),
    );
  }, [lines, q, level]);

  React.useEffect(() => {
    if (stick.current && !paused) bottom.current?.scrollIntoView();
  }, [filtered, paused]);

  const counts = React.useMemo(() => {
    let warn = 0,
      error = 0;
    for (const l of lines) {
      if (l.level === "warn") warn++;
      else if (l.level === "error") error++;
    }
    return { warn, error, total: lines.length };
  }, [lines]);

  return (
    <div className="flex flex-1 flex-col gap-3">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">系统日志</h1>
        <p className="text-muted-foreground text-sm">
          后端实时日志流（Agent / 规划器 / Worker / 数据库 / 通知）
        </p>
      </div>
      <div className="flex flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <Input
            placeholder="过滤(文本 / tag)…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            className="h-8 max-w-xs"
          />
          <div className="flex gap-1">
            {(["all", "info", "warn", "error"] as const).map((lv) => (
              <Button
                key={lv}
                size="sm"
                variant={level === lv ? "default" : "outline"}
                className="h-8"
                onClick={() => setLevel(lv)}
              >
                {lv === "all" ? "全部" : lv}
              </Button>
            ))}
          </div>
          <Button
            size="sm"
            variant={paused ? "default" : "outline"}
            className="h-8"
            onClick={() => setPaused((p) => !p)}
          >
            {paused ? "已暂停" : "暂停"}
          </Button>
          <Button size="sm" variant="outline" className="h-8" onClick={() => setLines([])}>
            清空
          </Button>
          <div
            className={cn(
              "flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs",
              connection === "connected" && "border-emerald-500/40 text-emerald-700",
              connection === "connecting" && "border-sky-500/40 text-sky-700",
              connection === "disconnected" && "border-destructive/40 text-destructive",
            )}
            role="status"
            aria-live="polite"
          >
            {connection === "connecting" && <Loader2Icon className="size-3.5 animate-spin" />}
            {connection === "connected" && (
              <span className="size-2 rounded-full bg-emerald-500" aria-hidden="true" />
            )}
            {connection === "disconnected" && <AlertCircleIcon className="size-3.5" />}
            {connection === "connected"
              ? "已连接"
              : connection === "connecting"
                ? "连接中"
                : "连接已断开"}
          </div>
          {connection === "disconnected" && (
            <Button
              size="sm"
              variant="outline"
              className="h-8"
              onClick={() => {
                setConnection("connecting");
                setStreamKey((v) => v + 1);
              }}
            >
              <RefreshCwIcon /> 重新连接
            </Button>
          )}
          <span className="ml-auto text-xs text-muted-foreground">
            {counts.total} 行 ·{" "}
            <span className="text-amber-600 dark:text-amber-400">{counts.warn} 警告</span> ·{" "}
            <span className="text-red-600 dark:text-red-400">{counts.error} 错误</span>
          </span>
        </div>

        {historyError && (
          <div className="border-destructive/40 bg-destructive/5 flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2">
            <AlertCircleIcon className="text-destructive size-4 shrink-0" />
            <p className="min-w-0 flex-1 text-sm">{historyError}</p>
            <Button size="sm" variant="outline" onClick={() => void loadOlderHistory()}>
              <RefreshCwIcon /> 重试
            </Button>
          </div>
        )}

        <div
          onScroll={(e) => {
            const el = e.currentTarget;
            stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
          }}
          className="h-[calc(100vh-14rem)] overflow-auto rounded-lg border bg-card p-2 font-mono text-xs leading-relaxed"
        >
          {hasMore && (
            <div className="flex justify-center py-2">
              <Button
                size="sm"
                variant="outline"
                className="h-7 text-xs"
                disabled={loadingHistory}
                onClick={loadOlderHistory}
              >
                {loadingHistory ? "加载中…" : "加载更早日志"}
              </Button>
            </div>
          )}
          {filtered.length === 0 ? (
            <p className="py-10 text-center text-muted-foreground">
              {connection === "disconnected" ? "日志流已断开，请重新连接。" : "尚未收到日志。"}
            </p>
          ) : (
            filtered.map((l) => {
              const body =
                l.tag && l.text.startsWith("[" + l.tag + "]")
                  ? l.text.slice(l.tag.length + 2).trimStart()
                  : l.text;
              return (
                <div key={l.seq} className="flex items-start gap-2 px-1 py-0.5 hover:bg-muted/40">
                  <span className={cn("mt-1.5 size-1.5 shrink-0 rounded-full", levelDot[l.level])} />
                  <span className="shrink-0 tabular-nums text-muted-foreground">{fmtTime(l.ts)}</span>
                  {l.tag && (
                    <span className="shrink-0 rounded bg-muted px-1 text-xs text-foreground/70">{l.tag}</span>
                  )}
                  <span className={cn("min-w-0 whitespace-pre-wrap break-words", levelTone[l.level])}>{body}</span>
                </div>
              );
            })
          )}
          <div ref={bottom} />
        </div>
      </div>
    </div>
  );
}
