"use client";

import * as React from "react";

import { SidebarTrigger } from "@/components/ui/sidebar";

export function WorkPageHeader({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 border-b pb-4 lg:flex-row lg:items-center lg:justify-between">
      <div className="flex items-start gap-3">
        <SidebarTrigger className="mt-1 lg:hidden" />
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
      </div>
      {children && <div className="flex flex-wrap items-center gap-2">{children}</div>}
    </div>
  );
}

export function WorkErrorState({ message, onRetry }: { message?: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-destructive/30 bg-destructive/5 px-6 py-8 text-sm text-destructive">
      <span>{message ?? "加载失败"}</span>
      {onRetry && (
        <button type="button" className="text-xs underline underline-offset-4" onClick={onRetry}>
          重试
        </button>
      )}
    </div>
  );
}

export function WorkEmptyState({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-1 rounded-xl border border-dashed px-6 py-10 text-center">
      <p className="text-sm font-medium">{title}</p>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      {children && <div className="mt-2">{children}</div>}
    </div>
  );
}
