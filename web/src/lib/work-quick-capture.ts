import type { WorkProject, WorkTask } from "./types";

type ProjectOption = Pick<WorkProject, "id" | "name">;

type DateToken = {
  offset: number;
  hours?: number;
  minutes?: number;
};

export type WorkQuickCapture = {
  title: string;
  due_at: string | null;
  priority?: WorkTask["priority"];
  project_id?: number | null;
};

const priorityTokens: Record<string, WorkTask["priority"]> = {
  "!紧急": "urgent",
  "!urgent": "urgent",
  p0: "urgent",
  "/紧急": "urgent",
  "!高": "high",
  "!high": "high",
  p1: "high",
  "/高": "high",
  "!中": "medium",
  "!medium": "medium",
  p2: "medium",
  "/中": "medium",
  "!低": "low",
  "!low": "low",
  p3: "low",
  "/低": "low",
};

const dateOffsets: Record<string, number> = {
  今天: 0,
  today: 0,
  明天: 1,
  tomorrow: 1,
  后天: 2,
};

export function parseWorkQuickCapture(raw: string, projects: ProjectOption[] = [], now = new Date()): WorkQuickCapture {
  const tokens = raw.trim().split(/\s+/).filter(Boolean);
  const words: string[] = [];
  const result: WorkQuickCapture = { title: "", due_at: null };
  let parsedDate: DateToken | undefined;

  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    const normalized = token.toLowerCase();
    const project = projects.find((candidate) => candidate.name === token || `#${candidate.name}` === token);
    if (project) {
      result.project_id = project.id;
      continue;
    }

    const priority = priorityTokens[normalized];
    if (priority) {
      result.priority = priority;
      continue;
    }

    const date = parseDateToken(token);
    if (date) {
      if (date.hours === undefined) {
        const clock = parseClock(tokens[index + 1]);
        if (clock) {
          date.hours = clock.hours;
          date.minutes = clock.minutes;
          index += 1;
        }
      }
      parsedDate = date;
      continue;
    }

    words.push(token);
  }

  result.title = words.join(" ").trim();
  if (parsedDate) {
    const due = new Date(now);
    due.setDate(due.getDate() + parsedDate.offset);
    due.setHours(parsedDate.hours ?? 18, parsedDate.minutes ?? 0, 0, 0);
    result.due_at = due.toISOString();
  }
  return result;
}

function parseDateToken(token: string): DateToken | undefined {
  const match = token.toLowerCase().match(/^(今天|明天|后天|today|tomorrow)(?:(\d{1,2}):(\d{2}))?$/);
  if (!match) return undefined;
  const clock = match[2] ? parseClock(`${match[2]}:${match[3]}`) : undefined;
  return {
    offset: dateOffsets[match[1]],
    hours: clock?.hours,
    minutes: clock?.minutes,
  };
}

function parseClock(token: string | undefined): { hours: number; minutes: number } | undefined {
  const match = token?.match(/^(\d{1,2}):(\d{2})$/);
  if (!match) return undefined;
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  if (hours > 23 || minutes > 59) return undefined;
  return { hours, minutes };
}
