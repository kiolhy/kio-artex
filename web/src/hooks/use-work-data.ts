"use client";

import * as React from "react";

import { api } from "@/lib/api";
import type { WorkTask } from "@/lib/types";
import { WORK_CHANGED_EVENT } from "@/lib/work-events";

export type WorkDataState<T> = {
  data: T;
  loading: boolean;
  error?: string;
  reload: () => void;
};

export function useWorkTasks(
  params?: Parameters<typeof api.workTasks>[0],
  refreshKey = 0,
): WorkDataState<WorkTask[]> {
  const [data, setData] = React.useState<WorkTask[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string>();
  const [reloadKey, setReloadKey] = React.useState(0);
  const serialized = JSON.stringify(params ?? {});

  React.useEffect(() => {
    let alive = true;
    const run = async () => {
      setLoading(true);
      try {
        const items = await api.workTasks(JSON.parse(serialized));
        if (!alive) return;
        setData(items);
        setError(undefined);
      } catch (e) {
        if (alive) setError((e as Error).message);
      } finally {
        if (alive) setLoading(false);
      }
    };
    void run();
    return () => {
      alive = false;
    };
  }, [serialized, reloadKey, refreshKey]);

  React.useEffect(() => {
    const refresh = () => setReloadKey((value) => value + 1);
    window.addEventListener(WORK_CHANGED_EVENT, refresh);
    return () => window.removeEventListener(WORK_CHANGED_EVENT, refresh);
  }, []);

  return { data, loading, error, reload: () => setReloadKey((v) => v + 1) };
}
