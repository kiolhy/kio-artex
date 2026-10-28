"use client";

import * as React from "react";
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  FileJson2,
  Loader2,
  RefreshCw,
  ShieldAlert,
  Sparkles,
  Trash2,
  WandSparkles,
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type {
  WorkAsset,
  WorkAssetOrganization,
  WorkAssetOrganizationCard,
  WorkAssetOrganizationEvidence,
  WorkAssetOrganizationRef,
  WorkAssetOrganizationStatus,
  WorkAssetVersion,
} from "@/lib/types";
import { cn } from "@/lib/utils";

const STATUS_META: Record<
  Exclude<WorkAssetOrganizationStatus, "none">,
  { label: string; className: string }
> = {
  queued: { label: "已排队", className: "border-sky-500/25 bg-sky-500/10 text-sky-700" },
  processing: {
    label: "AI 整理中",
    className: "border-violet-500/25 bg-violet-500/10 text-violet-700",
  },
  draft: {
    label: "草稿待确认",
    className: "border-amber-500/25 bg-amber-500/10 text-amber-700",
  },
  failed: {
    label: "整理失败",
    className: "border-destructive/25 bg-destructive/10 text-destructive",
  },
  confirmed: {
    label: "已确认",
    className: "border-emerald-500/25 bg-emerald-500/10 text-emerald-700",
  },
  discarded: {
    label: "已丢弃",
    className: "border-slate-400/25 bg-slate-500/10 text-slate-600",
  },
};

type EditableCard = WorkAssetOrganizationCard;

function emptyCard(): EditableCard {
  return {
    summary: "",
    use_cases: [],
    key_facts: [],
    constraints: [],
    tags: [],
    keywords: [],
    agent_guidance: [],
    evidence: [],
    uncertainties: [],
    warnings: [],
  };
}

function normalizeCard(value: Partial<WorkAssetOrganizationCard> | null | undefined): EditableCard {
  const list = (items: unknown) =>
    Array.isArray(items)
      ? items.filter((item): item is string => typeof item === "string")
      : [];
  const evidence = Array.isArray(value?.evidence)
    ? value.evidence
        .filter(
          (item): item is WorkAssetOrganizationEvidence =>
            Boolean(item) && typeof item === "object",
        )
        .map((item) => ({
          quote: typeof item.quote === "string" ? item.quote : "",
          locator: typeof item.locator === "string" ? item.locator : "",
        }))
    : [];
  return {
    summary: typeof value?.summary === "string" ? value.summary : "",
    use_cases: list(value?.use_cases),
    key_facts: list(value?.key_facts),
    constraints: list(value?.constraints),
    tags: list(value?.tags),
    keywords: list(value?.keywords),
    agent_guidance: list(value?.agent_guidance),
    evidence,
    uncertainties: list(value?.uncertainties),
    warnings: list(value?.warnings),
  };
}

function splitLines(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseEvidence(value: string): WorkAssetOrganizationEvidence[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error("证据必须是有效的 JSON 数组");
  }
  if (!Array.isArray(parsed)) throw new Error("证据必须是 JSON 数组");
  return parsed.map((item, index) => {
    if (!item || Array.isArray(item) || typeof item !== "object") {
      throw new Error(`第 ${index + 1} 条证据格式无效`);
    }
    const record = item as Record<string, unknown>;
    if (typeof record.quote !== "string" || !record.quote.trim()) {
      throw new Error(`第 ${index + 1} 条证据缺少 quote`);
    }
    if (record.locator != null && typeof record.locator !== "string") {
      throw new Error(`第 ${index + 1} 条证据的 locator 必须是文本`);
    }
    return {
      quote: record.quote.trim(),
      locator: typeof record.locator === "string" ? record.locator.trim() : "",
    };
  });
}

