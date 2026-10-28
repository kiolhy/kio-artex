import { parseWorkQuickCapture } from "./work-quick-capture.ts";
import assert from "node:assert/strict";
import test from "node:test";

const projects = [
  { id: 7, name: "ARTEX" },
  { id: 9, name: "周报" },
];
const now = new Date(2026, 9, 20, 9, 30);

test("Chinese date tokens set the deadline and are removed from the title", () => {
  const result = parseWorkQuickCapture("今天 跟进 #ARTEX !high", projects, now);
  assert.equal(result.title, "跟进");
  assert.equal(result.project_id, 7);
  assert.equal(result.priority, "high");
  assert.equal(new Date(result.due_at).getDate(), 20);
  assert.equal(new Date(result.due_at).getHours(), 18);
});

test("date words accept an adjacent or separate clock and do not leak into the title", () => {
  const adjacent = parseWorkQuickCapture("明天9:00 #周报 提交周报", projects, now);
  assert.equal(adjacent.title, "提交周报");
  assert.equal(adjacent.project_id, 9);
  assert.equal(new Date(adjacent.due_at).getDate(), 21);
  assert.equal(new Date(adjacent.due_at).getHours(), 9);

  const separate = parseWorkQuickCapture("today 10:15 publish notes", [], now);
  assert.equal(separate.title, "publish notes");
  assert.equal(new Date(separate.due_at).getDate(), 20);
  assert.equal(new Date(separate.due_at).getHours(), 10);
  assert.equal(new Date(separate.due_at).getMinutes(), 15);
});

test("a date word embedded in ordinary text is not treated as a command", () => {
  const result = parseWorkQuickCapture("更新今天计划 /高", [], now);
  assert.equal(result.title, "更新今天计划");
  assert.equal(result.priority, "high");
  assert.equal(result.due_at, null);
});

test("the day after tomorrow and invalid clocks behave predictably", () => {
  const result = parseWorkQuickCapture("后天 25:99 复盘", [], now);
  assert.equal(result.title, "25:99 复盘");
  assert.equal(new Date(result.due_at).getDate(), 22);
  assert.equal(new Date(result.due_at).getHours(), 18);
});
