"use client";

import * as React from "react";

import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { api } from "@/lib/api";

import { CardEmpty, CardError, CardLoading } from "./dashboard-states";

type CalendarEvent = Awaited<ReturnType<typeof api.workCalendarEvents>>[number];

function eventHref(event: CalendarEvent) {
  if (event.source === "artex") {
    return `/function/tasks/detail?id=${encodeURIComponent(String(event.id))}`;
  }
  return `/work/inbox?task=${encodeURIComponent(String(event.id))}`;
}

function eventDate(event: CalendarEvent) {
  const date = new Date(event.start);
  const day = new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
  }).format(date);
  if (event.allDay) return day;
  const time = new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
  return `${day} ${time}`;
}

export function WorkCalendarCard({ refreshKey = 0 }: { refreshKey?: number }) {
  const [events, setEvents] = React.useState<CalendarEvent[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const requestKey = `${refreshKey}:${reloadKey}`;

  // biome-ignore lint/correctness/useExhaustiveDependencies: requestKey intentionally retriggers the refresh.
  React.useEffect(() => {
    let alive = true;
    void (async () => {
      setLoading(true);
      try {
        const now = new Date();
        const from = new Date(now.getFullYear(), now.getMonth(), now.getDate());
        const to = new Date(from);
        to.setDate(to.getDate() + 7);
        const result = await api.workCalendarEvents(from.toISOString(), to.toISOString());
        if (!alive) return;
        setEvents(result.filter((event) => event.status !== "done" && event.status !== "canceled"));
        setError(undefined);
      } catch (e) {
        if (alive) setError((e as Error).message);
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [requestKey]);

  let content: React.ReactNode;
  if (loading) {
    content = <CardLoading rows={4} />;
  } else if (error) {
    content = <CardError message={error} onRetry={() => setReloadKey((value) => value + 1)} />;
  } else if (events.length === 0) {
    content = <CardEmpty title="未来 7 天没有安排" description="到日历页拖拽或编辑任务时间。" />;
  } else {
    content = events.slice(0, 6).map((event) => (
      <Link
        key={`${event.resourceId}:${event.id}`}
        href={eventHref(event)}
        className="flex min-h-12 items-center justify-between gap-3 rounded-lg border px-3 py-2 transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`查看${event.source === "artex" ? "执行" : "个人"}任务：${event.title}`}
      >
        <div className="min-w-0">
          <p className="truncate font-medium text-sm">{event.title}</p>
          <p className="text-muted-foreground text-xs">{eventDate(event)}</p>
        </div>
        <Badge variant="outline">{event.source === "artex" ? "ARTEX" : "个人"}</Badge>
      </Link>
    ));
  }

  return (
    <Card className="h-full">
      <CardHeader className="pb-0">
        <div className="flex items-center justify-between">
          <div>
            <p className="font-semibold text-sm">日历</p>
            <p className="text-muted-foreground text-xs">未来 7 天的任务与执行安排</p>
          </div>
          <Button variant="ghost" size="xs" className="h-10 px-3 sm:h-6 sm:px-2" asChild>
            <Link href="/work/calendar">查看</Link>
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">{content}</CardContent>
    </Card>
  );
}
