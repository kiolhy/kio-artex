"use client";

import * as React from "react";

import {
  AlertCircle,
  Archive,
  ArchiveRestore,
  Check,
  Loader2,
  NotebookPen,
  Pin,
  PinOff,
  Plus,
  RotateCcw,
  Search,
  Trash2,
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
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { WorkNote, WorkTag } from "@/lib/types";
import { cn } from "@/lib/utils";

import { WorkEmptyState, WorkErrorState, WorkPageHeader } from "../_components/work-shell";

type SaveState = "idle" | "saving" | "saved" | "failed";
type NoteBaseline = { id: number; title: string; content: string };
const SAVE_LABELS: Record<SaveState, string> = {
  idle: "等待编辑",
  saving: "保存中",
  saved: "已保存",
  failed: "保存失败",
};

export default function WorkNotesPage() {
  const [notes, setNotes] = React.useState<WorkNote[]>([]);
  const [tags, setTags] = React.useState<WorkTag[]>([]);
  const [activeId, setActiveId] = React.useState<number | null>(null);
  const [draft, setDraft] = React.useState<WorkNote | null>(null);
  const [query, setQuery] = React.useState("");
  const [searchQuery, setSearchQuery] = React.useState("");
  const [tagFilter, setTagFilter] = React.useState("");
  const [archivedOnly, setArchivedOnly] = React.useState(false);
  const [view, setView] = React.useState<"edit" | "preview">("edit");
  const [listLoading, setListLoading] = React.useState(true);
  const [listError, setListError] = React.useState<string>();
  const [saveState, setSaveState] = React.useState<SaveState>("idle");
  const [saveError, setSaveError] = React.useState<string>();
  const [creating, setCreating] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);

  const draftRef = React.useRef<WorkNote | null>(null);
  const selectNoteRef = React.useRef<(note: WorkNote | null) => void>(() => undefined);
  const persistNoteRef = React.useRef<(note: WorkNote, revision: number) => Promise<void>>(async () => undefined);
  const activeIdRef = React.useRef<number | null>(null);
  const baselineRef = React.useRef<NoteBaseline | null>(null);
  const revisionRef = React.useRef(0);
  const saveTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const listRequestRef = React.useRef(0);
  const writeQueueRef = React.useRef<Promise<void>>(Promise.resolve());
  const metaMutationRef = React.useRef(new Map<number, number>());
  const tagMutationRef = React.useRef(new Map<number, number>());

  React.useEffect(() => {
    draftRef.current = draft;
  }, [draft]);

  React.useEffect(() => {
    activeIdRef.current = activeId;
  }, [activeId]);

  React.useEffect(() => {
    const timer = setTimeout(() => setSearchQuery(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  React.useEffect(() => {
    let cancelled = false;
    void api
      .workTags()
      .then((items) => {
        if (!cancelled) setTags(items);
      })
      .catch((error: unknown) => {
        if (!cancelled) toast.error(`标签加载失败：${(error as Error).message}`);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const load = React.useCallback(async () => {
    const requestID = ++listRequestRef.current;
    setListLoading(true);
    try {
      const list = await api.workNotes({
        q: searchQuery || undefined,
        archived: archivedOnly,
        tag_ids: tagFilter ? [Number(tagFilter)] : undefined,
      });
      if (requestID !== listRequestRef.current) return;
      setNotes(list);
      setListError(undefined);

      const target = list.find((note) => note.id === activeIdRef.current) ?? list[0] ?? null;
      const currentDraft = draftRef.current;
      const baseline = baselineRef.current;
      const keepDirtyDraft =
        target !== null &&
        currentDraft?.id === target.id &&
        baseline?.id === currentDraft.id &&
        (baseline.title !== currentDraft.title || baseline.content !== currentDraft.content);

      if (!keepDirtyDraft) {
        selectNoteRef.current(target);
      }
    } catch (error) {
      if (requestID === listRequestRef.current) setListError((error as Error).message);
    } finally {
      if (requestID === listRequestRef.current) setListLoading(false);
    }
  }, [archivedOnly, searchQuery, tagFilter]);

  React.useEffect(() => {
    void load();
  }, [load]);

  React.useEffect(() => {
    const draftID = draft?.id;
    const title = draft?.title;
    const content = draft?.content;
    const current = draftRef.current;
    if (!draftID || !current || current.id !== draftID) return;
    if (baselineRef.current?.id !== draftID) return;
    if (title === baselineRef.current.title && content === baselineRef.current.content) {
      setSaveState("saved");
      setSaveError(undefined);
      return;
    }

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    setSaveState("saving");
    const revision = revisionRef.current;
    const snapshot = { ...current, title: title ?? "", content: content ?? "" };
    saveTimerRef.current = setTimeout(() => {
      saveTimerRef.current = null;
      void persistNoteRef.current(snapshot, revision);
    }, 800);
  }, [draft?.id, draft?.title, draft?.content]);

  React.useEffect(
    () => () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      const current = draftRef.current;
      if (current && isDraftDirty(current, baselineRef.current)) {
        void enqueueSerializedWrite(writeQueueRef, () =>
          api
            .updateWorkNote(current.id, {
              title: current.title,
              content: current.content,
            })
            .catch(() => undefined),
        );
      }
    },
    [],
  );

  async function persistNote(note: WorkNote, revision: number) {
    await enqueueWrite(async () => {
      try {
        const saved = await api.updateWorkNote(note.id, {
          title: note.title,
          content: note.content,
        });
        const latest = draftRef.current;
        const merged = latest?.id === saved.id ? { ...saved, title: latest.title, content: latest.content } : saved;

        setNotes((items) =>
          items.map((item) =>
            item.id === saved.id
              ? {
                  ...item,
                  title: merged.title,
                  content: merged.content,
                  updated_at: saved.updated_at,
                }
              : item,
          ),
        );
        setDraft((current) => (current?.id === saved.id ? { ...current, updated_at: saved.updated_at } : current));

        if (revisionRef.current === revision && draftRef.current?.id === saved.id) {
          baselineRef.current = {
            id: saved.id,
            title: draftRef.current.title,
            content: draftRef.current.content,
          };
          setSaveState("saved");
          setSaveError(undefined);
        }
      } catch (error) {
        if (revisionRef.current === revision) {
          setSaveState("failed");
          setSaveError((error as Error).message);
        }
      }
    });
  }
  persistNoteRef.current = persistNote;

  function enqueueWrite<T>(operation: () => Promise<T>): Promise<T> {
    return enqueueSerializedWrite(writeQueueRef, operation);
  }

  function selectNote(note: WorkNote | null) {
    const current = draftRef.current;
    if (current && current.id !== note?.id && isDraftDirty(current, baselineRef.current)) {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
      void persistNote(current, revisionRef.current);
    }
    revisionRef.current += 1;
    activeIdRef.current = note?.id ?? null;
    draftRef.current = note ? { ...note } : null;
    baselineRef.current = note ? { id: note.id, title: note.title, content: note.content } : null;
    setActiveId(note?.id ?? null);
    setDraft(note ? { ...note } : null);
    setSaveState(note ? "saved" : "idle");
    setSaveError(undefined);
  }
  selectNoteRef.current = selectNote;

  function editDraft(patch: Partial<Pick<WorkNote, "title" | "content">>) {
    revisionRef.current += 1;
    setDraft((current) => {
      if (!current) return current;
      const next = { ...current, ...patch };
      draftRef.current = next;
      return next;
    });
    setSaveState("saving");
    setSaveError(undefined);
  }

  function openNote(note: WorkNote) {
    selectNote(note);
    if (window.matchMedia("(max-width: 1023px)").matches) {
      window.requestAnimationFrame(() => {
        document.getElementById("note-editor")?.scrollIntoView({
          behavior: "smooth",
          block: "start",
        });
      });
    }
  }

  async function createNote() {
    setCreating(true);
    try {
      const note = await api.createWorkNote({ title: "无标题笔记", content: "" });
      if (archivedOnly) setArchivedOnly(false);
      setNotes((items) => [note, ...items]);
      selectNote(note);
      setView("edit");
      toast.success("已创建笔记");
    } catch (error) {
      toast.error(`创建失败：${(error as Error).message}`);
    } finally {
      setCreating(false);
    }
  }

  async function updateMeta(patch: { pinned?: boolean; archived?: boolean }, success: string) {
    if (!draft) return;
    const noteID = draft.id;
    const mutation = (metaMutationRef.current.get(noteID) ?? 0) + 1;
    metaMutationRef.current.set(noteID, mutation);
    const previousPinned = (draftRef.current ?? draft).pinned;
    const previousArchived = (draftRef.current ?? draft).archived;
    setDraft((current) => {
      if (current?.id !== noteID) return current;
      const next = { ...current, ...patch };
      draftRef.current = next;
      return next;
    });
    setNotes((items) => items.map((note) => (note.id === noteID ? { ...note, ...patch } : note)));
    try {
      const saved = await enqueueWrite(() => api.updateWorkNote(noteID, patch));
      if (metaMutationRef.current.get(saved.id) !== mutation) return;
      setDraft((current) =>
        current?.id === saved.id ? { ...current, pinned: saved.pinned, archived: saved.archived } : current,
      );
      setNotes((items) =>
        items
          .map((note) => (note.id === saved.id ? { ...note, pinned: saved.pinned, archived: saved.archived } : note))
          .filter((note) => note.archived === archivedOnly),
      );
      toast.success(success);
      if (patch.archived !== undefined) void load();
    } catch (error) {
      if (metaMutationRef.current.get(noteID) === mutation) {
        const fresh = await api.workNote(noteID).catch(() => null);
        const pinned = fresh?.pinned ?? previousPinned;
        const archived = fresh?.archived ?? previousArchived;
        setDraft((current) => (current?.id === noteID ? { ...current, pinned, archived } : current));
        setNotes((items) => items.map((note) => (note.id === noteID ? { ...note, pinned, archived } : note)));
      }
      toast.error(`操作失败：${(error as Error).message}`);
      void load();
    }
  }

  async function toggleTag(tag: WorkTag) {
    if (!draft) return;
    const noteID = draft.id;
    const mutation = (tagMutationRef.current.get(noteID) ?? 0) + 1;
    tagMutationRef.current.set(noteID, mutation);
    const previous = (draftRef.current ?? draft).tags;
    const next = previous.some((item) => item.id === tag.id)
      ? previous.filter((item) => item.id !== tag.id)
      : [...previous, tag];
    setDraft((current) => {
      if (current?.id !== noteID) return current;
      const updated = { ...current, tags: next };
      draftRef.current = updated;
      return updated;
    });
    setNotes((items) => items.map((note) => (note.id === noteID ? { ...note, tags: next } : note)));
    setSaveState("saving");
    try {
      const saved = await enqueueWrite(() =>
        api.updateWorkNote(noteID, {
          tag_ids: next.map((item) => item.id),
        }),
      );
      if (tagMutationRef.current.get(saved.id) !== mutation) return;
      setDraft((current) => {
        if (current?.id !== saved.id) return current;
        const updated = { ...current, tags: saved.tags };
        draftRef.current = updated;
        return updated;
      });
      setNotes((items) => items.map((note) => (note.id === saved.id ? { ...note, tags: saved.tags } : note)));
      if (draftRef.current?.id === saved.id) {
        setSaveState("saved");
        setSaveError(undefined);
      }
    } catch (error) {
      if (tagMutationRef.current.get(noteID) === mutation) {
        const fresh = await api.workNote(noteID).catch(() => null);
        const tags = fresh?.tags ?? previous;
        setDraft((current) => {
          if (current?.id !== noteID) return current;
          const updated = { ...current, tags };
          draftRef.current = updated;
          return updated;
        });
        setNotes((items) => items.map((note) => (note.id === noteID ? { ...note, tags } : note)));
        if (draftRef.current?.id === noteID) {
          setSaveState("failed");
          setSaveError((error as Error).message);
        }
      }
    }
  }

  async function deleteNote() {
    if (!draft) return;
    setDeleting(true);
    try {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      saveTimerRef.current = null;
      await writeQueueRef.current;
      await api.deleteWorkNote(draft.id);
      const next = notes.find((note) => note.id !== draft.id) ?? null;
      setNotes((items) => items.filter((note) => note.id !== draft.id));
      selectNote(next);
      setDeleteOpen(false);
      toast.success("笔记已删除");
    } catch (error) {
      toast.error(`删除失败：${(error as Error).message}`);
    } finally {
      setDeleting(false);
    }
  }

  const saveLabel = SAVE_LABELS[saveState];
  let noteListContent: React.ReactNode;
  if (listLoading) {
    noteListContent = (
      <div className="space-y-2">
        {[0, 1, 2].map((item) => (
          <Skeleton key={item} className="h-16 w-full" />
        ))}
      </div>
    );
  } else if (listError) {
    noteListContent = <WorkErrorState message={listError} onRetry={() => void load()} />;
  } else if (notes.length === 0) {
    noteListContent = (
      <WorkEmptyState title={archivedOnly ? "暂无归档笔记" : "暂无笔记"} description="创建一篇笔记开始记录。" />
    );
  } else {
    noteListContent = notes.map((note) => (
      <button
        key={note.id}
        type="button"
        onClick={() => openNote(note)}
        aria-current={note.id === activeId ? "page" : undefined}
        className={cn(
          "min-h-16 w-full rounded-lg border border-transparent px-3 py-2.5 text-left transition-colors hover:bg-muted/70 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none",
          note.id === activeId && "border-border bg-muted",
        )}
      >
        <div className="flex items-center gap-2">
          {note.pinned && <Pin className="size-3.5 shrink-0 text-amber-700 dark:text-amber-400" aria-label="已收藏" />}
          <p className="min-w-0 flex-1 truncate text-sm font-medium">{note.title || "无标题"}</p>
        </div>
        <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
          {note.content?.slice(0, 100) || "空笔记"}
        </p>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {note.tags.slice(0, 2).map((tag) => (
            <Badge key={tag.id} variant="secondary" className="px-1.5 text-xs">
              {tag.name}
            </Badge>
          ))}
          <span className="ml-auto text-xs text-muted-foreground">
            {new Date(note.updated_at).toLocaleDateString()}
          </span>
        </div>
      </button>
    ));
  }

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <WorkPageHeader title="笔记" description="支持 Markdown、全文搜索、标签、收藏与归档的长期知识库。">
        <div className="relative w-full sm:w-64">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索标题或正文"
            className="pl-8"
            aria-label="搜索笔记"
          />
        </div>
        <NativeSelect
          value={tagFilter}
          onChange={(event) => setTagFilter(event.target.value)}
          className="w-full sm:w-36"
          aria-label="按标签筛选"
        >
          <option value="">全部标签</option>
          {tags.map((tag) => (
            <option key={tag.id} value={tag.id}>
              {tag.name}
            </option>
          ))}
        </NativeSelect>
        <Button variant={archivedOnly ? "secondary" : "outline"} onClick={() => setArchivedOnly((value) => !value)}>
          <Archive className="size-4" />
          {archivedOnly ? "查看当前" : "已归档"}
        </Button>
        <Button onClick={() => void createNote()} disabled={creating}>
          {creating ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
          新建笔记
        </Button>
      </WorkPageHeader>

      <div className="grid min-h-0 gap-4 lg:min-h-[520px] lg:grid-cols-[290px_minmax(0,1fr)]">
        <Card className="max-h-[42vh] overflow-hidden lg:max-h-[72vh]">
          <CardHeader className="border-b pb-3">
            <CardTitle className="flex items-center justify-between gap-2">
              <span>{archivedOnly ? "归档笔记" : "全部笔记"}</span>
              <span className="text-xs font-normal text-muted-foreground">{notes.length}</span>
            </CardTitle>
          </CardHeader>
          <CardContent className="max-h-[calc(42vh-4rem)] space-y-2 overflow-y-auto lg:max-h-[calc(72vh-4rem)]">
            {noteListContent}
          </CardContent>
        </Card>

        {draft ? (
          <Card id="note-editor" className="min-w-0 scroll-mt-4">
            <CardContent className="space-y-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <Input
                  value={draft.title}
                  onChange={(event) => editDraft({ title: event.target.value })}
                  className="h-10 min-w-0 flex-1 text-base font-medium"
                  aria-label="笔记标题"
                  placeholder="笔记标题"
                />
                <div className="flex items-center gap-1.5 self-end sm:self-auto">
                  <span
                    className={cn(
                      "mr-1 inline-flex items-center gap-1 text-xs text-muted-foreground",
                      saveState === "failed" && "text-destructive",
                    )}
                    role="status"
                    aria-live="polite"
                  >
                    {saveState === "saving" && <Loader2 className="size-3.5 animate-spin" />}
                    {saveState === "saved" && <Check className="size-3.5 text-emerald-700 dark:text-emerald-400" />}
                    {saveState === "failed" && <AlertCircle className="size-3.5" />}
                    {saveLabel}
                  </span>
                  <Button
                    size="icon-sm"
                    className="size-10 sm:size-7"
                    variant="outline"
                    onClick={() => void updateMeta({ pinned: !draft.pinned }, draft.pinned ? "已取消收藏" : "已收藏")}
                    aria-label={draft.pinned ? "取消收藏" : "收藏笔记"}
                  >
                    {draft.pinned ? <PinOff className="size-4" /> : <Pin className="size-4" />}
                  </Button>
                  <Button
                    size="icon-sm"
                    className="size-10 sm:size-7"
                    variant="outline"
                    onClick={() =>
                      void updateMeta({ archived: !draft.archived }, draft.archived ? "笔记已恢复" : "笔记已归档")
                    }
                    aria-label={draft.archived ? "恢复笔记" : "归档笔记"}
                  >
                    {draft.archived ? <ArchiveRestore className="size-4" /> : <Archive className="size-4" />}
                  </Button>
                  <Button
                    size="icon-sm"
                    className="size-10 sm:size-7"
                    variant="destructive"
                    onClick={() => setDeleteOpen(true)}
                    aria-label="删除笔记"
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </div>

              {saveState === "failed" && (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                  <span className="min-w-0 break-words">草稿已保留，自动保存失败：{saveError || "未知错误"}</span>
                  <Button
                    size="sm"
                    className="min-h-10 sm:min-h-7"
                    variant="outline"
                    onClick={() => {
                      setSaveError(undefined);
                      setSaveState("saving");
                      void persistNote(draft, revisionRef.current);
                    }}
                  >
                    <RotateCcw className="size-3.5" />
                    重试保存
                  </Button>
                </div>
              )}

              <Tabs value={view} onValueChange={(value) => setView(value as "edit" | "preview")}>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <TabsList>
                    <TabsTrigger value="edit">编辑</TabsTrigger>
                    <TabsTrigger value="preview">预览</TabsTrigger>
                  </TabsList>
                  <fieldset className="flex flex-wrap items-center gap-1.5 border-0 p-0">
                    <legend className="sr-only">笔记标签</legend>
                    {tags.length === 0 ? (
                      <span className="text-xs text-muted-foreground">暂无可用标签</span>
                    ) : (
                      tags.map((tag) => {
                        const selected = draft.tags.some((item) => item.id === tag.id);
                        return (
                          <div
                            key={tag.id}
                            className={cn(
                              "flex cursor-pointer items-center gap-1.5 rounded-full border px-2 py-1 text-xs transition-colors",
                              selected ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted/60",
                            )}
                          >
                            <Checkbox
                              checked={selected}
                              onCheckedChange={() => void toggleTag(tag)}
                              aria-label={`${tag.name} 标签`}
                            />
                            <span
                              className="size-2 rounded-full"
                              style={{ backgroundColor: tag.color }}
                              aria-hidden="true"
                            />
                            {tag.name}
                          </div>
                        );
                      })
                    )}
                  </fieldset>
                </div>

                <TabsContent value="edit" className="mt-2">
                  <Textarea
                    value={draft.content}
                    onChange={(event) => editDraft({ content: event.target.value })}
                    placeholder="在这里输入 Markdown..."
                    className="min-h-[280px] resize-y font-mono leading-relaxed lg:min-h-[48vh]"
                    aria-label="Markdown 正文"
                  />
                </TabsContent>
                <TabsContent value="preview" className="mt-2">
                  <div className="min-h-[280px] rounded-lg border bg-muted/20 p-4 lg:min-h-[48vh]">
                    <div className="mb-3 flex items-center gap-2 text-xs font-medium text-muted-foreground">
                      <NotebookPen className="size-4" />
                      Markdown 预览
                    </div>
                    <Markdown text={draft.content || "_暂无内容_"} />
                  </div>
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent className="flex min-h-[420px] flex-col items-center justify-center gap-3 text-center">
              <NotebookPen className="size-7 text-muted-foreground" />
              <div>
                <p className="font-medium">选择或新建一篇笔记</p>
                <p className="mt-1 text-sm text-muted-foreground">内容会自动保存到你的个人工作台。</p>
              </div>
              <Button onClick={() => void createNote()} disabled={creating}>
                <Plus className="size-4" />
                新建笔记
              </Button>
            </CardContent>
          </Card>
        )}
      </div>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除这篇笔记？</AlertDialogTitle>
            <AlertDialogDescription>“{draft?.title || "无标题"}”会永久删除，此操作无法撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleting}
              onClick={(event) => {
                event.preventDefault();
                void deleteNote();
              }}
            >
              {deleting && <Loader2 className="size-4 animate-spin" />}
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function isDraftDirty(draft: WorkNote, baseline: NoteBaseline | null) {
  return Boolean(
    baseline && baseline.id === draft.id && (baseline.title !== draft.title || baseline.content !== draft.content),
  );
}

function enqueueSerializedWrite<T>(queue: { current: Promise<void> }, operation: () => Promise<T>): Promise<T> {
  const result = queue.current.then(operation, operation);
  queue.current = result.then(
    () => undefined,
    () => undefined,
  );
  return result;
}
