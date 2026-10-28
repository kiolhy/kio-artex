"use client";

import * as React from "react";
import { AlertCircleIcon, RefreshCwIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ExplorationGraph } from "@/components/exploration-graph";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { Edge, TaskNode } from "@/lib/types";

export function GraphTab({ taskId }: { taskId: string }) {
  const [nodes, setNodes] = React.useState<TaskNode[]>([]);
  const [edges, setEdges] = React.useState<Edge[]>([]);
  const [loaded, setLoaded] = React.useState(false);
  const [loadError, setLoadError] = React.useState("");
  const [retryNonce, setRetryNonce] = React.useState(0);
  // 上一次图数据的签名:轮询拿到相同数据时跳过 setState,避免整图无谓重建(拖动时
  // 才不会被 20s 轮询打断而顿挫)。只取影响渲染的字段。
  const sigRef = React.useRef("");

  React.useEffect(() => {
    let cancelled = false;
    sigRef.current = ""; // 换任务:强制下一次刷新
    const load = () => {
      api
        .explorationGraph(taskId)
        .then((g) => {
          if (cancelled) return;
          const ns = g.nodes ?? [];
          const es = g.edges ?? [];
          const sig = JSON.stringify([
            ns.map((n) => [n.id, n.type, n.state, n.priority, n.payload]),
            es.map((e) => [e.src, e.dst, e.rel]),
          ]);
          setLoaded(true);
          setLoadError("");
          if (sig === sigRef.current) return; // 无变化 → 不重建
          sigRef.current = sig;
          setNodes(ns);
          setEdges(es);
        })
        .catch((error) => {
          if (cancelled) return;
          // 轮询失败保留最后一次成功图;首次失败则明确展示错误和重试入口。
          setLoadError((error as Error).message || "执行图读取失败");
          setLoaded(true);
        });
    };
    load();
    const timer = setInterval(load, 20000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [taskId, retryNonce]);

  const retry = () => {
    setLoadError("");
    if (nodes.length === 0) setLoaded(false);
    setRetryNonce((value) => value + 1);
  };

  return (
    <Card>
      <CardContent className="p-0">
        {loadError && nodes.length > 0 && (
          <div
            className="flex flex-wrap items-center justify-center gap-2 border-b bg-amber-50 px-4 py-2 text-sm text-amber-800"
            role="alert"
          >
            <AlertCircleIcon className="size-4" />
            <span>执行图刷新失败，当前保留上一次成功结果：{loadError}</span>
            <Button variant="outline" size="sm" className="h-7" onClick={retry}>
              <RefreshCwIcon /> 重试
            </Button>
          </div>
        )}
        {!loaded ? (
          <div className="flex h-[72vh] flex-col gap-3 p-6">
            <Skeleton className="h-10 w-48" />
            <Skeleton className="h-full w-full" />
          </div>
        ) : loadError && nodes.length === 0 ? (
          <div className="flex h-[72vh] flex-col items-center justify-center gap-3 p-6 text-center" role="alert">
            <AlertCircleIcon className="size-7 text-destructive" />
            <div>
              <p className="text-sm font-medium text-destructive">执行图读取失败</p>
              <p className="mt-1 text-sm text-muted-foreground">{loadError}</p>
            </div>
            <Button variant="outline" size="sm" onClick={retry}>
              <RefreshCwIcon /> 重试
            </Button>
          </div>
        ) : nodes.length === 0 ? (
          <div className="flex h-[72vh] flex-col items-center justify-center gap-2 p-6 text-center">
            <p className="text-sm font-medium">还没有执行图数据</p>
            <p className="text-sm text-muted-foreground">任务启动并产生目标、意图或工具步骤后，这里会自动更新。</p>
          </div>
        ) : (
          <ExplorationGraph nodes={nodes} edges={edges} className="h-[72vh]" />
        )}
      </CardContent>
    </Card>
  );
}
