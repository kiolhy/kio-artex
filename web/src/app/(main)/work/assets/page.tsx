"use client";

import * as React from "react";
import {
  Archive,
  ArchiveRestore,
  Braces,
  CheckCircle2,
  Download,
  ExternalLink,
  File,
  FileImage,
  FileSpreadsheet,
  FileText,
  Globe2,
  History,
  Loader2,
  Plus,
  RefreshCw,
  RotateCw,
  Search,
  Tags,
  Trash2,
  Upload,
  XCircle,
} from "lucide-react";
import { toast } from "sonner";

import { Markdown } from "@/components/markdown";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type {
  WorkAsset,
  WorkAssetDetail,
  WorkAssetParseStatus,
  WorkAssetStats,
  WorkAssetType,
  WorkAssetVersion,
  WorkTag,
} from "@/lib/types";
import { cn } from "@/lib/utils";

import { WorkEmptyState, WorkErrorState, WorkPageHeader } from "../_components/work-shell";
import { WorkAssetOrganizationPanel } from "./work-asset-organization-panel";

type AssetView = "active" | "archived" | "trash";

const ASSET_TYPE_LABELS: Record<WorkAssetType, string> = {
  image: "图片",
  document: "文档",
  table: "表格",
  link: "链接",
  api: "接口",
};

const PARSE_LABELS: Record<WorkAssetParseStatus, string> = {
  pending: "等待解析",
  processing: "解析中",
  ready: "可用",
  partial: "部分可用",
  failed: "解析失败",
};

const PARSE_CLASSES: Record<WorkAssetParseStatus, string> = {
  pending: "border-sky-500/25 bg-sky-500/10 text-sky-700",
  processing: "border-violet-500/25 bg-violet-500/10 text-violet-700",
  ready: "border-emerald-500/25 bg-emerald-500/10 text-emerald-700",
  partial: "border-amber-500/25 bg-amber-500/10 text-amber-700",
  failed: "border-destructive/25 bg-destructive/10 text-destructive",
};

const ORGANIZATION_LABELS: Record<
  Exclude<WorkAsset["organization_status"], "none">,
  string
> = {
  queued: "整理已排队",
  processing: "AI 整理中",
  draft: "草稿待确认",
  failed: "整理失败",
  confirmed: "整理已确认",
  discarded: "整理已丢弃",
};

const ORGANIZATION_CLASSES: Record<
  Exclude<WorkAsset["organization_status"], "none">,
  string
> = {
  queued: "border-sky-500/25 bg-sky-500/10 text-sky-700",
  processing: "border-violet-500/25 bg-violet-500/10 text-violet-700",
  draft: "border-amber-500/25 bg-amber-500/10 text-amber-700",
  failed: "border-destructive/25 bg-destructive/10 text-destructive",
  confirmed: "border-emerald-500/25 bg-emerald-500/10 text-emerald-700",
  discarded: "border-slate-400/25 bg-slate-500/10 text-slate-600",
};

function assetIcon(type: WorkAssetType, className = "size-5") {
  if (type === "image") return <FileImage className={className} />;
  if (type === "table") return <FileSpreadsheet className={className} />;
  if (type === "link") return <Globe2 className={className} />;
  if (type === "api") return <Braces className={className} />;
  return <FileText className={className} />;
}

