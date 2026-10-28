"use client";

import { CalendarClock, Inbox, Sun } from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useWorkTasks } from "@/hooks/use-work-data";
import type { WorkTask } from "@/lib/types";

function WorkError({ message, onRetry }: { message?: string; onRetry: () => void }) {
  return (
    <div className="flex flex-col items-center gap-2 py-6 text-sm text-destructive">
      <span>{message ?? "个人待办加载失败"}</span>
      <Button variant="outline" size="xs" className="h-10 px-3 sm:h-6 sm:px-2" onClick={onRetry}>
        重试
      </Button>
    </div>
  );
}

export function WorkFocusCard({ refreshKey = 0 }: { refreshKey?: number }) {
  const today = useWorkTasks({ view: "today" }, refreshKey);
  const overdue = useWorkTasks({ view: "overdue" }, refreshKey);
  const upcoming = useWorkTasks({ view: "upcoming" }, refreshKey);
  const loading = today.loading || overdue.loading || upcoming.loading;
  const error = today.error ?? overdue.error ?? upcoming.error;

  function renderTask(task: WorkTask) {
    return (
      <Link
        key={task.id}
        href={`/work/inbox?task=${task.id}`}
        className="group flex items-center justify-between gap-3 rounded-lg border px-3 py-2 transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`查看任务：${task.title}`}
      >
        <div className="min-w-0">
          <p className="truncate text-sm font-medium group-hover:underline">{task.title}</p>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {task.due_at ? new Date(task.due_at).toLocaleString() : task.project_name || "未设时间"}
          </p>
        </div>
        <Badge variant="outline">{task.priority}</Badge>
      </Link>
    );
  }

  return (
    <Card>
      <CardHeader className="pb-0">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-semibold">今日焦点</p>
            <p className="text-xs text-muted-foreground">个人待办与逾期提醒</p>
          </div>
          <Link href="/work/today">
            <Button variant="ghost" size="xs" className="h-10 px-3 sm:h-6 sm:px-2">
              查看
            </Button>
          </Link>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <div className="space-y-2">
            <Skeleton className="h-11" />
            <Skeleton className="h-11" />
            <Skeleton className="h-11" />
          </div>
        ) : error ? (
          <WorkError message={error} onRetry={() => {
            today.reload();
            overdue.reload();
            upcoming.reload();
          }} />
        ) : today.data.length + overdue.data.length + upcoming.data.length === 0 ? (
          <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
            今天没有待办，去收件箱收集新任务。
          </div>
        ) : (
          <>
            {overdue.data.length > 0 && (
              <section className="space-y-2">
                <div className="flex items-center gap-2 text-xs font-medium text-red-700 dark:text-red-400">
                  <CalendarClock className="size-3.5" /> 逾期 {overdue.data.length}
                </div>
                {overdue.data.slice(0, 2).map(renderTask)}
              </section>
            )}
            <section className="space-y-2">
              <div className="flex items-center gap-2 text-xs font-medium text-amber-700 dark:text-amber-400">
                <Sun className="size-3.5" /> 今日 {today.data.length}
              </div>
              {today.data.slice(0, 3).map(renderTask)}
            </section>
            {upcoming.data.length > 0 && (
              <section className="space-y-2">
                <div className="flex items-center gap-2 text-xs font-medium text-blue-700 dark:text-blue-400">
                  <Inbox className="size-3.5" /> 即将到期 {upcoming.data.length}
                </div>
                {upcoming.data.slice(0, 2).map(renderTask)}
              </section>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
