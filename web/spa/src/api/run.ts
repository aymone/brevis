import { postJSON } from "./client";

// The shapes of GET /api/runs/{id} (web/pages/run_view.go).
export type LogItem = { n?: number; text?: string; level?: "info" | "warn" | "err"; cause?: boolean; gap?: number };
export type LogBlock = {
  step: string; attempt: number; status: string; statusLabel: string;
  exitCode: number | null; running: boolean; items: LogItem[];
};
export type RunLog = { lines: number; causeN: number; blocks: LogBlock[] };
export type KV = { key: string; value: string; note?: string };
export type StepRow = {
  node: string; item: string; status: string; attempt: number; exitCode: number | null;
  durationMs: number; command: string; err: string;
};
export type StepGroup = {
  node: string; mapped: boolean; status: string; ok: number; total: number; command: string; rows: StepRow[];
};
export type Stage = {
  label: string; state: string; detail: string; ms: number | null; numbers: { key: string; value: string }[];
};
export type Run = {
  id: string; workflow: string; logos: string[];
  status: string; statusLabel: string; attempt: number;
  inFlight: boolean; terminal: boolean; canceling: boolean;
  trigger: string; slot: string | null; createdAt: string; startedAt: string | null; finishedAt: string | null;
  durationMs: number | null;
  rerun: "" | "now" | "params";
  err: string;
  phases: { key: string; label: string; detail: string; state: string }[];
  why: { title: string; streak: number; excerpt: string; hasLine: boolean } | null;
  wait: {
    group: string; limit: number; ahead: number; more: number;
    holders: { runId: string; workflow: string; since: string }[];
  } | null;
  step: {
    name: string; status: string; action: string; runtime: string; runtimeInferred: boolean; sdk: string;
    durationMs: number; published: number; stages: Stage[];
  } | null;
  auto: KV[]; params: KV[]; steps: StepGroup[];
  alerts: {
    tone: string; wording: string; channel: string; attempts: number; err: string;
    createdAt: string; deliveredAt: string | null;
  }[];
  context: { step: string; json: string; bytes: number; note: string }[];
  log: RunLog;
};

export const cancelRun = (id: string) => postJSON<{ outcome: string; status: string }>(`/api/runs/${id}/cancel`);
