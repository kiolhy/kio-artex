"use client";

import type * as React from "react";
import { AlertTriangle, RotateCcw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export function CardLoading({ rows = 3, className }: { rows?: number; className?: string }) {
  return (
    <div className={cn("space-y-2", className)} aria-busy="true" aria-live="polite">
      {Array.from({ length: rows }, (_, index) => index).map((row) => (
        <Skeleton key={row} className="h-11 w-full" />
      ))}
    </div>
  );
}

export function CardError({ message, onRetry }: { message?: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-6 text-sm text-destructive">
      <AlertTriangle className="size-4" />
      <span>{message ?? "加载失败"}</span>
      {onRetry && (
        <Button
          variant="outline"
          size="xs"
          className="h-10 px-3 sm:h-6 sm:px-2"
          onClick={onRetry}
        >
          <RotateCcw className="size-3.5" />
          重试
        </Button>
      )}
    </div>
  );
}

export function CardEmpty({ title, description }: { title: string; description?: string }) {
  return (
    <div className="rounded-lg border border-dashed px-4 py-8 text-center">
      <p className="text-sm font-medium">{title}</p>
      {description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}
    </div>
  );
}

export function DashboardCardShell({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <Card className={cn("min-h-0", className)}>
      <CardContent className="h-full">{children}</CardContent>
    </Card>
  );
}