function cardMarkdown(card: EditableCard) {
  const section = (title: string, items: string[]) =>
    items.length ? `\n### ${title}\n${items.map((item) => `- ${item}`).join("\n")}` : "";
  const evidence = card.evidence.length
    ? `\n### 证据\n${card.evidence
        .map(
          (item) =>
            `- ${item.locator ? `**${item.locator}**：` : ""}${item.quote}`,
        )
        .join("\n")}`
    : "";
  return [
    `## ${card.summary || "尚未填写摘要"}`,
    section("适用场景", card.use_cases),
    section("关键事实", card.key_facts),
    section("约束与注意事项", card.constraints),
    section("Agent 使用建议", card.agent_guidance),
    evidence,
    section("不确定项", card.uncertainties),
    section("风险警告", card.warnings),
  ].join("\n");
}

export function WorkAssetOrganizationPanel({
  asset,
  sourceVersion,
  onChanged,
}: {
  asset: WorkAsset;
  sourceVersion?: WorkAssetVersion;
  onChanged: () => Promise<void>;
}) {
  const [organizations, setOrganizations] = React.useState<WorkAssetOrganizationRef[]>([]);
  const [selected, setSelected] = React.useState<WorkAssetOrganization | null>(null);
  const [card, setCard] = React.useState<EditableCard>(emptyCard);
  const [evidenceText, setEvidenceText] = React.useState("[]");
  const [openapiText, setOpenapiText] = React.useState("{}");
  const [mode, setMode] = React.useState<"edit" | "preview">("preview");
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [busy, setBusy] = React.useState<
    "organize" | "save" | "confirm" | "discard" | null
  >(null);
  const [discardOpen, setDiscardOpen] = React.useState(false);
  const [reloadKey, setReloadKey] = React.useState(0);
  const selectedIDRef = React.useRef<number | null>(null);
  const previousStatusRef = React.useRef<string | null>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const list = await api.workAssetOrganizations(asset.id);
      setOrganizations(list);
      const targetID = selectedIDRef.current;
      const target = list.find((item) => item.id === targetID) ?? list[0];
      if (!target) {
        setSelected(null);
        selectedIDRef.current = null;
        return;
      }
      const full = await api.workAssetOrganization(asset.id, target.id);
      selectedIDRef.current = full.id;
      setSelected(full);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, [asset.id]);

  React.useEffect(() => {
    selectedIDRef.current = null;
    previousStatusRef.current = null;
    setOrganizations([]);
    setSelected(null);
    void load();
  }, [asset.id, load]);

  React.useEffect(() => {
    if (reloadKey > 0) void load();
  }, [load, reloadKey]);

  React.useEffect(() => {
    if (!selected) return;
    const normalized = normalizeCard(selected.draft_card);
    setCard(normalized);
    setEvidenceText(JSON.stringify(normalized.evidence, null, 2));
    setOpenapiText(JSON.stringify(selected.draft_openapi ?? {}, null, 2));
    setMode(selected.status === "draft" ? "preview" : "preview");
  }, [selected]);

  React.useEffect(() => {
    if (!selected || !["queued", "processing"].includes(selected.status)) return;
    const timer = window.setTimeout(() => setReloadKey((value) => value + 1), 1800);
    return () => window.clearTimeout(timer);
  }, [selected]);

  React.useEffect(() => {
    if (!selected) return;
    const previous = previousStatusRef.current;
    previousStatusRef.current = selected.status;
    if (
      previous &&
      previous !== selected.status &&
      ["draft", "failed", "confirmed", "discarded"].includes(selected.status)
    ) {
      void onChanged();
    }
  }, [onChanged, selected]);

  async function selectOrganization(id: number) {
    selectedIDRef.current = id;
    setLoading(true);
    setError(undefined);
    try {
      setSelected(await api.workAssetOrganization(asset.id, id));
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function organize() {
    setBusy("organize");
    try {
      const queued = await api.organizeWorkAsset(asset.id, sourceVersion?.id);
      selectedIDRef.current = queued.id;
      toast.success("AI 整理任务已排队，生成后会保留为草稿等待确认");
      await load();
      await onChanged();
    } catch (cause) {
      toast.error(`提交 AI 整理失败：${(cause as Error).message}`);
    } finally {
      setBusy(null);
    }
  }

  function currentDraft() {
    const summary = card.summary.trim();
    if (!summary) throw new Error("资料卡摘要不能为空");
    const evidence = parseEvidence(evidenceText);
    let openapi: unknown = {};
    if (asset.asset_type === "api") {
      try {
        openapi = JSON.parse(openapiText);
      } catch {
        throw new Error("OpenAPI 必须是有效 JSON");
      }
      if (!openapi || Array.isArray(openapi) || typeof openapi !== "object") {
        throw new Error("OpenAPI 根节点必须是 JSON 对象");
      }
    }
    return {
      draft_card: {
        ...card,
        summary,
        evidence,
      },
      draft_openapi: openapi,
    };
  }

  async function saveDraft() {
    if (!selected) return;
    setBusy("save");
    try {
      const updated = await api.updateWorkAssetOrganization(
        asset.id,
        selected.id,
        currentDraft(),
      );
      setSelected(updated);
      toast.success("整理草稿已保存");
      await onChanged();
    } catch (cause) {
      toast.error(`保存草稿失败：${(cause as Error).message}`);
    } finally {
      setBusy(null);
    }
  }

  async function confirmDraft() {
    if (!selected) return;
    setBusy("confirm");
    try {
      const input = currentDraft();
      const result = await api.confirmWorkAssetOrganization(
        asset.id,
        selected.id,
        input,
      );
      toast.success(`已确认并创建 v${result.version.version_number} 整理版本`);
      selectedIDRef.current = result.organization.id;
      await load();
      await onChanged();
    } catch (cause) {
      toast.error(`确认整理版本失败：${(cause as Error).message}`);
    } finally {
      setBusy(null);
    }
  }

  async function discardDraft() {
    if (!selected) return;
    setBusy("discard");
    try {
      await api.discardWorkAssetOrganization(asset.id, selected.id);
      toast.success("整理草稿已丢弃，当前可用版本未改变");
      setDiscardOpen(false);
      await load();
      await onChanged();
    } catch (cause) {
      toast.error(`丢弃草稿失败：${(cause as Error).message}`);
    } finally {
      setBusy(null);
    }
  }

  const status = selected?.status ?? asset.organization_status;
  const statusMeta = status !== "none" ? STATUS_META[status] : null;
  const canOrganize =
    asset.can_organize &&
    status !== "queued" &&
    status !== "processing" &&
    status !== "draft";

  return (
    <section className="rounded-[20px] border border-violet-200/70 bg-gradient-to-br from-violet-50/90 via-white/80 to-sky-50/60 p-4 shadow-[0_20px_50px_-38px_rgba(91,67,196,0.55)]">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="rounded-2xl bg-violet-600 p-2.5 text-white shadow-lg shadow-violet-500/20">
            <WandSparkles className="size-5" />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-semibold">AI 整理与接口规范化</h3>
              {statusMeta && (
                <Badge variant="outline" className={cn("border", statusMeta.className)}>
                  {statusMeta.label}
                </Badge>
              )}
            </div>
            <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
              AI 只生成草稿，不会自动覆盖原文件。确认后会创建不可变整理版本，并参与后续任务资料召回。
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {organizations.length > 1 && (
            <NativeSelect
              value={selected?.id ?? ""}
              onChange={(event) => void selectOrganization(Number(event.target.value))}
              className="[&>select]:h-11 [&>select]:rounded-xl"
              aria-label="选择 AI 整理任务"
            >
              {organizations.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.trigger === "automatic" ? "自动" : "手动"} ·{" "}
                  {STATUS_META[item.status].label} · {formatShortTime(item.created_at)}
                </option>
              ))}
            </NativeSelect>
          )}
          {canOrganize && (
            <Button
              className="h-11 rounded-xl"
              disabled={busy !== null || Boolean(asset.deleted_at)}
              onClick={() => void organize()}
            >
              {busy === "organize" ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Sparkles className="size-4" />
              )}
              {status === "failed" || status === "discarded" ? "重新整理" : "AI 整理"}
            </Button>
          )}
        </div>
      </div>

      {(asset.asset_type === "api" || asset.asset_type === "link") && (
        <p className="mt-3 flex items-start gap-2 rounded-xl border border-sky-200/70 bg-sky-50/80 px-3 py-2 text-xs leading-5 text-sky-800">
          <Bot className="mt-0.5 size-4 shrink-0" />
          链接和接口在解析完成后会自动生成一次整理草稿；源版本变化后会为新版本再次排队。
        </p>
      )}

      {loading && !selected ? (
        <div className="mt-4 space-y-2" aria-busy="true">
          <Skeleton className="h-16 rounded-2xl" />
          <Skeleton className="h-28 rounded-2xl" />
        </div>
      ) : error ? (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive">
          <span>整理状态加载失败：{error}</span>
          <Button variant="outline" className="h-11 rounded-xl" onClick={() => void load()}>
            <RefreshCw className="size-4" />
            重试
          </Button>
        </div>
      ) : selected ? (
        <div className="mt-4 space-y-4">
          {selected.status === "queued" && (
            <div className="flex items-start gap-3 rounded-2xl border bg-white/75 p-3 text-sm">
              <Loader2 className="mt-0.5 size-4 animate-spin text-sky-600" />
              <div>
                <p className="font-medium">等待后台整理</p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  任务已持久化，服务重启后仍会继续处理。
                </p>
              </div>
            </div>
          )}
          {selected.status === "processing" && (
            <div className="flex items-start gap-3 rounded-2xl border border-violet-200 bg-white/75 p-3 text-sm">
              <Loader2 className="mt-0.5 size-4 animate-spin text-violet-600" />
              <div>
                <p className="font-medium">正在生成资料卡{asset.asset_type === "api" ? "和 OpenAPI" : ""}</p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  页面可以关闭，稍后回来会恢复当前整理状态。
                </p>
              </div>
            </div>
          )}
          {selected.status === "failed" && (
            <div className="rounded-2xl border border-destructive/25 bg-destructive/5 p-3">
              <div className="flex items-start gap-2 text-sm text-destructive">
                <ShieldAlert className="mt-0.5 size-4 shrink-0" />
                <div>
                  <p className="font-medium">整理失败，当前可用版本未受影响</p>
                  <p className="mt-1 break-words text-xs leading-5">
                    {selected.error || asset.organization_error || "未返回具体错误"}
                  </p>
                </div>
              </div>
              <Button
                variant="outline"
                className="mt-3 h-11 rounded-xl"
                disabled={busy !== null}
                onClick={() => void organize()}
              >
                <RefreshCw className="size-4" />
                重新生成草稿
              </Button>
            </div>
          )}
          {selected.status === "discarded" && (
            <div className="rounded-2xl border bg-white/70 p-3 text-sm text-muted-foreground">
              这份草稿已丢弃，当前可用版本和任务引用未改变。
            </div>
          )}
          {selected.status === "confirmed" && (
            <div className="flex items-start gap-2 rounded-2xl border border-emerald-200 bg-emerald-50/80 p-3 text-sm text-emerald-800">
              <CheckCircle2 className="mt-0.5 size-4 shrink-0" />
              <div>
                <p className="font-medium">整理版本已确认</p>
                <p className="mt-1 text-xs leading-5">
                  当前资产已切换到确认后的不可变版本。历史任务仍保留启动时的旧快照。
                </p>
              </div>
            </div>
          )}

          {selected.status === "draft" && (
            <>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="inline-flex rounded-xl border bg-white p-1" role="group" aria-label="整理草稿显示模式">
                  <button
                    type="button"
                    className={cn(
                      "min-h-10 rounded-lg px-3 text-sm font-medium transition-colors",
                      mode === "preview" && "bg-violet-600 text-white shadow-sm",
                    )}
                    aria-pressed={mode === "preview"}
                    onClick={() => setMode("preview")}
                  >
                    预览
                  </button>
                  <button
                    type="button"
                    className={cn(
                      "min-h-10 rounded-lg px-3 text-sm font-medium transition-colors",
                      mode === "edit" && "bg-violet-600 text-white shadow-sm",
                    )}
                    aria-pressed={mode === "edit"}
                    onClick={() => setMode("edit")}
                  >
                    编辑
                  </button>
                </div>
                <span className="text-xs text-muted-foreground">
                  来源 v{selected.source_version_number} ·{" "}
                  {selected.trigger === "automatic" ? "自动生成" : "手动生成"}
                </span>
              </div>

              {mode === "preview" ? (
                <div className="grid gap-3 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
                  <OriginalReference asset={asset} version={sourceVersion} />
                  <div className="min-w-0 rounded-2xl border bg-white/80 p-4">
                    <div className="mb-3 flex items-center gap-2">
                      <FileJson2 className="size-4 text-violet-600" />
                      <h4 className="text-sm font-semibold">整理结果预览</h4>
                    </div>
                    <div className="max-h-[420px] overflow-auto pr-1">
                      <Markdown text={cardMarkdown(card)} />
                      {asset.asset_type === "api" && (
                        <details className="mt-4 rounded-xl border bg-background/70 p-3">
                          <summary className="min-h-9 cursor-pointer text-sm font-medium">
                            OpenAPI 3.x 规范
                          </summary>
                          <pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words text-xs leading-5">
                            {openapiText}
                          </pre>
                        </details>
                      )}
                    </div>
                  </div>
                </div>
              ) : (
                <DraftEditor
                  assetType={asset.asset_type}
                  card={card}
                  evidenceText={evidenceText}
                  openapiText={openapiText}
                  onCardChange={setCard}
                  onEvidenceChange={setEvidenceText}
                  onOpenAPIChange={setOpenapiText}
                />
              )}

              <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                <Button
                  variant="outline"
                  className="h-11 rounded-xl text-destructive"
                  disabled={busy !== null}
                  onClick={() => setDiscardOpen(true)}
                >
                  {busy === "discard" ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Trash2 className="size-4" />
                  )}
                  丢弃草稿
                </Button>
                <Button
                  variant="outline"
                  className="h-11 rounded-xl"
                  disabled={busy !== null}
                  onClick={() => void saveDraft()}
                >
                  {busy === "save" && <Loader2 className="size-4 animate-spin" />}
                  保存草稿
                </Button>
                <Button
                  className="h-11 rounded-xl"
                  disabled={busy !== null}
                  onClick={() => void confirmDraft()}
                >
                  {busy === "confirm" ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <CheckCircle2 className="size-4" />
                  )}
                  确认并创建版本
                </Button>
              </div>
              <p className="flex items-start gap-2 text-xs leading-5 text-muted-foreground">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-600" />
                请核对证据、风险警告和 AI 推断项。确认操作会切换当前资料版本，但不会自动重跑已经启动的任务。
              </p>
            </>
          )}
        </div>
      ) : (
        <div className="mt-4 rounded-2xl border border-dashed bg-white/65 p-5 text-center">
          <p className="text-sm font-medium">还没有 AI 整理草稿</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            可手动生成统一资料卡；接口资产会同时生成可校验、可预览的 OpenAPI 规范。
          </p>
        </div>
      )}

      <AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>丢弃这次 AI 整理草稿？</AlertDialogTitle>
            <AlertDialogDescription>
              草稿内容会被标记为已丢弃，当前资产版本、检索片段和任务执行快照都不会改变。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy !== null}>保留草稿</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busy !== null}
              onClick={(event) => {
                event.preventDefault();
                void discardDraft();
              }}
            >
              {busy === "discard" && <Loader2 className="size-4 animate-spin" />}
              丢弃草稿
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

