"use client";

import * as React from "react";
import {
  Archive,
  ArchiveRestore,
  FolderKanban,
  Loader2,
  Pencil,
  Plus,
  Tags,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

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
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { WorkProject, WorkTag, WorkTask } from "@/lib/types";
import { WORK_CHANGED_EVENT, emitWorkChanged } from "@/lib/work-events";

import { WorkEmptyState, WorkErrorState, WorkPageHeader } from "../_components/work-shell";

type ProjectForm = {
  name: string;
  color: string;
  description: string;
};

type TagForm = {
  name: string;
  color: string;
};

const defaultProjectForm: ProjectForm = {
  name: "",
  color: "#38bdf8",
  description: "",
};

const defaultTagForm: TagForm = {
  name: "",
  color: "#64748b",
};

export default function WorkProjectsPage() {
  const [projects, setProjects] = React.useState<WorkProject[]>([]);
  const [tasks, setTasks] = React.useState<WorkTask[]>([]);
  const [tags, setTags] = React.useState<WorkTag[]>([]);
  const [projectName, setProjectName] = React.useState("");
  const [tagName, setTagName] = React.useState("");
  const [tagColor, setTagColor] = React.useState(defaultTagForm.color);
  const [quickTitles, setQuickTitles] = React.useState<Record<number, string>>({});
  const [editingProject, setEditingProject] = React.useState<WorkProject | null>(null);
  const [projectForm, setProjectForm] = React.useState<ProjectForm>(defaultProjectForm);
  const [editingTag, setEditingTag] = React.useState<WorkTag | null>(null);
  const [tagForm, setTagForm] = React.useState<TagForm>(defaultTagForm);
  const [deleteProject, setDeleteProject] = React.useState<WorkProject | null>(null);
  const [deleteTag, setDeleteTag] = React.useState<WorkTag | null>(null);
  const [busy, setBusy] = React.useState<string>();
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const requestSequence = React.useRef(0);

  const load = React.useCallback(async (quiet = false) => {
    const requestID = ++requestSequence.current;
    if (!quiet) setLoading(true);
    try {
      const [projectList, taskList, tagList] = await Promise.all([
        api.workProjects(),
        api.workTasks(),
        api.workTags(),
      ]);
      if (requestID !== requestSequence.current) return;
      setProjects(projectList);
      setTasks(taskList);
      setTags(tagList);
      setError(undefined);
    } catch (cause) {
      if (requestID === requestSequence.current) setError((cause as Error).message);
    } finally {
      if (requestID === requestSequence.current) setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load, reloadKey]);

  React.useEffect(() => {
    const refresh = () => void load(true);
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") refresh();
    };
    window.addEventListener(WORK_CHANGED_EVENT, refresh);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      window.removeEventListener(WORK_CHANGED_EVENT, refresh);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
      requestSequence.current += 1;
    };
  }, [load]);

  async function createProject(event: React.FormEvent) {
    event.preventDefault();
    if (!projectName.trim()) return;
    setBusy("create-project");
    try {
      const created = await api.createWorkProject({ name: projectName.trim() });
      setProjects((items) => [...items, created].sort(compareProjects));
      setProjectName("");
      emitWorkChanged();
      toast.success("项目已创建");
    } catch (cause) {
      toast.error(`创建项目失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  function openProjectEditor(project: WorkProject) {
    setEditingProject(project);
    setProjectForm({
      name: project.name,
      color: project.color || defaultProjectForm.color,
      description: project.description || "",
    });
  }

  async function saveProject(event: React.FormEvent) {
    event.preventDefault();
    if (!editingProject || !projectForm.name.trim()) return;
    setBusy("save-project");
    try {
      const saved = await api.updateWorkProject(editingProject.id, {
        name: projectForm.name.trim(),
        color: projectForm.color,
        description: projectForm.description,
      });
      setProjects((items) =>
        items.map((project) => (project.id === saved.id ? saved : project)).sort(compareProjects),
      );
      setEditingProject(null);
      emitWorkChanged();
      toast.success("项目已更新");
    } catch (cause) {
      toast.error(`更新项目失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  async function toggleArchive(project: WorkProject) {
    const previous = projects;
    const nextArchived = !project.archived;
    setProjects((items) =>
      items
        .map((item) => (item.id === project.id ? { ...item, archived: nextArchived } : item))
        .sort(compareProjects),
    );
    try {
      const saved = await api.updateWorkProject(project.id, { archived: nextArchived });
      setProjects((items) =>
        items.map((item) => (item.id === saved.id ? saved : item)).sort(compareProjects),
      );
      emitWorkChanged();
      toast.success(nextArchived ? "项目已归档" : "项目已恢复");
    } catch (cause) {
      setProjects(previous);
      toast.error(`操作失败：${(cause as Error).message}`);
    }
  }

  async function removeProject() {
    if (!deleteProject) return;
    const target = deleteProject;
    setBusy(`delete-project-${target.id}`);
    try {
      await api.deleteWorkProject(target.id);
      setProjects((items) => items.filter((project) => project.id !== target.id));
      setTasks((items) =>
        items.map((task) =>
          task.project_id === target.id
            ? { ...task, project_id: null, project_name: null }
            : task,
        ),
      );
      setDeleteProject(null);
      emitWorkChanged();
      toast.success("项目已删除，原任务已转为未归属");
    } catch (cause) {
      toast.error(`删除项目失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  async function createTask(event: React.FormEvent, project: WorkProject) {
    event.preventDefault();
    const title = quickTitles[project.id]?.trim();
    if (!title) return;
    setBusy(`create-task-${project.id}`);
    try {
      const created = await api.createWorkTask({
        title,
        project_id: project.id,
      });
      setTasks((items) => [created, ...items]);
      setQuickTitles((items) => ({ ...items, [project.id]: "" }));
      emitWorkChanged();
      toast.success("待办已加入项目");
    } catch (cause) {
      toast.error(`创建待办失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  async function createTag(event: React.FormEvent) {
    event.preventDefault();
    if (!tagName.trim()) return;
    setBusy("create-tag");
    try {
      const created = await api.createWorkTag({
        name: tagName.trim(),
        color: tagColor,
      });
      setTags((items) => [...items, created].sort(compareTags));
      setTagName("");
      setTagColor(defaultTagForm.color);
      emitWorkChanged();
      toast.success("标签已创建");
    } catch (cause) {
      toast.error(`创建标签失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  function openTagEditor(tag: WorkTag) {
    setEditingTag(tag);
    setTagForm({ name: tag.name, color: tag.color || defaultTagForm.color });
  }

  async function saveTag(event: React.FormEvent) {
    event.preventDefault();
    if (!editingTag || !tagForm.name.trim()) return;
    setBusy("save-tag");
    try {
      const saved = await api.updateWorkTag(editingTag.id, {
        name: tagForm.name.trim(),
        color: tagForm.color,
      });
      setTags((items) =>
        items.map((tag) => (tag.id === saved.id ? saved : tag)).sort(compareTags),
      );
      setEditingTag(null);
      emitWorkChanged();
      toast.success("标签已更新");
    } catch (cause) {
      toast.error(`更新标签失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  async function removeTag() {
    if (!deleteTag) return;
    const target = deleteTag;
    setBusy(`delete-tag-${target.id}`);
    try {
      await api.deleteWorkTag(target.id);
      setTags((items) => items.filter((tag) => tag.id !== target.id));
      setTasks((items) =>
        items.map((task) => ({
          ...task,
          tags: task.tags.filter((tag) => tag.id !== target.id),
        })),
      );
      setDeleteTag(null);
      emitWorkChanged();
      toast.success("标签已删除");
    } catch (cause) {
      toast.error(`删除标签失败：${(cause as Error).message}`);
    } finally {
      setBusy(undefined);
    }
  }

  return (
    <div className="flex flex-1 flex-col gap-4 pb-6">
      <WorkPageHeader title="项目" description="组织个人待办、追踪完成进度，并维护跨项目标签。">
        <form onSubmit={createProject} className="flex w-full gap-2 sm:w-auto">
          <Input
            value={projectName}
            onChange={(event) => setProjectName(event.target.value)}
            placeholder="新项目名称"
            aria-label="新项目名称"
            className="min-w-0 sm:w-56"
          />
          <Button
            type="submit"
            className="min-h-10 sm:min-h-7"
            disabled={!projectName.trim() || busy === "create-project"}
          >
            {busy === "create-project" ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
            新建
          </Button>
        </form>
      </WorkPageHeader>

      {loading ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {[0, 1, 2].map((item) => <Skeleton key={item} className="h-56" />)}
        </div>
      ) : error ? (
        <WorkErrorState message={error} onRetry={() => setReloadKey((value) => value + 1)} />
      ) : (
        <>
          {projects.length === 0 ? (
            <WorkEmptyState title="还没有项目" description="创建项目后即可在项目内快速收集待办。" />
          ) : (
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {projects.map((project) => {
                const projectTasks = tasks.filter((task) => task.project_id === project.id);
                const doneCount = projectTasks.filter((task) => task.status === "done").length;
                const progress = projectTasks.length === 0 ? 0 : Math.round((doneCount / projectTasks.length) * 100);
                const creatingTask = busy === `create-task-${project.id}`;
                return (
                  <Card
                    key={project.id}
                    className={project.archived ? "opacity-75" : undefined}
                    style={{ borderLeft: `3px solid ${project.color || "#38bdf8"}` }}
                  >
                    <CardContent className="space-y-3">
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <FolderKanban className="size-4 shrink-0" style={{ color: project.color || "#38bdf8" }} />
                            <p className="truncate font-medium">{project.name}</p>
                            {project.archived && <Badge variant="secondary">已归档</Badge>}
                          </div>
                          <p className="mt-1 line-clamp-2 min-h-8 text-sm text-muted-foreground">
                            {project.description || "暂无描述"}
                          </p>
                        </div>
                        <div className="flex shrink-0 gap-0.5">
                          <Button
                            size="icon-sm"
                            className="size-10 sm:size-7"
                            variant="ghost"
                            onClick={() => openProjectEditor(project)}
                            aria-label={`编辑项目 ${project.name}`}
                          >
                            <Pencil className="size-3.5" />
                          </Button>
                          <Button
                            size="icon-sm"
                            className="size-10 sm:size-7"
                            variant="ghost"
                            onClick={() => void toggleArchive(project)}
                            aria-label={project.archived ? `恢复项目 ${project.name}` : `归档项目 ${project.name}`}
                          >
                            {project.archived ? <ArchiveRestore className="size-3.5" /> : <Archive className="size-3.5" />}
                          </Button>
                          <Button
                            size="icon-sm"
                            className="size-10 sm:size-7"
                            variant="ghost"
                            onClick={() => setDeleteProject(project)}
                            aria-label={`删除项目 ${project.name}`}
                          >
                            <Trash2 className="size-3.5 text-destructive" />
                          </Button>
                        </div>
                      </div>

                      <div className="space-y-1.5">
                        <div className="flex items-center justify-between text-xs text-muted-foreground">
                          <span>完成进度</span>
                          <span>{doneCount} / {projectTasks.length} · {progress}%</span>
                        </div>
                        <Progress value={progress} aria-label={`${project.name} 完成进度 ${progress}%`} />
                      </div>

                      {!project.archived ? (
                        <form onSubmit={(event) => void createTask(event, project)} className="flex gap-2">
                          <Input
                            value={quickTitles[project.id] || ""}
                            onChange={(event) =>
                              setQuickTitles((items) => ({ ...items, [project.id]: event.target.value }))
                            }
                            placeholder="快速添加待办"
                            aria-label={`向 ${project.name} 添加待办`}
                          />
                          <Button
                            type="submit"
                            size="icon"
                            disabled={!quickTitles[project.id]?.trim() || creatingTask}
                            aria-label="添加待办"
                          >
                            {creatingTask ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
                          </Button>
                        </form>
                      ) : (
                        <p className="text-xs text-muted-foreground">恢复项目后可继续添加待办。</p>
                      )}
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}

          <Card>
            <CardHeader className="border-b">
              <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <CardTitle className="flex items-center gap-2">
                  <Tags className="size-4" />
                  标签管理
                </CardTitle>
                <p className="text-xs text-muted-foreground">标签可同时用于个人待办和 Markdown 笔记。</p>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <form onSubmit={createTag} className="flex flex-col gap-2 sm:flex-row">
                <Input
                  value={tagName}
                  onChange={(event) => setTagName(event.target.value)}
                  placeholder="新标签名称"
                  aria-label="新标签名称"
                  className="min-w-0 flex-1"
                />
                <Input
                  type="color"
                  value={tagColor}
                  onChange={(event) => setTagColor(event.target.value)}
                  aria-label="新标签颜色"
                  className="h-10 w-12 cursor-pointer p-1"
                />
                <Button
                  type="submit"
                  className="min-h-10 sm:min-h-7"
                  disabled={!tagName.trim() || busy === "create-tag"}
                >
                  {busy === "create-tag" ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
                  添加标签
                </Button>
              </form>

              {tags.length === 0 ? (
                <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
                  暂无标签。
                </p>
              ) : (
                <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
                  {tags.map((tag) => (
                    <div
                      key={tag.id}
                      className="flex items-center gap-2 rounded-lg border px-3 py-2"
                    >
                      <span
                        className="size-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: tag.color }}
                        aria-hidden="true"
                      />
                      <span className="min-w-0 flex-1 truncate text-sm">{tag.name}</span>
                      <Button
                        size="icon-sm"
                        className="size-10 sm:size-7"
                        variant="ghost"
                        onClick={() => openTagEditor(tag)}
                        aria-label={`编辑标签 ${tag.name}`}
                      >
                        <Pencil className="size-3.5" />
                      </Button>
                      <Button
                        size="icon-sm"
                        className="size-10 sm:size-7"
                        variant="ghost"
                        onClick={() => setDeleteTag(tag)}
                        aria-label={`删除标签 ${tag.name}`}
                      >
                        <Trash2 className="size-3.5 text-destructive" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}

      <Dialog open={Boolean(editingProject)} onOpenChange={(open) => !open && setEditingProject(null)}>
        {editingProject && (
          <DialogContent>
            <form onSubmit={(event) => void saveProject(event)}>
              <DialogHeader>
                <DialogTitle>编辑项目</DialogTitle>
                <DialogDescription>更新项目名称、标识颜色和用途说明。</DialogDescription>
              </DialogHeader>
              <div className="my-4 space-y-3">
                <div className="grid gap-2">
                  <Label htmlFor="project-name">项目名称</Label>
                  <Input
                    id="project-name"
                    value={projectForm.name}
                    onChange={(event) => setProjectForm((form) => ({ ...form, name: event.target.value }))}
                    autoFocus
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="project-color">标识颜色</Label>
                  <Input
                    id="project-color"
                    type="color"
                    value={projectForm.color}
                    onChange={(event) => setProjectForm((form) => ({ ...form, color: event.target.value }))}
                    className="h-9 w-16 cursor-pointer p-1"
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="project-description">描述</Label>
                  <Textarea
                    id="project-description"
                    value={projectForm.description}
                    onChange={(event) =>
                      setProjectForm((form) => ({ ...form, description: event.target.value }))
                    }
                    placeholder="这个项目用于..."
                    className="min-h-24"
                  />
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setEditingProject(null)}>
                  取消
                </Button>
                <Button type="submit" disabled={!projectForm.name.trim() || busy === "save-project"}>
                  {busy === "save-project" && <Loader2 className="size-4 animate-spin" />}
                  保存
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        )}
      </Dialog>

      <Dialog open={Boolean(editingTag)} onOpenChange={(open) => !open && setEditingTag(null)}>
        {editingTag && (
          <DialogContent>
            <form onSubmit={(event) => void saveTag(event)}>
              <DialogHeader>
                <DialogTitle>编辑标签</DialogTitle>
                <DialogDescription>名称和颜色会同步到任务与笔记。</DialogDescription>
              </DialogHeader>
              <div className="my-4 space-y-3">
                <div className="grid gap-2">
                  <Label htmlFor="tag-name">标签名称</Label>
                  <Input
                    id="tag-name"
                    value={tagForm.name}
                    onChange={(event) => setTagForm((form) => ({ ...form, name: event.target.value }))}
                    autoFocus
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="tag-color">标签颜色</Label>
                  <Input
                    id="tag-color"
                    type="color"
                    value={tagForm.color}
                    onChange={(event) => setTagForm((form) => ({ ...form, color: event.target.value }))}
                    className="h-9 w-16 cursor-pointer p-1"
                  />
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setEditingTag(null)}>
                  取消
                </Button>
                <Button type="submit" disabled={!tagForm.name.trim() || busy === "save-tag"}>
                  {busy === "save-tag" && <Loader2 className="size-4 animate-spin" />}
                  保存
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        )}
      </Dialog>

      <AlertDialog
        open={Boolean(deleteProject)}
        onOpenChange={(open) => !open && setDeleteProject(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除项目？</AlertDialogTitle>
            <AlertDialogDescription>
              “{deleteProject?.name}”会永久删除。项目内待办会保留，并转为“不归属项目”。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={Boolean(busy)}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={Boolean(busy)}
              onClick={(event) => {
                event.preventDefault();
                void removeProject();
              }}
            >
              {busy?.startsWith("delete-project-") && <Loader2 className="size-4 animate-spin" />}
              删除项目
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={Boolean(deleteTag)}
        onOpenChange={(open) => !open && setDeleteTag(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除标签？</AlertDialogTitle>
            <AlertDialogDescription>
              “{deleteTag?.name}”会从所有待办和笔记中移除，但不会删除这些内容。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={Boolean(busy)}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={Boolean(busy)}
              onClick={(event) => {
                event.preventDefault();
                void removeTag();
              }}
            >
              {busy?.startsWith("delete-tag-") && <Loader2 className="size-4 animate-spin" />}
              删除标签
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function compareProjects(a: WorkProject, b: WorkProject) {
  return Number(a.archived) - Number(b.archived) || a.name.localeCompare(b.name, "zh-CN");
}

function compareTags(a: WorkTag, b: WorkTag) {
  return a.name.localeCompare(b.name, "zh-CN");
}
