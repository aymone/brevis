import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { Cat, Msg } from "../i18n";
import { postJSON } from "./client";

/** A language, tool or connector by its logo key; the app names it (useLogoTitle). */
export type Logo = { key: string };
export type Recent = { id: string; status: string; at: string };

export type WorkflowRow = {
  slug: string;
  project: string;
  steps: number | null;
  failing: boolean;
  inFlight: boolean;
  hasParams: boolean;
  runsIn: Logo[];
  connects: Logo[];
  hasSchedule: boolean;
  active: boolean;
  cron: string;
  /** The cron in words, a sentence the app says; null when it has no plain reading. */
  schedule: Msg | null;
  timezone: string;
  catchup: boolean;
  nextRun: string | null;
  lastRunAt: string | null;
  totalRuns: number;
  recent: Recent[];
  tags: string[];
};

export type WorkflowsList = {
  filter: {
    q: string;
    state: string;
    active: string;
    lang: string;
    tags: string[];
    project: string;
    sort: string;
    dir: "asc" | "desc";
    page: number;
    perPage: number;
  };
  rows: WorkflowRow[];
  counts: Record<string, number>;
  tags: string[];
  langs: (Logo & { count: number })[];
  total: number;
  filtered: number;
};

export type Step = {
  id: string;
  command: Cat | null;
  langs: Logo[];
  uses: Logo[];
  needs: string[];
  lastStatus: string;
  lastMs: number;
};

export type WorkflowDetail = {
  slug: string;
  description: string;
  project: string;
  hasSchedule: boolean;
  active: boolean;
  cron: string;
  schedule: Msg | null;
  timezone: string;
  nextRun: string | null;
  tags: string[];
  langs: Logo[];
  edges: number;
  stages: number;
  stats: { ranDays: number; failedDays: number; meanMs: number; running: number; queued: number };
  calendar: {
    days: { date: string; total: number; failed: number; open: number }[];
    ranDays: number;
    failedDays: number;
    streak: number;
    lastFailure: string | null;
    since: string | null;
  };
  /** The "What it loads" series (null without load history): numbers and codes, the lines already drawn; the app says the rest. */
  load: {
    /** The first and last day of the window, YYYY-MM-DD. */
    from: string;
    to: string;
    series: {
      key: "rows" | "bytes" | "extract" | "load";
      unit: "count" | "size" | "seconds";
      latest: number;
      /** "" (nothing to say), "new", "steady" or "pct" (pct is the change in percent). */
      delta: "" | "new" | "steady" | "pct";
      pct: number;
      hue: string;
      path: string;
      area: string;
      flat: boolean;
      points: { x: number; y: number; date: string; value: number }[];
    }[];
  } | null;
  steps: Step[];
  runs: {
    id: string;
    status: string;
    trigger: string;
    slot: boolean;
    logicalDate: string | null;
    given: string[];
    durationMs: number | null;
    createdAt: string;
  }[];
  day: string | null;
  params: { name: string; type: string; default: string; title: string; description: string; warn: string; group: string; uses: string; enum: string[]; pattern: string }[];
  /** The schedule's last slots, newest first; the app says each label. */
  slots: { at: string; label: Msg }[];
  yaml: string;
  now: string;
};

type Toggled = { active: boolean };

// useScheduleToggle pauses or resumes a schedule and keeps what happened (the
// page says it, in the reader's language, and undoes from it). Every cached list and page
// is refreshed, so the switch, the pill and the count agree.
export function useScheduleToggle() {
  const qc = useQueryClient();
  const [note, setNote] = useState<{ slug: string; active: boolean } | null>(null);
  const m = useMutation({
    mutationFn: (slug: string) => postJSON<Toggled>(`/api/workflows/${encodeURIComponent(slug)}/toggle`),
    onSuccess: (r, slug) => {
      if (r) setNote({ slug, active: r.active });
      void qc.invalidateQueries({ queryKey: ["workflows"] });
      void qc.invalidateQueries({ queryKey: ["workflow", slug] });
    },
  });
  return { toggle: m.mutate, pending: m.isPending ? m.variables : null, error: m.error, note, dismiss: () => setNote(null) };
}