function DraftEditor({
  assetType,
  card,
  evidenceText,
  openapiText,
  onCardChange,
  onEvidenceChange,
  onOpenAPIChange,
}: {
  assetType: WorkAsset["asset_type"];
  card: EditableCard;
  evidenceText: string;
  openapiText: string;
  onCardChange: (value: EditableCard) => void;
  onEvidenceChange: (value: string) => void;
  onOpenAPIChange: (value: string) => void;
}) {
  function setList(
    key: "use_cases" | "key_facts" | "constraints" | "tags" | "keywords" | "agent_guidance" | "uncertainties" | "warnings",
    value: string,
  ) {
    onCardChange({ ...card, [key]: splitLines(value) });
  }
  return (
    <div className="grid gap-3 xl:grid-cols-2">
      <div className="space-y-3 rounded-2xl border bg-white/80 p-4">
        <div className="grid gap-2">
          <Label htmlFor="organization-summary">摘要</Label>
          <Textarea
            id="organization-summary"
            className="min-h-24"
            value={card.summary}
            onChange={(event) => onCardChange({ ...card, summary: event.target.value })}
          />
        </div>
        <ListField id="organization-use-cases" label="适用场景" value={card.use_cases} onChange={(value) => setList("use_cases", value)} />
        <ListField id="organization-key-facts" label="关键事实" value={card.key_facts} onChange={(value) => setList("key_facts", value)} />
        <ListField id="organization-constraints" label="约束与注意事项" value={card.constraints} onChange={(value) => setList("constraints", value)} />
        <ListField id="organization-agent-guidance" label="Agent 使用建议" value={card.agent_guidance} onChange={(value) => setList("agent_guidance", value)} />
        <div className="grid gap-3 sm:grid-cols-2">
          <ListField id="organization-tags" label="标签" value={card.tags} onChange={(value) => setList("tags", value)} compact />
          <ListField id="organization-keywords" label="关键词" value={card.keywords} onChange={(value) => setList("keywords", value)} compact />
        </div>
      </div>
      <div className="space-y-3 rounded-2xl border bg-white/80 p-4">
        <div className="grid gap-2">
          <Label htmlFor="organization-evidence">证据 JSON</Label>
          <Textarea
            id="organization-evidence"
            className="min-h-40 font-mono text-xs leading-5"
            value={evidenceText}
            onChange={(event) => onEvidenceChange(event.target.value)}
            spellCheck={false}
          />
          <p className="text-xs leading-5 text-muted-foreground">
            每条证据使用 {"{"}&quot;quote&quot;,&quot;locator&quot;{"}"}，引用必须来自原始资料。
          </p>
        </div>
        <ListField id="organization-uncertainties" label="不确定项" value={card.uncertainties} onChange={(value) => setList("uncertainties", value)} />
        <ListField id="organization-warnings" label="风险警告" value={card.warnings} onChange={(value) => setList("warnings", value)} />
        {assetType === "api" && (
          <div className="grid gap-2">
            <Label htmlFor="organization-openapi">OpenAPI JSON</Label>
            <Textarea
              id="organization-openapi"
              className="min-h-72 font-mono text-xs leading-5"
              value={openapiText}
              onChange={(event) => onOpenAPIChange(event.target.value)}
              spellCheck={false}
            />
            <p className="text-xs leading-5 text-muted-foreground">
              确认时会再次校验版本、操作 ID、响应、服务地址、允许主机和密钥别名。
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function ListField({
  id,
  label,
  value,
  onChange,
  compact = false,
}: {
  id: string;
  label: string;
  value: string[];
  onChange: (value: string) => void;
  compact?: boolean;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Textarea
        id={id}
        className={compact ? "min-h-20 text-sm" : "min-h-24 text-sm"}
        value={value.join("\n")}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  );
}

function OriginalReference({
  asset,
  version,
}: {
  asset: WorkAsset;
  version?: WorkAssetVersion;
}) {
  const content =
    version?.openapi_content ||
    version?.extracted_text ||
    asset.summary ||
    "当前版本没有可展示的文本内容。";
  return (
    <div className="min-w-0 rounded-2xl border bg-white/70 p-4">
      <div className="mb-3 flex items-center gap-2">
        <FileJson2 className="size-4 text-sky-600" />
        <h4 className="text-sm font-semibold">原始资料</h4>
      </div>
      <pre className="max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-xl bg-background/70 p-3 font-sans text-xs leading-6 text-foreground/75">
        {content.slice(0, 80_000)}
      </pre>
    </div>
  );
}

function formatShortTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "时间未知";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