function formatBytes(value: number) {
  if (!value) return "0 B";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function formatTime(value?: string | null) {
  if (!value) return "尚未使用";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "时间未知";
  return date.toLocaleString("zh-CN", { hour12: false });
}

export default function WorkAssetsPage() {
  const [assets, setAssets] = React.useState<WorkAsset[]>([]);
  const [tags, setTags] = React.useState<WorkTag[]>([]);
  const [stats, setStats] = React.useState<WorkAssetStats>({ total: 0, processing: 0, failed: 0, trashed: 0 });
  const [view, setView] = React.useState<AssetView>("active");
  const [query, setQuery] = React.useState("");
  const [searchQuery, setSearchQuery] = React.useState("");
  const [typeFilter, setTypeFilter] = React.useState<WorkAssetType | "">("");
  const [statusFilter, setStatusFilter] = React.useState<WorkAssetParseStatus | "">("");
  const [tagFilter, setTagFilter] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const [selectedID, setSelectedID] = React.useState<number | null>(null);
  const [detail, setDetail] = React.useState<WorkAssetDetail | null>(null);
  const [detailLoading, setDetailLoading] = React.useState(false);
  const [detailError, setDetailError] = React.useState<string>();
  const [detailReloadKey, setDetailReloadKey] = React.useState(0);
  const [detailVersionID, setDetailVersionID] = React.useState<number | null>(null);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [uploadOpen, setUploadOpen] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const listRequestRef = React.useRef(0);
  const detailRequestRef = React.useRef(0);

  React.useEffect(() => {
    const timer = window.setTimeout(() => setSearchQuery(query.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [query]);

  React.useEffect(() => {
    let cancelled = false;
    void api
      .workTags()
      .then((items) => {
        if (!cancelled) setTags(items);
      })
      .catch((cause: Error) => toast.error(`标签加载失败：${cause.message}`));
    return () => {
      cancelled = true;
    };
  }, []);

  const loadStats = React.useCallback(async () => {
    try {
      setStats(await api.workAssetStats());
    } catch (cause) {
      toast.error(`资产统计加载失败：${(cause as Error).message}`);
    }
  }, []);

  const load = React.useCallback(async () => {
    const requestID = ++listRequestRef.current;
    setLoading(true);
    try {
      const items = await api.workAssets({
        q: searchQuery || undefined,
        type: typeFilter,
        status: statusFilter,
        tag_ids: tagFilter ? [Number(tagFilter)] : undefined,
        archived: view === "archived",
        trash: view === "trash",
      });
      if (requestID !== listRequestRef.current) return;
      setAssets(items);
      setError(undefined);
    } catch (cause) {
      if (requestID === listRequestRef.current) setError((cause as Error).message);
    } finally {
      if (requestID === listRequestRef.current) setLoading(false);
    }
  }, [searchQuery, statusFilter, tagFilter, typeFilter, view]);

  React.useEffect(() => {
    void load();
  }, [load, reloadKey]);

  React.useEffect(() => {
    void loadStats();
  }, [loadStats, reloadKey]);

  React.useEffect(() => {
    if (!selectedID) {
      setDetail(null);
      setDetailError(undefined);
      return;
    }
    const requestID = ++detailRequestRef.current;
    setDetailLoading(true);
    setDetailError(undefined);
    void api
      .workAsset(selectedID)
      .then((next) => {
        if (requestID !== detailRequestRef.current) return;
        setDetail(next);
        setDetailVersionID(next.versions[0]?.id ?? null);
      })
      .catch((cause: Error) => {
        if (requestID === detailRequestRef.current) setDetailError(cause.message);
      })
      .finally(() => {
        if (requestID === detailRequestRef.current) setDetailLoading(false);
      });
  }, [detailReloadKey, selectedID]);

  async function refreshAll() {
    setReloadKey((value) => value + 1);
  }

  async function refreshDetail() {
    setDetailReloadKey((value) => value + 1);
  }

  async function selectVersion(versionID: number) {
    if (!detail) return;
    setDetailVersionID(versionID);
    try {
      const chunks = await api.workAssetChunks(detail.asset.id, versionID);
      setDetail((current) => (current ? { ...current, chunks } : current));
    } catch (cause) {
      toast.error(`版本内容加载失败：${(cause as Error).message}`);
    }
  }

  async function trashSelected() {
    if (!detail) return;
    setBusy(true);
    try {
      await api.trashWorkAsset(detail.asset.id);
      toast.success("资产已移入回收站");
      setSelectedID(null);
      await refreshAll();
    } catch (cause) {
      toast.error(`移入回收站失败：${(cause as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  async function restoreSelected() {
    if (!detail) return;
    setBusy(true);
    try {
      await api.restoreWorkAsset(detail.asset.id);
      toast.success("资产已恢复");
      setView("active");
      await refreshAll();
      await refreshDetail();
    } catch (cause) {
      toast.error(`恢复失败：${(cause as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  async function purgeSelected() {
    if (!detail) return;
    setBusy(true);
    try {
      await api.purgeWorkAsset(detail.asset.id);
      toast.success("资产已彻底删除");
      setDeleteOpen(false);
      setSelectedID(null);
      await refreshAll();
    } catch (cause) {
      toast.error(`彻底删除失败：${(cause as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  async function reparseSelected() {
    if (!detail) return;
    setBusy(true);
    try {
      await api.reparseWorkAsset(detail.asset.id);
      toast.success("已重新提交解析");
      await refreshAll();
      await refreshDetail();
    } catch (cause) {
      toast.error(`重新解析失败：${(cause as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-5 pb-8">
      <WorkPageHeader
        title="资料资产"
        description="把图片、文档、表格、链接和接口变成 Agent 可检索、可引用、可复现的资料。"
      >
        <Button className="h-11 rounded-xl sm:h-9" onClick={() => setUploadOpen(true)}>
          <Upload className="size-4" />
          上传文件
        </Button>
        <Button variant="outline" className="h-11 rounded-xl sm:h-9" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          新建资料
        </Button>
      </WorkPageHeader>

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="全部资料" value={stats.total} icon={<File className="size-4" />} />
        <StatCard label="处理中" value={stats.processing} icon={<Loader2 className="size-4" />} />
        <StatCard label="解析失败" value={stats.failed} icon={<XCircle className="size-4" />} tone="danger" />
        <StatCard label="回收站" value={stats.trashed} icon={<Trash2 className="size-4" />} />
      </div>

      <Card className="overflow-hidden rounded-[24px] border-white/70 bg-white/80 shadow-[0_20px_60px_-42px_rgba(70,58,145,0.5)]">
        <CardContent className="space-y-4 p-4 sm:p-5">
          <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="flex flex-wrap items-center gap-2">
              <ViewButton active={view === "active"} onClick={() => setView("active")}>
                资料库
              </ViewButton>
              <ViewButton active={view === "archived"} onClick={() => setView("archived")}>
                <Archive className="size-4" />
                已归档
              </ViewButton>
              <ViewButton active={view === "trash"} onClick={() => setView("trash")}>
                <Trash2 className="size-4" />
                回收站
              </ViewButton>
            </div>
            <div className="relative min-w-0 flex-1 xl:max-w-md">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                className="h-11 rounded-xl pl-9"
                placeholder="搜索标题、摘要或关键词"
                aria-label="搜索资料资产"
              />
            </div>
          </div>

          <div className="grid gap-2 sm:grid-cols-3">
            <FilterSelect
              label="全部类型"
              value={typeFilter}
              onChange={(value) => setTypeFilter(value as WorkAssetType | "")}
              options={Object.entries(ASSET_TYPE_LABELS)}
            />
            <FilterSelect
              label="全部状态"
              value={statusFilter}
              onChange={(value) => setStatusFilter(value as WorkAssetParseStatus | "")}
              options={Object.entries(PARSE_LABELS)}
            />
            <NativeSelect
              value={tagFilter}
              onChange={(event) => setTagFilter(event.target.value)}
              className="w-full [&>select]:h-11 [&>select]:rounded-xl"
              aria-label="按标签筛选"
            >
              <option value="">全部标签</option>
              {tags.map((tag) => (
                <option key={tag.id} value={tag.id}>
                  {tag.name}
                </option>
              ))}
            </NativeSelect>
          </div>

          {loading ? (
            <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3" aria-busy="true">
              {[0, 1, 2, 3, 4, 5].map((item) => (
                <Skeleton key={item} className="h-48 rounded-[22px]" />
              ))}
            </div>
          ) : error ? (
            <WorkErrorState message={`资料加载失败：${error}`} onRetry={() => void load()} />
          ) : assets.length === 0 ? (
            <WorkEmptyState
              title={view === "trash" ? "回收站是空的" : view === "archived" ? "还没有归档资料" : "还没有资料资产"}
              description={
                view === "active"
                  ? "上传文档、表格或图片，也可以登记链接和接口。Agent 会在任务执行时自动检索并引用相关资料。"
                  : undefined
              }
            >
              {view === "active" && (
                <Button className="h-11 rounded-xl" onClick={() => setUploadOpen(true)}>
                  <Upload className="size-4" />
                  上传第一份资料
                </Button>
              )}
            </WorkEmptyState>
          ) : (
            <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
              {assets.map((asset) => (
                <AssetCard key={asset.id} asset={asset} onClick={() => setSelectedID(asset.id)} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <UploadAssetDialog
        open={uploadOpen}
        tags={tags}
        onClose={() => setUploadOpen(false)}
        onUploaded={async () => {
          setUploadOpen(false);
          await refreshAll();
        }}
      />
      <CreateAssetDialog
        open={createOpen}
        tags={tags}
        onClose={() => setCreateOpen(false)}
        onCreated={async (asset) => {
          setCreateOpen(false);
          setSelectedID(asset.id);
          await refreshAll();
        }}
      />
      <AssetDetailDialog
        open={selectedID !== null}
        detail={detail}
        loading={detailLoading}
        error={detailError}
        tags={tags}
        selectedVersionID={detailVersionID}
        busy={busy}
        onClose={() => setSelectedID(null)}
        onRetry={() => void refreshDetail()}
        onSelectVersion={(id) => void selectVersion(id)}
        onReparse={() => void reparseSelected()}
        onTrash={() => void trashSelected()}
        onRestore={() => void restoreSelected()}
        onPurge={() => setDeleteOpen(true)}
        onChanged={async () => {
          await refreshAll();
          await refreshDetail();
        }}
      />

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>彻底删除「{detail?.asset.title ?? "这份资料"}」？</AlertDialogTitle>
            <AlertDialogDescription>
              这会永久清理原文件、全部版本、检索片段和任务引用快照，无法恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={busy} onClick={(event) => {
              event.preventDefault();
              void purgeSelected();
            }}>
              {busy && <Loader2 className="size-4 animate-spin" />}
              永久删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function StatCard({
  label,
  value,
  icon,
  tone,
}: {
  label: string;
  value: number;
  icon: React.ReactNode;
  tone?: "danger";
}) {
  return (
    <div className="rounded-[22px] border border-white/70 bg-white/75 p-4 shadow-[0_16px_40px_-36px_rgba(70,58,145,0.6)]">
      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>{label}</span>
        <span className={cn("rounded-xl bg-violet-500/10 p-2 text-violet-600", tone === "danger" && "bg-destructive/10 text-destructive")}>
          {icon}
        </span>
      </div>
      <p className="mt-2 text-2xl font-semibold tracking-tight">{value}</p>
    </div>
  );
}

function ViewButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <Button
      type="button"
      variant={active ? "default" : "outline"}
      className="h-11 rounded-xl px-3 sm:h-9"
      aria-pressed={active}
      onClick={onClick}
    >
      {children}
    </Button>
  );
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: [string, string][];
}) {
  return (
    <NativeSelect
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="w-full [&>select]:h-11 [&>select]:rounded-xl"
      aria-label={label}
    >
      <option value="">{label}</option>
      {options.map(([key, text]) => (
        <option key={key} value={key}>
          {text}
        </option>
      ))}
    </NativeSelect>
  );
}

function AssetCard({ asset, onClick }: { asset: WorkAsset; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="group min-h-48 rounded-[22px] border border-white/80 bg-white/85 p-4 text-left shadow-[0_16px_44px_-38px_rgba(70,58,145,0.65)] transition duration-200 hover:-translate-y-0.5 hover:border-violet-300/70 hover:shadow-[0_24px_56px_-36px_rgba(93,76,199,0.5)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex items-start gap-3">
        <span className="rounded-2xl bg-gradient-to-br from-violet-500/15 to-sky-400/10 p-3 text-violet-650">
          {assetIcon(asset.asset_type)}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <h3 className="line-clamp-2 text-sm font-semibold leading-5">{asset.title}</h3>
            <Badge variant="outline" className={cn("shrink-0 border", PARSE_CLASSES[asset.parse_status])}>
              {PARSE_LABELS[asset.parse_status]}
            </Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {ASSET_TYPE_LABELS[asset.asset_type]} · v{asset.current_version} · {formatBytes(asset.size_bytes)}
          </p>
          {asset.organization_status !== "none" && (
            <Badge
              variant="outline"
              className={cn(
                "mt-2 border",
                ORGANIZATION_CLASSES[asset.organization_status],
              )}
            >
              {ORGANIZATION_LABELS[asset.organization_status]}
            </Badge>
          )}
        </div>
      </div>
      <p className="mt-3 line-clamp-3 text-sm leading-6 text-foreground/70">
        {asset.summary || asset.description || "尚未生成摘要，打开可查看提取内容和版本。"}
      </p>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {asset.tags.slice(0, 3).map((tag) => (
          <span key={tag.id} className="rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground">
            {tag.name}
          </span>
        ))}
        {asset.tags.length > 3 && <span className="px-1 py-1 text-xs text-muted-foreground">+{asset.tags.length - 3}</span>}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">更新于 {formatTime(asset.updated_at)}</p>
    </button>
  );
}

function TagPicker({
  tags,
  selected,
  onChange,
}: {
  tags: WorkTag[];
  selected: number[];
  onChange: (ids: number[]) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {tags.map((tag) => {
        const checked = selected.includes(tag.id);
        return (
          <label
            key={tag.id}
            className="flex min-h-11 cursor-pointer items-center gap-2 rounded-xl border px-3 py-2 text-sm"
          >
            <Checkbox
              checked={checked}
              onCheckedChange={() =>
                onChange(checked ? selected.filter((id) => id !== tag.id) : [...selected, tag.id])
              }
            />
            <span>{tag.name}</span>
          </label>
        );
      })}
      {tags.length === 0 && <span className="text-sm text-muted-foreground">暂无标签，可在笔记页管理标签。</span>}
    </div>
  );
}

function UploadAssetDialog({
  open,
  tags,
  onClose,
  onUploaded,
}: {
  open: boolean;
  tags: WorkTag[];
  onClose: () => void;
  onUploaded: () => Promise<void>;
}) {
  const [files, setFiles] = React.useState<File[]>([]);
  const [description, setDescription] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [tagIDs, setTagIDs] = React.useState<number[]>([]);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string>();
  const inputRef = React.useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    if (open) return;
    setFiles([]);
    setDescription("");
    setSummary("");
    setTagIDs([]);
    setError(undefined);
  }, [open]);

  async function submit() {
    if (!files.length) return;
    setBusy(true);
    setError(undefined);
    try {
      await api.uploadWorkAssets(files, {
        description: description.trim() || undefined,
        summary: summary.trim() || undefined,
        tag_ids: tagIDs,
      });
      toast.success(`已上传 ${files.length} 份资料`);
      await onUploaded();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && !busy && onClose()}>
      <DialogContent className="max-h-[calc(100dvh-1rem)] overflow-y-auto sm:max-h-[90dvh] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>上传资料</DialogTitle>
          <DialogDescription>支持批量上传，单文件最大 128 MiB。文件只用于解析和参考，不会在服务器上直接执行。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <button
            type="button"
            className="flex min-h-36 w-full flex-col items-center justify-center gap-2 rounded-[22px] border border-dashed border-violet-300 bg-violet-500/[0.04] px-5 py-7 text-center transition hover:bg-violet-500/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={() => inputRef.current?.click()}
          >
            <Upload className="size-7 text-violet-600" />
            <span className="text-sm font-medium">{files.length ? `已选择 ${files.length} 个文件` : "选择图片、文档或表格"}</span>
            <span className="text-xs text-muted-foreground">PDF、Office、CSV、Markdown、JSON、YAML 等</span>
          </button>
          <input
            ref={inputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(event) => setFiles(Array.from(event.target.files ?? []))}
          />
          {files.length > 0 && (
            <div className="max-h-32 space-y-1 overflow-y-auto rounded-xl bg-muted/50 p-2">
              {files.map((file) => (
                <div key={`${file.name}-${file.lastModified}`} className="flex items-center justify-between gap-3 px-2 py-1 text-sm">
                  <span className="truncate">{file.name}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">{formatBytes(file.size)}</span>
                </div>
              ))}
            </div>
          )}
          <div className="grid gap-2">
            <Label htmlFor="asset-upload-description">说明</Label>
            <Textarea
              id="asset-upload-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="这份资料适合在哪些任务中使用？"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="asset-upload-summary">补充摘要</Label>
            <Textarea
              id="asset-upload-summary"
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="可选。解析失败或扫描件可在这里填写人工摘要。"
            />
          </div>
          <div className="grid gap-2">
            <Label>标签</Label>
            <TagPicker tags={tags} selected={tagIDs} onChange={setTagIDs} />
          </div>
          {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" className="h-11 rounded-xl" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button className="h-11 rounded-xl" disabled={busy || files.length === 0} onClick={() => void submit()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
            {busy ? "上传中" : "开始上传"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CreateAssetDialog({
  open,
  tags,
  onClose,
  onCreated,
}: {
  open: boolean;
  tags: WorkTag[];
  onClose: () => void;
  onCreated: (asset: WorkAsset) => Promise<void>;
}) {
  const [type, setType] = React.useState<"link" | "api" | "document">("link");
  const [title, setTitle] = React.useState("");
  const [description, setDescription] = React.useState("");
  const [sourceURL, setSourceURL] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [openAPI, setOpenAPI] = React.useState("");
  const [requestExample, setRequestExample] = React.useState("");
  const [internalAccess, setInternalAccess] = React.useState(false);
  const [allowedHosts, setAllowedHosts] = React.useState("");
  const [tagIDs, setTagIDs] = React.useState<number[]>([]);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string>();

  React.useEffect(() => {
    if (open) return;
    setType("link");
    setTitle("");
    setDescription("");
    setSourceURL("");
    setSummary("");
    setOpenAPI("");
    setRequestExample("");
    setInternalAccess(false);
    setAllowedHosts("");
    setTagIDs([]);
    setError(undefined);
  }, [open]);

  async function submit() {
    if (!title.trim()) return;
    setBusy(true);
    setError(undefined);
    try {
      const asset = await api.createWorkAsset({
        asset_type: type,
        title: title.trim(),
        description,
        source_url: sourceURL.trim(),
        summary,
        tag_ids: tagIDs,
        internal_access: internalAccess,
        allowed_hosts: allowedHosts
          .split(/[\n,]/)
          .map((item) => item.trim())
          .filter(Boolean),
        openapi_content: openAPI,
        request_example: requestExample,
      });
      toast.success("资料已创建");
      await onCreated(asset);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && !busy && onClose()}>
      <DialogContent className="max-h-[calc(100dvh-1rem)] overflow-y-auto sm:max-h-[90dvh] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>新建资料</DialogTitle>
          <DialogDescription>登记网页、接口定义，或创建可补充摘要的手工资料。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-2">
            <Label htmlFor="asset-create-type">类型</Label>
            <NativeSelect
              id="asset-create-type"
              value={type}
              onChange={(event) => setType(event.target.value as typeof type)}
              className="w-full [&>select]:h-11 [&>select]:rounded-xl"
            >
              <option value="link">网页链接</option>
              <option value="api">接口 / OpenAPI</option>
              <option value="document">手工资料</option>
            </NativeSelect>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="asset-create-title">标题</Label>
            <Input
              id="asset-create-title"
              className="h-11 rounded-xl"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="例如：季度经营数据、内部接口说明"
            />
          </div>
          {type !== "document" && (
            <div className="grid gap-2">
              <Label htmlFor="asset-create-url">{type === "api" ? "OpenAPI 地址（可选）" : "网页地址"}</Label>
              <Input
                id="asset-create-url"
                className="h-11 rounded-xl"
                value={sourceURL}
                onChange={(event) => setSourceURL(event.target.value)}
                placeholder="https://..."
              />
            </div>
          )}
          <div className="grid gap-2">
            <Label htmlFor="asset-create-description">说明</Label>
            <Textarea
              id="asset-create-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="这份资料包含什么，适合解决什么问题？"
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="asset-create-summary">摘要</Label>
            <Textarea
              id="asset-create-summary"
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="帮助 Agent 快速判断是否应引用这份资料。"
            />
          </div>
          {type === "api" && (
            <>
              <div className="grid gap-2">
                <Label htmlFor="asset-create-openapi">OpenAPI JSON / YAML</Label>
                <Textarea
                  id="asset-create-openapi"
                  className="min-h-32 font-mono text-xs"
                  value={openAPI}
                  onChange={(event) => setOpenAPI(event.target.value)}
                  placeholder="可直接粘贴 OpenAPI 定义；留空则作为手工登记接口。"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="asset-create-request">请求示例 / 操作说明</Label>
                <Textarea
                  id="asset-create-request"
                  className="min-h-24 font-mono text-xs"
                  value={requestExample}
                  onChange={(event) => setRequestExample(event.target.value)}
                  placeholder="GET /reports?month=2026-10&#10;用途：读取月度报表"
                />
              </div>
            </>
          )}
          {(type === "link" || type === "api") && (
            <div className="space-y-3 rounded-xl border border-amber-500/20 bg-amber-500/[0.04] p-3">
              <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
                <Checkbox checked={internalAccess} onCheckedChange={(value) => setInternalAccess(value === true)} />
                允许访问明确列出的内网主机
              </label>
              {internalAccess && (
                <div className="grid gap-2">
                  <Label htmlFor="asset-allowed-hosts">允许主机（逗号或换行分隔）</Label>
                  <Textarea
                    id="asset-allowed-hosts"
                    value={allowedHosts}
                    onChange={(event) => setAllowedHosts(event.target.value)}
                    placeholder="intranet.example, 10.10.0.8"
                  />
                </div>
              )}
            </div>
          )}
          <div className="grid gap-2">
            <Label>标签</Label>
            <TagPicker tags={tags} selected={tagIDs} onChange={setTagIDs} />
          </div>
          {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" className="h-11 rounded-xl" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button className="h-11 rounded-xl" disabled={busy || !title.trim()} onClick={() => void submit()}>
            {busy && <Loader2 className="size-4 animate-spin" />}
            创建资料
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function AssetDetailDialog({
  open,
  detail,
  loading,
  error,
  tags,
  selectedVersionID,
  busy,
  onClose,
  onRetry,
  onSelectVersion,
  onReparse,
  onTrash,
  onRestore,
  onPurge,
  onChanged,
}: {
  open: boolean;
  detail: WorkAssetDetail | null;
  loading: boolean;
  error?: string;
  tags: WorkTag[];
  selectedVersionID: number | null;
  busy: boolean;
  onClose: () => void;
  onRetry: () => void;
  onSelectVersion: (id: number) => void;
  onReparse: () => void;
  onTrash: () => void;
  onRestore: () => void;
  onPurge: () => void;
  onChanged: () => Promise<void>;
}) {
  const [editing, setEditing] = React.useState(false);
  const [title, setTitle] = React.useState("");
  const [description, setDescription] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [tagIDs, setTagIDs] = React.useState<number[]>([]);
  const [archived, setArchived] = React.useState(false);
  const [internalAccess, setInternalAccess] = React.useState(false);
  const [allowedHosts, setAllowedHosts] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const [editError, setEditError] = React.useState<string>();
  const [contentURL, setContentURL] = React.useState<string>();
  const [textPreview, setTextPreview] = React.useState("");
  const [contentError, setContentError] = React.useState<string>();
  const [versionBusy, setVersionBusy] = React.useState(false);
  const [apiOperationID, setAPIOperationID] = React.useState("");
  const [apiPathParams, setAPIPathParams] = React.useState("{}");
  const [apiQuery, setAPIQuery] = React.useState("{}");
  const [apiBody, setAPIBody] = React.useState("");
  const [apiResult, setAPIResult] = React.useState("");
  const [apiBusy, setAPIBusy] = React.useState(false);
  const versionInputRef = React.useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    if (!detail) return;
    setTitle(detail.asset.title);
    setDescription(detail.asset.description);
    setSummary(detail.asset.summary);
    setTagIDs(detail.asset.tags.map((tag) => tag.id));
    setArchived(Boolean(detail.asset.archived_at));
    setInternalAccess(detail.asset.internal_access);
    setAllowedHosts(detail.asset.allowed_hosts.join(", "));
    setEditError(undefined);
    setAPIOperationID("");
    setAPIPathParams("{}");
    setAPIQuery("{}");
    setAPIBody("");
    setAPIResult("");
  }, [detail]);

  React.useEffect(() => {
    setContentURL(undefined);
    setTextPreview("");
    setContentError(undefined);
    if (!detail) return;
    const asset = detail.asset;
    const previewable =
      asset.asset_type === "image" ||
      asset.mime_type === "application/pdf" ||
      asset.mime_type.startsWith("text/") ||
      ["application/json", "application/xml", "application/yaml", "application/x-yaml"].includes(asset.mime_type);
    if (!previewable) return;
    let objectURL: string | undefined;
    let cancelled = false;
    void api
      .workAssetContentBlob(asset.id)
      .then(async (blob) => {
        if (cancelled) return;
        if (asset.mime_type.startsWith("text/") || asset.mime_type.includes("json") || asset.mime_type.includes("yaml") || asset.mime_type.includes("xml")) {
          const text = await blob.text();
          if (!cancelled) setTextPreview(text.slice(0, 120_000));
        } else {
          objectURL = URL.createObjectURL(blob);
          setContentURL(objectURL);
        }
      })
      .catch((cause: Error) => {
        if (!cancelled) setContentError(cause.message);
      });
    return () => {
      cancelled = true;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [detail]);

  async function saveMetadata() {
    if (!detail || !title.trim()) return;
    setSaving(true);
    setEditError(undefined);
    try {
      await api.updateWorkAsset(detail.asset.id, {
        title: title.trim(),
        description,
        summary,
        tag_ids: tagIDs,
        archived,
        internal_access: internalAccess,
        allowed_hosts: allowedHosts
          .split(/[\n,]/)
          .map((item) => item.trim())
          .filter(Boolean),
      });
      toast.success("资料信息已保存");
      setEditing(false);
      await onChanged();
    } catch (cause) {
      setEditError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function uploadVersion(file: File) {
    if (!detail) return;
    setVersionBusy(true);
    try {
      await api.uploadWorkAssetVersion(detail.asset.id, file);
      toast.success("新版本已上传");
      await onChanged();
    } catch (cause) {
      toast.error(`版本上传失败：${(cause as Error).message}`);
    } finally {
      setVersionBusy(false);
    }
  }

  async function testAPI() {
    if (!detail) return;
    if (!apiOperationID.trim()) {
      setAPIResult("请选择 OpenAPI 中声明的操作 ID。");
      return;
    }
    setAPIBusy(true);
    setAPIResult("");
    try {
      const pathParams = parseJSONObject(apiPathParams, "路径参数");
      const query = parseJSONObject(apiQuery, "查询参数");
      const body = apiBody.trim() ? JSON.parse(apiBody) : undefined;
      const result = await api.testWorkAssetAPI(detail.asset.id, {
        operation_id: apiOperationID.trim(),
        path_params: pathParams,
        query,
        body,
      });
      setAPIResult(JSON.stringify(result, null, 2));
    } catch (cause) {
      setAPIResult((cause as Error).message);
    } finally {
      setAPIBusy(false);
    }
  }

  const selectedVersion = detail?.versions.find((version) => version.id === selectedVersionID) ?? detail?.versions[0];
  const inTrash = Boolean(detail?.asset.deleted_at);
  const apiOperationIDs = openAPIOperationIDs(selectedVersion?.openapi_content ?? "");

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[calc(100dvh-0.75rem)] overflow-y-auto sm:max-h-[92dvh] sm:max-w-5xl">
        {loading ? (
          <div className="space-y-3 py-10" aria-busy="true">
            <Skeleton className="h-10 w-2/3" />
            <Skeleton className="h-32 w-full" />
            <Skeleton className="h-64 w-full" />
          </div>
        ) : error ? (
          <WorkErrorState message={`资料详情加载失败：${error}`} onRetry={onRetry} />
        ) : detail ? (
          <>
            <DialogHeader className="pr-10">
              <div className="flex items-start gap-3">
                <span className="rounded-2xl bg-gradient-to-br from-violet-500/15 to-sky-400/10 p-3 text-violet-650">
                  {assetIcon(detail.asset.asset_type, "size-6")}
                </span>
                <div className="min-w-0 flex-1">
                  <DialogTitle className="break-words">{detail.asset.title}</DialogTitle>
                  <DialogDescription className="mt-1">
                    {ASSET_TYPE_LABELS[detail.asset.asset_type]} · v{detail.asset.current_version} ·{" "}
                    {formatBytes(detail.asset.size_bytes)} · 更新于 {formatTime(detail.asset.updated_at)}
                  </DialogDescription>
                </div>
                <Badge variant="outline" className={cn("border", PARSE_CLASSES[detail.asset.parse_status])}>
                  {PARSE_LABELS[detail.asset.parse_status]}
                </Badge>
              </div>
            </DialogHeader>

            <WorkAssetOrganizationPanel
              asset={detail.asset}
              sourceVersion={selectedVersion}
              onChanged={onChanged}
            />

            <div className="grid gap-4 lg:grid-cols-[minmax(0,1.5fr)_minmax(280px,0.8fr)]">
              <div className="space-y-4">
                <section className="rounded-[20px] border bg-white/65 p-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <h3 className="text-sm font-semibold">资料内容</h3>
                    {detail.versions.length > 1 && (
                      <NativeSelect
                        value={selectedVersionID ?? detail.versions[0]?.id ?? ""}
                        onChange={(event) => onSelectVersion(Number(event.target.value))}
                        className="[&>select]:h-10 [&>select]:rounded-xl"
                        aria-label="选择资料版本"
                      >
                        {detail.versions.map((version) => (
                          <option key={version.id} value={version.id}>
                            v{version.version_number} · {formatTime(version.created_at)}
                          </option>
                        ))}
                      </NativeSelect>
                    )}
                  </div>
                  <div className="mt-3">
                    <AssetPreview
                      asset={detail.asset}
                      contentURL={contentURL}
                      textPreview={textPreview}
                      contentError={contentError}
                      version={selectedVersion}
                      chunks={detail.chunks}
                    />
                  </div>
                </section>

                <section className="rounded-[20px] border bg-white/65 p-4">
                  <div className="flex items-center gap-2">
                    <History className="size-4 text-violet-600" />
                    <h3 className="text-sm font-semibold">提取片段</h3>
                    <span className="text-xs text-muted-foreground">{detail.chunks.length} 段</span>
                  </div>
                  <div className="mt-3 max-h-72 space-y-2 overflow-y-auto pr-1">
                    {detail.chunks.map((chunk) => (
                      <details key={chunk.id} className="rounded-xl border bg-background/60 px-3 py-2">
                        <summary className="cursor-pointer text-sm font-medium">
                          {chunk.locator || `片段 ${chunk.ordinal + 1}`}
                        </summary>
                        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words font-sans text-xs leading-6 text-foreground/75">
                          {chunk.content}
                        </pre>
                      </details>
                    ))}
                    {detail.chunks.length === 0 && (
                      <p className="text-sm text-muted-foreground">还没有可检索片段。可重新解析或手动补充摘要。</p>
                    )}
                  </div>
                </section>

                {detail.asset.asset_type === "api" && (
                  <section className="rounded-[20px] border bg-white/65 p-4">
                    <div className="flex items-center gap-2">
                      <Braces className="size-4 text-violet-600" />
                      <h3 className="text-sm font-semibold">接口测试</h3>
                    </div>
                    <p className="mt-2 text-xs leading-5 text-muted-foreground">
                      仅可调用当前 OpenAPI 版本中声明的只读操作。写操作必须由 Agent 发起并经过审批。
                    </p>
                    <div className="mt-3 grid gap-2">
                      <Label htmlFor="asset-api-operation">操作 ID</Label>
                      {apiOperationIDs.length > 0 ? (
                        <NativeSelect
                          id="asset-api-operation"
                          value={apiOperationID}
                          onChange={(event) => setAPIOperationID(event.target.value)}
                          className="[&>select]:h-11 [&>select]:rounded-xl"
                        >
                          <option value="">选择 OpenAPI 操作</option>
                          {apiOperationIDs.map((operationID) => (
                            <option key={operationID} value={operationID}>
                              {operationID}
                            </option>
                          ))}
                        </NativeSelect>
                      ) : (
                        <Input
                          id="asset-api-operation"
                          className="h-11 rounded-xl font-mono"
                          value={apiOperationID}
                          onChange={(event) => setAPIOperationID(event.target.value)}
                          placeholder="例如 listReports"
                        />
                      )}
                      <p className="text-xs text-muted-foreground">
                        未自动识别操作时，请填写 OpenAPI 文件中的 operationId。
                      </p>
                    </div>
                    <div className="mt-3 grid gap-3 sm:grid-cols-2">
                      <div className="grid gap-2">
                        <Label htmlFor="asset-api-path-params">路径参数 JSON</Label>
                        <Textarea
                          id="asset-api-path-params"
                          className="min-h-24 font-mono text-xs"
                          value={apiPathParams}
                          onChange={(event) => setAPIPathParams(event.target.value)}
                          placeholder='{"id":"123"}'
                        />
                      </div>
                      <div className="grid gap-2">
                        <Label htmlFor="asset-api-query">查询参数 JSON</Label>
                        <Textarea
                          id="asset-api-query"
                          className="min-h-24 font-mono text-xs"
                          value={apiQuery}
                          onChange={(event) => setAPIQuery(event.target.value)}
                          placeholder='{"limit":"20"}'
                        />
                      </div>
                    </div>
                    <div className="mt-3 grid gap-2">
                      <Label htmlFor="asset-api-body">请求体 JSON（可选）</Label>
                      <Textarea
                        id="asset-api-body"
                        className="min-h-24 font-mono text-xs"
                        value={apiBody}
                        onChange={(event) => setAPIBody(event.target.value)}
                        placeholder='{"key":"value"}'
                      />
                    </div>
                    <Button className="mt-2 h-11 rounded-xl" disabled={apiBusy} onClick={() => void testAPI()}>
                      {apiBusy ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                      发送测试请求
                    </Button>
                    {apiResult && (
                      <pre className="mt-3 max-h-64 overflow-auto rounded-xl bg-slate-950 p-3 text-xs leading-5 text-slate-100">
                        {apiResult}
                      </pre>
                    )}
                  </section>
                )}
              </div>

              <div className="space-y-4">
                <section className="rounded-[20px] border bg-white/65 p-4">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-sm font-semibold">资料信息</h3>
                    <Button variant="outline" className="h-10 rounded-xl" onClick={() => setEditing((value) => !value)}>
                      {editing ? "收起编辑" : "编辑"}
                    </Button>
                  </div>
                  {editing ? (
                    <div className="mt-3 space-y-3">
                      <div className="grid gap-2">
                        <Label htmlFor="asset-detail-title">标题</Label>
                        <Input id="asset-detail-title" value={title} onChange={(event) => setTitle(event.target.value)} />
                      </div>
                      <div className="grid gap-2">
                        <Label htmlFor="asset-detail-description">说明</Label>
                        <Textarea id="asset-detail-description" value={description} onChange={(event) => setDescription(event.target.value)} />
                      </div>
                      <div className="grid gap-2">
                        <Label htmlFor="asset-detail-summary">摘要</Label>
                        <Textarea id="asset-detail-summary" value={summary} onChange={(event) => setSummary(event.target.value)} />
                      </div>
                      <div className="grid gap-2">
                        <Label>标签</Label>
                        <TagPicker tags={tags} selected={tagIDs} onChange={setTagIDs} />
                      </div>
                      <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
                        <Checkbox checked={archived} onCheckedChange={(value) => setArchived(value === true)} />
                        归档这份资料
                      </label>
                      {(detail.asset.asset_type === "link" || detail.asset.asset_type === "api") && (
                        <>
                          <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
                            <Checkbox
                              checked={internalAccess}
                              onCheckedChange={(value) => setInternalAccess(value === true)}
                            />
                            允许访问内网白名单
                          </label>
                          {internalAccess && (
                            <Textarea
                              value={allowedHosts}
                              onChange={(event) => setAllowedHosts(event.target.value)}
                              placeholder="允许主机，逗号或换行分隔"
                            />
                          )}
                        </>
                      )}
                      {editError && <p className="text-sm text-destructive">{editError}</p>}
                      <Button className="h-11 w-full rounded-xl" disabled={saving || !title.trim()} onClick={() => void saveMetadata()}>
                        {saving && <Loader2 className="size-4 animate-spin" />}
                        保存资料信息
                      </Button>
                    </div>
                  ) : (
                    <div className="mt-3 space-y-3">
                      <p className="whitespace-pre-wrap text-sm leading-6 text-foreground/75">
                        {detail.asset.description || detail.asset.summary || "还没有说明。"}
                      </p>
                      {detail.asset.source_url && (
                        <a
                          href={detail.asset.source_url}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex min-h-11 items-center gap-2 text-sm text-violet-700 hover:underline"
                        >
                          <ExternalLink className="size-4" />
                          打开原始地址
                        </a>
                      )}
                      <div className="flex flex-wrap gap-1.5">
                        {detail.asset.tags.map((tag) => (
                          <Badge key={tag.id} variant="secondary">
                            {tag.name}
                          </Badge>
                        ))}
                      </div>
                      <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                        <span>文件：{detail.asset.file_name || "无"}</span>
                        <span>SHA-256：{detail.asset.sha256 ? `${detail.asset.sha256.slice(0, 12)}…` : "无"}</span>
                      </div>
                      {detail.asset.parse_error && (
                        <p className="rounded-xl border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive">
                          {detail.asset.parse_error}
                        </p>
                      )}
                    </div>
                  )}
                </section>

                <section className="rounded-[20px] border bg-white/65 p-4">
                  <div className="flex items-center gap-2">
                    <History className="size-4 text-violet-600" />
                    <h3 className="text-sm font-semibold">版本</h3>
                  </div>
                  <div className="mt-3 space-y-2">
                    {detail.versions.map((version) => (
                      <button
                        key={version.id}
                        type="button"
                        onClick={() => onSelectVersion(version.id)}
                        className={cn(
                          "flex min-h-11 w-full items-center justify-between rounded-xl border px-3 py-2 text-left text-sm",
                          version.id === selectedVersionID && "border-violet-300 bg-violet-500/[0.06]",
                        )}
                      >
                        <span className="flex flex-wrap items-center gap-2">
                          <span>版本 {version.version_number}</span>
                          {version.content_origin === "ai_organized" && (
                            <Badge
                              variant="outline"
                              className="border-violet-500/25 bg-violet-500/10 text-violet-700"
                            >
                              AI 整理
                            </Badge>
                          )}
                        </span>
                        <span className="text-right text-xs text-muted-foreground">
                          {version.confirmed_at
                            ? `确认于 ${formatTime(version.confirmed_at)}`
                            : formatTime(version.created_at)}
                        </span>
                      </button>
                    ))}
                  </div>
                  <input
                    ref={versionInputRef}
                    type="file"
                    className="hidden"
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      if (file) void uploadVersion(file);
                      event.target.value = "";
                    }}
                  />
                  <Button
                    variant="outline"
                    className="mt-3 h-11 w-full rounded-xl"
                    disabled={versionBusy || inTrash}
                    onClick={() => versionInputRef.current?.click()}
                  >
                    {versionBusy ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
                    上传新版本
                  </Button>
                </section>

                <div className="grid grid-cols-2 gap-2">
                  <Button
                    variant="outline"
                    className="h-11 rounded-xl"
                    disabled={busy || inTrash}
                    onClick={onReparse}
                  >
                    <RotateCw className="size-4" />
                    重新解析
                  </Button>
                  <Button
                    variant="outline"
                    className="h-11 rounded-xl"
                    disabled={busy}
                    onClick={() => {
                      void api.workAssetContentBlob(detail.asset.id).then((blob) => {
                        const url = URL.createObjectURL(blob);
                        const anchor = document.createElement("a");
                        anchor.href = url;
                        anchor.download = detail.asset.file_name || detail.asset.title;
                        anchor.click();
                        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
                      }).catch((cause: Error) => toast.error(`下载失败：${cause.message}`));
                    }}
                  >
                    <Download className="size-4" />
                    下载
                  </Button>
                  {inTrash ? (
                    <>
                      <Button variant="outline" className="h-11 rounded-xl" disabled={busy} onClick={onRestore}>
                        <ArchiveRestore className="size-4" />
                        恢复
                      </Button>
                      <Button variant="destructive" className="h-11 rounded-xl" disabled={busy} onClick={onPurge}>
                        <Trash2 className="size-4" />
                        彻底删除
                      </Button>
                    </>
                  ) : (
                    <Button variant="outline" className="col-span-2 h-11 rounded-xl text-destructive" disabled={busy} onClick={onTrash}>
                      <Trash2 className="size-4" />
                      移入回收站
                    </Button>
                  )}
                </div>
              </div>
            </div>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function AssetPreview({
  asset,
  contentURL,
  textPreview,
  contentError,
  version,
  chunks,
}: {
  asset: WorkAsset;
  contentURL?: string;
  textPreview: string;
  contentError?: string;
  version?: WorkAssetVersion;
  chunks: WorkAssetDetail["chunks"];
}) {
  if (contentError) {
    return <p className="rounded-xl bg-destructive/10 p-3 text-sm text-destructive">{contentError}</p>;
  }
  if (asset.asset_type === "image" && contentURL) {
    return (
      <div className="overflow-hidden rounded-[18px] border bg-muted/30">
        {/* A private authenticated response is converted to a short-lived blob URL. */}
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={contentURL} alt={asset.title} className="max-h-[420px] w-full object-contain" />
      </div>
    );
  }
  if (asset.mime_type === "application/pdf" && contentURL) {
    return <iframe title={`${asset.title} PDF 预览`} src={contentURL} className="h-[460px] w-full rounded-[18px] border" />;
  }
  if (textPreview) {
    return (
      <div className="max-h-[460px] overflow-auto rounded-[18px] border bg-background/70 p-4">
        <Markdown text={textPreview} />
      </div>
    );
  }
  if (version?.openapi_content) {
    return (
      <pre className="max-h-[460px] overflow-auto rounded-[18px] border bg-background/70 p-4 text-xs leading-5">
        {version.openapi_content}
      </pre>
    );
  }
  if (version?.extracted_text) {
    return (
      <pre className="max-h-[460px] overflow-auto whitespace-pre-wrap break-words rounded-[18px] border bg-background/70 p-4 font-sans text-sm leading-6">
        {version.extracted_text}
      </pre>
    );
  }
  if (chunks.length > 0) {
    return (
      <pre className="max-h-[460px] overflow-auto whitespace-pre-wrap break-words rounded-[18px] border bg-background/70 p-4 font-sans text-sm leading-6">
        {chunks.slice(0, 12).map((chunk) => `${chunk.locator ? `[${chunk.locator}]\n` : ""}${chunk.content}`).join("\n\n")}
      </pre>
    );
  }
  return (
    <div className="rounded-[18px] border border-dashed p-8 text-center text-sm text-muted-foreground">
      <File className="mx-auto mb-2 size-7" />
      这类资料暂不提供内嵌预览，可下载原文件或查看提取片段。
    </div>
  );
}

function parseJSONObject(value: string, label: string): Record<string, string> {
  if (!value.trim()) return {};
  const parsed: unknown = JSON.parse(value);
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error(`${label}必须是 JSON 对象`);
  }
  const result: Record<string, string> = {};
  for (const [key, item] of Object.entries(parsed)) {
    if (item !== null && typeof item === "object") {
      throw new Error(`${label}的 ${key} 必须是字符串、数字或布尔值`);
    }
    result[key] = item == null ? "" : String(item);
  }
  return result;
}

function openAPIOperationIDs(content: string) {
  const ids = new Set<string>();
  const patterns = [
    /["']?operationId["']?\s*:\s*["']([^"']+)["']/g,
    /^\s*operationId\s*:\s*["']?([\w.-]+)["']?\s*$/gm,
  ];
  for (const pattern of patterns) {
    for (const match of content.matchAll(pattern)) {
      const id = match[1]?.trim();
      if (id) ids.add(id);
    }
  }
  return [...ids].sort((left, right) => left.localeCompare(right));
}
