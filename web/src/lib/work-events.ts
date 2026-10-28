export const WORK_CHANGED_EVENT = "artex:work-changed";

export function emitWorkChanged() {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(WORK_CHANGED_EVENT));
}
