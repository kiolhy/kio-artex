"use client";

import * as React from "react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import type { WorkReminderFeedEvent } from "@/lib/types";

const CURSOR_KEY = "artex_work_reminder_cursor";
const SEEN_KEY = "artex_work_reminder_seen";
const PERMISSION_EVENT = "artex:work-reminder-permission";
const POLL_INTERVAL_MS = 30_000;

function readCursor(): number | undefined {
  const value = Number(localStorage.getItem(CURSOR_KEY) ?? "");
  return Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function writeCursor(id: number) {
  localStorage.setItem(CURSOR_KEY, String(id));
}

function readSeen(): number[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(SEEN_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is number => Number.isSafeInteger(id)) : [];
  } catch {
    return [];
  }
}

function rememberSeen(id: number) {
  const ids = readSeen();
  if (ids.includes(id)) return;
  localStorage.setItem(SEEN_KEY, JSON.stringify([...ids, id].slice(-250)));
}

function openReminder(event: WorkReminderFeedEvent) {
  const url = event.snapshot.detail_url || "/work/today";
  window.location.assign(url);
}

function showReminder(event: WorkReminderFeedEvent) {
  const title = event.snapshot.title || (event.kind === "work_task_reminder_overdue" ? "待办已逾期" : "待办即将到期");
  const body = event.snapshot.body || "打开个人工作台查看详情。";

  toast(title, {
    description: body,
    action: {
      label: "查看",
      onClick: () => openReminder(event),
    },
  });

  if (typeof window === "undefined" || !("Notification" in window) || window.Notification.permission !== "granted") {
    return;
  }
  const native = new window.Notification(title, {
    body,
    tag: `artex-work-reminder-${event.id}`,
    icon: "/favicon.ico",
  });
  native.onclick = () => {
    window.focus();
    openReminder(event);
  };
}

export function useWorkReminderFeed(enabled: boolean) {
  React.useEffect(() => {
    if (!enabled || typeof window === "undefined") return;

    let cancelled = false;
    let running = false;
    let cursor = readCursor();

    const poll = async () => {
      if (cancelled || running || document.visibilityState === "hidden") return;
      running = true;
      try {
        const events = await api.workReminderFeed(cursor, 50);
        if (cancelled || events.length === 0) return;
        const latestID = Math.max(...events.map((event) => event.id));

        // The first successful request establishes a cursor. Events that were
        // waiting before the workbench opened are deliberately not replayed.
        if (cursor === undefined) {
          cursor = latestID;
          writeCursor(cursor);
          return;
        }

        const seen = new Set(readSeen());
        for (const event of events) {
          if (event.id <= cursor || seen.has(event.id)) continue;
          rememberSeen(event.id);
          showReminder(event);
        }
        cursor = latestID;
        writeCursor(cursor);
      } catch {
        // A transient feed failure must stay silent; the next tick retries.
      } finally {
        running = false;
      }
    };

    const onVisible = () => {
      if (document.visibilityState === "visible") void poll();
    };

    void poll();
    const timer = window.setInterval(() => void poll(), POLL_INTERVAL_MS);
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("focus", onVisible);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("focus", onVisible);
    };
  }, [enabled]);
}

export type BrowserNotificationState = NotificationPermission | "unsupported";

export function getBrowserNotificationState(): BrowserNotificationState {
  if (typeof window === "undefined" || !("Notification" in window)) return "unsupported";
  return window.Notification.permission;
}

export function useBrowserNotificationPermission() {
  const [state, setState] = React.useState<BrowserNotificationState>("default");

  React.useEffect(() => {
    const sync = () => setState(getBrowserNotificationState());
    sync();
    window.addEventListener(PERMISSION_EVENT, sync);
    return () => window.removeEventListener(PERMISSION_EVENT, sync);
  }, []);

  const request = React.useCallback(async () => {
    if (typeof window === "undefined" || !("Notification" in window)) {
      setState("unsupported");
      return "unsupported" as const;
    }
    const result = await window.Notification.requestPermission();
    setState(result);
    window.dispatchEvent(new Event(PERMISSION_EVENT));
    return result;
  }, []);

  return { state, request };
}
