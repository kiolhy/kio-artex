import { api } from "@/lib/api";
import type { WorkTask } from "@/lib/types";

export function workTaskIsAutomating(task: WorkTask) {
  if (task.automation_status === "running") return true;

  // List responses can briefly lag behind the scheduler. If a link already
  // exists, still treat it as active unless automation reached a terminal
  // state; deleting first could otherwise leave the linked run behind.
  return Boolean(task.artex_task_id) && !["done", "failed", "canceled"].includes(task.automation_status);
}

/**
 * Deletes a personal task without leaving an associated Agent task running.
 * If cancellation fails, deletion is aborted and the original task is kept.
 */
export async function deleteWorkTaskSafely(task: WorkTask) {
  if (workTaskIsAutomating(task)) {
    try {
      await api.cancelWorkTaskAutomation(task.id);
    } catch (error) {
      throw new Error(`无法取消自动执行，任务未删除：${(error as Error).message}`);
    }
  }
  await api.deleteWorkTask(task.id);
}
