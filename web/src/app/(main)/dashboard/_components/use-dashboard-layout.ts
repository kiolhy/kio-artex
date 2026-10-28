"use client";

import * as React from "react";

import { api } from "@/lib/api";
import type { WorkDashboardCardLayout, WorkDashboardLayout } from "@/lib/types";

export type DashboardCardType =
  | "personal-focus"
  | "personal-quick-capture"
  | "personal-calendar"
  | "personal-automation"
  | "personal-assets";

export const DASHBOARD_CARD_LABELS: Record<DashboardCardType, string> = {
  "personal-focus": "今日焦点",
  "personal-quick-capture": "快捷记录",
  "personal-calendar": "日历",
  "personal-automation": "个人自动化执行",
  "personal-assets": "资料资产",
};

export const DEFAULT_DASHBOARD_LAYOUT: WorkDashboardCardLayout[] = [
  { type: "personal-focus", visible: true, span: 1 },
  { type: "personal-quick-capture", visible: true, span: 1 },
  { type: "personal-calendar", visible: true, span: 2 },
  { type: "personal-automation", visible: true, span: 1 },
  { type: "personal-assets", visible: true, span: 1 },
];

function normalizeSpan(value: unknown, fallback: WorkDashboardCardLayout["span"]) {
  const span = Number(value);
  if (!Number.isFinite(span)) return fallback;
  return Math.min(4, Math.max(1, Math.round(span))) as WorkDashboardCardLayout["span"];
}

export function normalizeDashboardLayout(input?: WorkDashboardLayout | null): WorkDashboardLayout {
  const cards = Array.isArray(input?.cards) ? input.cards : [];
  const defaults = new Map(DEFAULT_DASHBOARD_LAYOUT.map((card) => [card.type, card]));
  const seen = new Set<string>();
  // Preserve the persisted order. Only append cards introduced by a newer
  // release so an existing drag-and-drop layout is not reset on every load.
  const normalized = cards
    .filter((card) => defaults.has(card.type) && !seen.has(card.type) && seen.add(card.type))
    .map((card) => {
      const fallback = defaults.get(card.type)!;
      return {
        ...fallback,
        ...card,
        span: normalizeSpan(card.span, fallback.span),
        visible: typeof card.visible === "boolean" ? card.visible : fallback.visible,
      };
    });
  for (const fallback of DEFAULT_DASHBOARD_LAYOUT) {
    if (!seen.has(fallback.type)) normalized.push(fallback);
  }
  return { version: input?.version ?? 1, cards: normalized };
}

export function useDashboardLayout() {
  const [layout, setLayout] = React.useState<WorkDashboardLayout>({
    version: 1,
    cards: DEFAULT_DASHBOARD_LAYOUT,
  });
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);

  React.useEffect(() => {
    let alive = true;
    void (async () => {
      setLoading(true);
      try {
        const result = await api.workDashboardLayout();
        if (!alive) return;
        setLayout(normalizeDashboardLayout(result.layout));
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
  }, [reloadKey]);

  const update = React.useCallback((next: WorkDashboardCardLayout[]) => {
    setLayout((current) => ({ ...current, cards: next }));
  }, []);

  const save = React.useCallback(async (next: WorkDashboardCardLayout[]) => {
    setSaving(true);
    setError(undefined);
    try {
      await api.saveWorkDashboardLayout({ version: 1, cards: next });
      return true;
    } catch (e) {
      setError((e as Error).message);
      return false;
    } finally {
      setSaving(false);
    }
  }, []);

  const reload = React.useCallback(() => setReloadKey((value) => value + 1), []);

  return { layout, loading, saving, error, update, save, reload };
}
