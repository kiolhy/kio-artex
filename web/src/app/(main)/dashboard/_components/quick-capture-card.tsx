"use client";

import * as React from "react";

import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import type { WorkProject } from "@/lib/types";
import { emitWorkChanged } from "@/lib/work-events";
import { parseWorkQuickCapture } from "@/lib/work-quick-capture";

import { CardError, CardLoading } from "./dashboard-states";

export function QuickCaptureCard({ onCreated }: { onCreated: () => void }) {
  const [value, setValue] = React.useState("");
  const [projects, setProjects] = React.useState<WorkProject[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [creating, setCreating] = React.useState(false);
  const [message, setMessage] = React.useState<string>();

  const reload = React.useCallback(() => {
    void (async () => {
      setLoading(true);
      try {
        setProjects(await api.workProjects());
        setError(undefined);
      } catch (e) {
        setError((e as Error).message);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  React.useEffect(reload, [reload]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const parsed = parseWorkQuickCapture(value, projects);
    if (!parsed.title) return;
    setCreating(true);
    try {
      const created = await api.createWorkTask({
        title: parsed.title,
        due_at: parsed.due_at,
        priority: parsed.priority,
        project_id: parsed.project_id,
      });
      setValue("");
      setMessage(`已加入自动化队列：${created.title}`);
      emitWorkChanged();
      onCreated();
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setCreating(false);
    }
  }

  return (
    <Card className="h-full">
      <CardHeader className="pb-0">
        <div>
          <p className="text-sm font-semibold">快捷记录</p>
          <p className="text-xs text-muted-foreground">一行创建待办，支持项目名、今天、明天和 !high。</p>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <CardLoading rows={2} />
        ) : error ? (
          <CardError message={error} onRetry={reload} />
        ) : (
          <form onSubmit={submit} className="flex flex-col gap-2">
            <Input
              className="h-11 sm:h-8"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              placeholder="写周报 #项目 明天 !high"
            />
            <Button type="submit" size="sm" className="h-11 sm:h-7" disabled={creating || !value.trim()}>
              <Plus className="size-4" />
              添加待办
            </Button>
            {message && <p className="text-xs text-muted-foreground">{message}</p>}
          </form>
        )}
      </CardContent>
    </Card>
  );
}
