"use client";

import * as React from "react";
import {
  Activity,
  Check,
  CheckCircle2,
  ChevronDown,
  CircleSlash2,
  Clock3,
  Code2,
  ExternalLink,
  FileImage,
  FileSpreadsheet,
  FileText,
  Globe2,
  Loader2,
  Pin,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  TriangleAlert,
  XCircle,
} from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import type {
  WorkAsset,
  WorkAssetAPICall,
  WorkAssetChunk,
  WorkTaskAssetCandidate,
  WorkTaskAssetRun,
  WorkTaskAssets,
} from "@/lib/types";
import { cn } from "@/lib/utils";

type AssetSelection = {
  pinned: number[];
  excluded: number[];
};

const typeLabels: Record<WorkAsset["asset_type"], string> = {
  image: "图片",
  document: "文档",
  table: "表格",
  link: "链接",
  api: "接口",
};

function AssetTypeIcon({ type }: { type: WorkAsset["asset_type"] }) {
  if (type === "image") return <FileImage className="size-4" />;
  if (type === "table") return <FileSpreadsheet className="size-4" />;
  if (type === "link" || type === "api") return <Globe2 className="size-4" />;
  return <FileText className="size-4" />;
}

export function WorkTaskAssetsPanel({
  taskId,
  initialPinned = [],
  initialExcluded = [],
  onSelectionChange,
}: {
  taskId?: number;
  initialPinned?: number[];
  initialExcluded?: number[];
  onSelectionChange: (selection: AssetSelection) => void;
}) {
  const [pinnedIDs, setPinnedIDs] = React.useState<number[]>(initialPinned);
  const [excludedIDs, setExcludedIDs] = React.useState<number[]>(initialExcluded);
  const [relations, setRelations] = React.useState<WorkTaskAssets | null>(null);
  const [runs, setRuns] = React.useState<WorkTaskAssetRun[]>([]);
  const [apiCalls, setApiCalls] = React.useState<WorkAssetAPICall[]>([]);
  const [apiCallsLoading, setApiCallsLoading] = React.useState(false);
  const [apiCallsError, setApiCallsError] = React.useState<string>();
  const [apiCallsReloadKey, setApiCallsReloadKey] = React.useState(0);
  const [allAssets, setAllAssets] = React.useState<WorkAsset[]>([]);
  const [query, setQuery] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const [recommending, setRecommending] = React.useState(false);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const loadedRef = React.useRef(false);
  const initialPinnedRef = React.useRef(initialPinned);
  const initialExcludedRef = React.useRef(initialExcluded);
  const onSelectionChangeRef = React.useRef(onSelectionChange);

  React.useEffect(() => {
    initialPinnedRef.current = initialPinned;
    initialExcludedRef.current = initialExcluded;
  }, [initialExcluded, initialPinned]);

  React.useEffect(() => {
    onSelectionChangeRef.current = onSelectionChange;
  }, [onSelectionChange]);

  React.useEffect(() => {
    let alive = true;
    loadedRef.current = false;
    setLoading(true);
    setError(undefined);
    const request = taskId
      ? Promise.all([api.workTaskAssets(taskId), api.workTaskAssetRuns(taskId)]).then(
          ([nextRelations, nextRuns]) => ({
            relations: nextRelations,
            runs: nextRuns,
            assets: [] as WorkAsset[],
          }),
        )
      : api.workAssets().then((assets) => ({
          relations: null,
          runs: [] as WorkTaskAssetRun[],
          assets,
        }));
    void request
      .then((result) => {
        if (!alive) return;
        setRelations(result.relations);
        setRuns(result.runs);
        setAllAssets(result.assets);
        if (result.relations) {
          const pinned = result.relations.pinned.map((asset) => asset.id);
          const excluded = result.relations.excluded.map((asset) => asset.id);
          setPinnedIDs(pinned);
          setExcludedIDs(excluded);
          onSelectionChangeRef.current({ pinned, excluded });
        } else {
          const pinned = initialPinnedRef.current;
          const excluded = initialExcludedRef.current;
          setPinnedIDs(pinned);
          setExcludedIDs(excluded);
          onSelectionChangeRef.current({ pinned, excluded });
        }
        loadedRef.current = true;
      })
      .catch((cause: Error) => {
        if (alive) setError(cause.message || "资料读取失败");
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [reloadKey, taskId]);

  React.useEffect(() => {
    if (!taskId) {
      setApiCalls([]);
      setApiCallsLoading(false);
      setApiCallsError(undefined);
      return;
    }
    let alive = true;
    setApiCallsLoading(true);
    setApiCallsError(undefined);
    void api
      .workTaskAssetAPICalls(taskId)
      .then((items) => {
        if (alive) setApiCalls(items);
      })
      .catch((cause: Error) => {
        if (alive) setApiCallsError(cause.message || "接口调用审计读取失败");
      })
      .finally(() => {
        if (alive) setApiCallsLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [apiCallsReloadKey, taskId]);

  const candidates = React.useMemo(() => {
    if (relations) {
      const merged = [
        ...relations.pinned.map((asset) => ({ asset, pinned: true })),
        ...relations.recommended,
      ];
      const seen = new Set<number>();
      return merged.filter((item) => {
        if (seen.has(item.asset.id) || excludedIDs.includes(item.asset.id)) return false;
        seen.add(item.asset.id);
        return true;
      });
    }
    const normalized = query.trim().toLowerCase();
    return allAssets
      .filter((asset) => !excludedIDs.includes(asset.id))
      .filter(
        (asset) =>
          !normalized ||
          `${asset.title} ${asset.summary} ${asset.keywords.join(" ")} ${asset.tags.map((tag) => tag.name).join(" ")}`
            .toLowerCase()
            .includes(normalized),
      )
      .slice(0, 20)
      .map((asset) => ({ asset, pinned: pinnedIDs.includes(asset.id), selection_reason: "" }));
  }, [allAssets, excludedIDs, pinnedIDs, query, relations]);

  const excludedAssets = relations?.excluded ?? allAssets.filter((asset) => excludedIDs.includes(asset.id));

  function applySelection(nextPinned: number[], nextExcluded: number[]) {
    setPinnedIDs(nextPinned);
    setExcludedIDs(nextExcluded);
    onSelectionChange({ pinned: nextPinned, excluded: nextExcluded });
  }

  function togglePin(assetID: number) {
    if (pinnedIDs.includes(assetID)) {
      applySelection(
        pinnedIDs.filter((id) => id !== assetID),
        excludedIDs,
      );
      return;
    }
    applySelection(
      [...pinnedIDs, assetID],
      excludedIDs.filter((id) => id !== assetID),
    );
  }

  function exclude(assetID: number) {
    applySelection(
      pinnedIDs.filter((id) => id !== assetID),
      [...excludedIDs.filter((id) => id !== assetID), assetID],
    );
  }

  function restoreExcluded(assetID: number) {
    applySelection(
      pinnedIDs,
      excludedIDs.filter((id) => id !== assetID),
    );
  }

  async function refreshRecommendations() {
    if (!taskId) return;
    setRecommending(true);
    setError(undefined);
    try {
      const recommended = await api.recommendWorkTaskAssets(taskId);
      setRelations((current) => (current ? { ...current, recommended } : current));
    } catch (cause) {
      setError(`重新推荐失败：${(cause as Error).message}`);
    } finally {
      setRecommending(false);
    }
  }

  return (
    <section className="space-y-3 rounded-2xl border border-violet-500/15 bg-violet-500/[0.035] p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <Sparkles className="size-4 text-violet-600" />
            参考资产
          </h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            Agent 会优先使用固定资料，再自动检索相关资料。保存任务时一并保存选择。
          </p>
        </div>
        <div className="flex items-center gap-1">
          {taskId && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-10 rounded-xl"
              disabled={recommending}
              onClick={() => void refreshRecommendations()}
            >
              <RefreshCw className={cn("size-4", recommending && "animate-spin")} />
              重新推荐
            </Button>
          )}
          <Button variant="ghost" size="sm" className="h-10 rounded-xl" asChild>
            <Link href="/work/assets">
              管理资料
              <ExternalLink className="size-3.5" />
            </Link>
          </Button>
        </div>
      </div>

      {!taskId && (
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="h-11 rounded-xl bg-background pl-9"
            placeholder="搜索要固定的资料"
            aria-label="搜索参考资产"
          />
        </div>
      )}

      {loading ? (
        <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          正在检索相关资料...
        </div>
      ) : error ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive">
          <span>{error}</span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-10 rounded-xl"
            onClick={() => setReloadKey((value) => value + 1)}
          >
            重试
          </Button>
        </div>
      ) : candidates.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-background/60 p-4 text-center text-sm text-muted-foreground">
          暂无相关资产。任务仍可正常执行，也可以先到资料资产中上传资料。
        </div>
      ) : (
        <div className="grid gap-2 lg:grid-cols-2">
          {candidates.map((item) => {
            const asset = item.asset;
            const pinned = pinnedIDs.includes(asset.id);
            const reason =
              "selection_reason" in item && typeof item.selection_reason === "string"
                ? item.selection_reason
                : "";
            const relevance = "relevance" in item ? item.relevance : asset.relevance;
            const chunks: WorkAssetChunk[] =
              "suggested_chunks" in item && Array.isArray(item.suggested_chunks)
                ? (item.suggested_chunks as WorkAssetChunk[])
                : [];
            return (
              <article
                key={asset.id}
                className={cn(
                  "rounded-xl border bg-background/85 p-3 transition-colors",
                  pinned && "border-violet-500/35 bg-violet-500/[0.055]",
                )}
              >
                <div className="flex items-start gap-2.5">
                  <label className="mt-0.5 flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-xl bg-violet-500/10 text-violet-700">
                    <Checkbox
                      checked={pinned}
                      onCheckedChange={() => togglePin(asset.id)}
                      aria-label={pinned ? `取消固定 ${asset.title}` : `固定 ${asset.title}`}
                    />
                  </label>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="truncate text-sm font-medium">{asset.title}</span>
                      <Badge variant="outline" className="h-5 gap-1 px-1.5 text-[11px]">
                        <AssetTypeIcon type={asset.asset_type} />
                        {typeLabels[asset.asset_type]}
                      </Badge>
                      {pinned && (
                        <Badge className="h-5 gap-1 bg-violet-600 px-1.5 text-[11px] text-white">
                          <Pin className="size-3" />
                          已固定
                        </Badge>
                      )}
                    </div>
                    <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                      {reason || asset.summary || asset.description || "等待解析摘要"}
                    </p>
                    {typeof relevance === "number" && relevance > 0 && (
                      <p className="mt-1 text-[11px] text-violet-700">
                        相关度 {Math.round(relevance * 100)}%
                        {asset.current_version ? ` · v${asset.current_version}` : ""}
                      </p>
                    )}
                    {chunks.length > 0 && (
                      <details className="group mt-2">
                        <summary className="flex min-h-9 cursor-pointer list-none items-center gap-1 text-xs font-medium text-violet-700">
                          <ChevronDown className="size-3.5 transition-transform group-open:rotate-180" />
                          查看推荐片段
                        </summary>
                        <div className="space-y-1.5 pt-1">
                          {chunks.slice(0, 3).map((chunk) => (
                            <p key={chunk.id} className="rounded-lg bg-muted/50 p-2 text-xs leading-5">
                              <span className="font-medium">{chunk.locator || `片段 ${chunk.ordinal + 1}`}：</span>
                              {chunk.content.slice(0, 240)}
                            </p>
                          ))}
                        </div>
                      </details>
                    )}
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-10 shrink-0 rounded-xl text-muted-foreground"
                    onClick={() => exclude(asset.id)}
                    aria-label={`排除 ${asset.title}`}
                    title="排除这份资料"
                  >
                    <CircleSlash2 className="size-4" />
                  </Button>
                </div>
              </article>
            );
          })}
        </div>
      )}

      {excludedAssets.length > 0 && (
        <details className="rounded-xl border bg-background/65 px-3 py-2">
          <summary className="flex min-h-10 cursor-pointer list-none items-center justify-between gap-2 text-sm">
            <span className="flex items-center gap-2 text-muted-foreground">
              <CircleSlash2 className="size-4" />
              已排除 {excludedAssets.length} 份资料
            </span>
            <ChevronDown className="size-4 text-muted-foreground" />
          </summary>
          <div className="flex flex-wrap gap-2 pt-2">
            {excludedAssets.map((asset) => (
              <Button
                key={asset.id}
                type="button"
                variant="outline"
                size="sm"
                className="h-10 rounded-xl"
                onClick={() => restoreExcluded(asset.id)}
              >
                <Check className="size-3.5" />
                恢复 {asset.title}
              </Button>
            ))}
          </div>
        </details>
      )}

      {taskId && (
        <details className="rounded-xl border bg-background/65 px-3 py-2" open={runs.length > 0}>
          <summary className="flex min-h-10 cursor-pointer list-none items-center justify-between gap-2 text-sm">
            <span className="font-medium">执行引用快照 · {runs.length}</span>
            <ChevronDown className="size-4 text-muted-foreground" />
          </summary>
          {runs.length === 0 ? (
            <p className="pb-2 text-xs leading-5 text-muted-foreground">
              任务启动后，这里会冻结本次实际使用的资料版本、片段和选择原因。
            </p>
          ) : (
            <div className="space-y-2 pb-1 pt-1">
              {runs.map((run) => (
                <article key={run.id} className="min-w-0 rounded-xl border bg-white/70 p-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <AssetTypeIcon type={run.asset_type} />
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">{run.asset_title}</span>
                    <Badge variant="outline">v{run.version_number}</Badge>
                    <Badge
                      variant="outline"
                      className={cn(
                        run.content_origin === "ai_organized" &&
                          "border-violet-500/30 bg-violet-500/10 text-violet-700",
                      )}
                    >
                      {run.content_origin === "ai_organized" ? (
                        <Sparkles className="mr-1 size-3" />
                      ) : (
                        <FileText className="mr-1 size-3" />
                      )}
                      {run.content_origin === "ai_organized" ? "AI 整理版" : "原始资料"}
                    </Badge>
                    <Badge variant="outline">{run.source || "自动选择"}</Badge>
                  </div>
                  <p className="mt-1.5 text-xs leading-5 text-muted-foreground">
                    {run.selection_reason || "由执行器自动选择"}
                  </p>
                  {run.content_origin === "ai_organized" && (
                    <p className="mt-1 break-words text-xs leading-5 text-violet-700">
                      AI 整理确认版本
                      {run.organization_id ? ` #${run.organization_id}` : ""}
                      {run.confirmed_at ? ` · ${formatAuditTime(run.confirmed_at)}` : ""}
                    </p>
                  )}
                  {run.content_preview && (
                    <p className="mt-2 line-clamp-4 whitespace-pre-wrap rounded-lg bg-muted/50 p-2 text-xs leading-5">
                      {run.content_preview}
                    </p>
                  )}
                  {run.chunk_ids.length > 0 && (
                    <p className="mt-1.5 flex items-center gap-1 text-[11px] text-muted-foreground">
                      <TriangleAlert className="size-3" />
                      引用 {run.chunk_ids.length} 个冻结片段
                    </p>
                  )}
                  {run.operations.length > 0 && (
                    <div className="mt-3 space-y-1.5 border-t border-violet-500/15 pt-2">
                      <p className="flex items-center gap-1.5 text-xs font-medium text-violet-800">
                        <Code2 className="size-3.5" />
                        冻结的接口操作 · {run.operations.length}
                      </p>
                      {run.operations.map((operation) => (
                        <div
                          key={`${run.id}-${operation.operation_id}`}
                          className="flex min-w-0 flex-wrap items-center gap-1.5 rounded-lg bg-violet-500/[0.06] px-2 py-1.5 text-xs"
                        >
                          <Badge variant="outline" className="font-mono text-[11px]">
                            {operation.method}
                          </Badge>
                          <span className="min-w-0 flex-1 break-all font-mono text-[11px]">
                            {operation.path}
                          </span>
                          {operation.write && (
                            <Badge className="border-amber-500/30 bg-amber-500/10 text-amber-800" variant="outline">
                              写操作
                            </Badge>
                          )}
                          <span className="basis-full break-words text-muted-foreground">
                            {operation.summary || operation.operation_id}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </article>
              ))}
            </div>
          )}
        </details>
      )}

      {taskId && (
        <details className="rounded-xl border bg-background/65 px-3 py-2" open={apiCalls.length > 0}>
          <summary className="flex min-h-10 cursor-pointer list-none items-center justify-between gap-2 text-sm">
            <span className="flex items-center gap-2 font-medium">
              <ShieldCheck className="size-4 text-emerald-700" />
              接口调用审计 · {apiCalls.length}
            </span>
            <ChevronDown className="size-4 text-muted-foreground" />
          </summary>
          {apiCallsLoading ? (
            <div className="flex items-center gap-2 py-3 text-xs text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              正在读取调用记录...
            </div>
          ) : apiCallsError ? (
            <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-destructive/20 bg-destructive/5 p-2 text-xs text-destructive">
              <span className="min-w-0 break-words">{apiCallsError}</span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-10 rounded-xl"
                onClick={() => setApiCallsReloadKey((value) => value + 1)}
              >
                重试
              </Button>
            </div>
          ) : apiCalls.length === 0 ? (
            <p className="pb-2 text-xs leading-5 text-muted-foreground">
              本次执行尚未调用接口。确认后的接口资料由 Agent 按需调用，每次调用都会记录在这里。
            </p>
          ) : (
            <div className="space-y-2 pb-1 pt-1">
              {apiCalls.map((call) => (
                <article
                  key={call.id}
                  className="min-w-0 rounded-xl border bg-white/75 p-3 text-xs shadow-sm"
                >
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <Badge variant="outline" className="font-mono">
                      {call.method}
                    </Badge>
                    <span className="min-w-0 flex-1 break-all font-medium">{call.operation_id}</span>
                    <Badge
                      variant="outline"
                      className={cn(
                        call.success
                          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700"
                          : "border-destructive/30 bg-destructive/10 text-destructive",
                      )}
                    >
                      {call.success ? (
                        <CheckCircle2 className="mr-1 size-3" />
                      ) : (
                        <XCircle className="mr-1 size-3" />
                      )}
                      HTTP {call.status_code || "—"}
                    </Badge>
                  </div>
                  <p className="mt-2 break-all rounded-lg bg-muted/45 px-2 py-1.5 font-mono text-[11px] leading-5">
                    {call.host}
                    {call.path}
                  </p>
                  <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Clock3 className="size-3" />
                      {call.duration_ms} ms
                    </span>
                    <span className="flex items-center gap-1">
                      <Activity className="size-3" />
                      资产版本 #{call.asset_version_id}
                    </span>
                    <span>{formatAuditTime(call.created_at)}</span>
                  </div>
                  {call.error && (
                    <p className="mt-2 break-words rounded-lg bg-destructive/5 p-2 leading-5 text-destructive">
                      {call.error}
                    </p>
                  )}
                  {call.response_preview && (
                    <details className="mt-2">
                      <summary className="min-h-9 cursor-pointer text-xs font-medium text-violet-700">
                        查看响应预览
                      </summary>
                      <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-slate-950 p-2 font-mono text-[11px] leading-5 text-slate-100">
                        {call.response_preview}
                      </pre>
                    </details>
                  )}
                </article>
              ))}
            </div>
          )}
        </details>
      )}
    </section>
  );
}

function formatAuditTime(value: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
