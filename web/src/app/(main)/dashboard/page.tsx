"use client";

import * as React from "react";
import { RotateCcw, Save } from "lucide-react";
import { toast } from "sonner";

import { SidebarTrigger } from "@/components/ui/sidebar";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DashboardGrid } from "./_components/dashboard-grid";
import { BrowserReminderButton } from "./_components/browser-reminder-button";
import { useDashboardLayout } from "./_components/use-dashboard-layout";
import { WORK_CHANGED_EVENT } from "@/lib/work-events";

export default function DashboardPage() {
  const layout = useDashboardLayout();
  const [dirty, setDirty] = React.useState(false);
  const [isDesktop, setIsDesktop] = React.useState(false);
  const [isTablet, setIsTablet] = React.useState(false);
  const [refreshKey, setRefreshKey] = React.useState(0);

  React.useEffect(() => {
    const desktop = window.matchMedia("(min-width: 1280px)");
    const tablet = window.matchMedia("(min-width: 768px) and (max-width: 1279px)");
    const sync = () => {
      setIsDesktop(desktop.matches);
      setIsTablet(tablet.matches);
    };
    desktop.addEventListener("change", sync);
    tablet.addEventListener("change", sync);
    sync();
    return () => {
      desktop.removeEventListener("change", sync);
      tablet.removeEventListener("change", sync);
    };
  }, []);

  React.useEffect(() => {
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") setRefreshKey((value) => value + 1);
    };
    document.addEventListener("visibilitychange", refreshWhenVisible);
    window.addEventListener(WORK_CHANGED_EVENT, refreshWhenVisible);
    return () => {
      document.removeEventListener("visibilitychange", refreshWhenVisible);
      window.removeEventListener(WORK_CHANGED_EVENT, refreshWhenVisible);
    };
  }, []);

  function updateCards(cards: Parameters<typeof layout.update>[0]) {
    layout.update(cards);
    setDirty(true);
  }

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <div className="flex flex-col gap-3 border-b pb-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-start gap-3">
          <SidebarTrigger className="mt-1 lg:hidden" />
          <div>
            <h1 className="text-xl font-semibold tracking-tight">个人工作台</h1>
            <p className="text-sm text-muted-foreground">个人待办、项目、日历、笔记与自动化执行统一编排。</p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <BrowserReminderButton />
          <Button
            variant="outline"
            size="sm"
            className="h-10 sm:h-7"
            onClick={async () => {
              if (await layout.save(layout.layout.cards)) {
                setDirty(false);
                toast.success("工作台布局已保存");
              } else {
                toast.error("工作台布局保存失败，请重试");
              }
            }}
            disabled={!dirty || layout.saving}
          >
            <Save className="size-4" />
            {layout.saving ? "保存中" : "保存布局"}
          </Button>
          <Button variant="outline" size="sm" className="h-10 sm:h-7" onClick={layout.reload}>
            <RotateCcw className="size-4" />
            重新加载
          </Button>
        </div>
      </div>

      {layout.error && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          <span>布局加载失败：{layout.error}</span>
          <Button variant="outline" size="sm" className="h-10 sm:h-7" onClick={layout.reload}>
            重试
          </Button>
        </div>
      )}

      {layout.loading ? (
        <div className="space-y-3" aria-busy="true" aria-label="正在加载工作台布局">
          <Skeleton className="h-5 w-72" />
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Skeleton className="h-64" />
            <Skeleton className="h-64" />
            <Skeleton className="h-64 sm:col-span-2" />
            <Skeleton className="h-64" />
          </div>
        </div>
      ) : (
        <DashboardGrid
          cards={layout.layout.cards}
          onChange={updateCards}
          isTablet={isTablet}
          isDesktop={isDesktop}
          refreshKey={refreshKey}
          onTaskCreated={() => setRefreshKey((value) => value + 1)}
        />
      )}
    </div>
  );
}
