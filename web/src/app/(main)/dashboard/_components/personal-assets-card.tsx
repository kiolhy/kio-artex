"use client";

import * as React from "react";
import {
  ArrowRight,
  File,
  FileImage,
  FileSpreadsheet,
  FileText,
  Globe2,
  Loader2,
  RefreshCw,
  TriangleAlert,
} from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { WorkAsset, WorkAssetStats } from "@/lib/types";
import { cn } from "@/lib/utils";

const emptyStats: WorkAssetStats = { total: 0, processing: 0, failed: 0, trashed: 0 };

function assetIcon(type: WorkAsset["asset_type"]) {
  if (type === "image") return <FileImage className="size-4" />;
  if (type === "table") return <FileSpreadsheet className="size-4" />;
  if (type === "link" || type === "api") return <Globe2 className="size-4" />;
  return <FileText className="size-4" />;
}

export function PersonalAssetsCard({ refreshKey = 0 }: { refreshKey?: number }) {
  const [stats, setStats] = React.useState<WorkAssetStats>(emptyStats);
  const [recent, setRecent] = React.useState<WorkAsset[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);

  React.useEffect(() => {
    let alive = true;
    setLoading(true);
    void Promise.all([api.workAssetStats(), api.workAssets()])
      .then(([nextStats, assets]) => {
        if (!alive) return;
        setStats(nextStats);
        setRecent(
          [...assets]
            .sort((a, b) => (b.last_used_at || b.updated_at).localeCompare(a.last_used_at || a.updated_at))
            .slice(0, 3),
        );
        setError(undefined);
      })
      .catch((cause: Error) => {
        if (alive) setError(cause.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [refreshKey, reloadKey]);

  return (
    <Card className="h-full rounded-[24px] border-white/70 bg-white/80 shadow-[0_20px_60px_-42px_rgba(70,58,145,0.5)]">
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-sm font-semibold">资料资产</p>
            <p className="text-xs text-muted-foreground">Agent 执行时可检索和引用的个人资料</p>
          </div>
          <Button variant="ghost" size="icon" className="size-10 shrink-0 rounded-xl" asChild>
            <Link href="/work/assets" aria-label="打开资料资产">
              <ArrowRight className="size-4" />
            </Link>
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <div className="space-y-2" aria-busy="true">
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-16 rounded-xl" />
          </div>
        ) : error ? (
          <div className="rounded-xl border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive">
            <p>资产状态加载失败：{error}</p>
            <Button
              variant="outline"
              size="sm"
              className="mt-2 h-10 rounded-xl"
              onClick={() => setReloadKey((value) => value + 1)}
            >
              <RefreshCw className="size-4" />
              重试
            </Button>
          </div>
        ) : (
          <>
            <div className="grid grid-cols-3 gap-2 text-center">
              <Metric label="全部" value={stats.total} icon={<File className="size-3.5" />} />
              <Metric label="处理中" value={stats.processing} icon={<Loader2 className="size-3.5" />} />
              <Metric label="失败" value={stats.failed} icon={<TriangleAlert className="size-3.5" />} danger />
            </div>
            {recent.length === 0 ? (
              <div className="rounded-xl border border-dashed p-4 text-center">
                <p className="text-sm font-medium">还没有资料资产</p>
                <p className="mt-1 text-xs text-muted-foreground">上传文档、表格、图片或登记接口，让 Agent 使用你的资料。</p>
                <Button className="mt-3 h-10 rounded-xl" asChild>
                  <Link href="/work/assets">添加第一份资料</Link>
                </Button>
              </div>
            ) : (
              <div className="space-y-1.5">
                {recent.map((asset) => (
                  <Link
                    key={asset.id}
                    href="/work/assets"
                    className="flex min-h-11 items-center gap-2 rounded-xl px-2 py-1.5 text-sm transition-colors hover:bg-violet-500/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="rounded-lg bg-violet-500/10 p-2 text-violet-700">{assetIcon(asset.asset_type)}</span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{asset.title}</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {asset.summary || asset.file_name || "等待解析"}
                      </span>
                    </span>
                  </Link>
                ))}
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function Metric({
  label,
  value,
  icon,
  danger,
}: {
  label: string;
  value: number;
  icon: React.ReactNode;
  danger?: boolean;
}) {
  return (
    <div className={cn("rounded-xl border bg-violet-500/[0.05] px-2 py-2", danger && "bg-destructive/[0.05]")}>
      <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground">
        {icon}
        {label}
      </div>
      <p className={cn("mt-1 text-lg font-semibold text-violet-700", danger && "text-destructive")}>{value}</p>
    </div>
  );
}
