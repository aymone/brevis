import type { Cat, Msg } from "../i18n";
import { postJSON } from "./client";

// The shapes of GET /api/runs/{id} (web/pages/run_view.go).
export type LogItem = { n?: number; text?: string; level?: "info" | "warn" | "err"; cause?: boolean; gap?: number };
export type LogBlock = {
  step: string; attempt: number; status: string;
  exitCode: number | null; running: boolean; items: LogItem[];
};
export type RunLog = { lines: number; causeN: number; blocks: LogBlock[] };
export type KV = { key: string; value: string };
/** An auto param: its note is a sentence, its value an instant, a lateness or a plain value. */
export type Auto = { key: string; note: Msg; value: string; at: string | null; delaySeconds: number | null };
/** What a phase has to say, as data: when (at), the slot, how late it started, how long it waited or ran, how it ended. */
export type PhaseDetail = {
  kind: "at" | "slot" | "late" | "waiting" | "ran" | "ended";
  at?: string; ms?: number; state?: string;
};
export type StepRow = {
  node: string; item: string; status: string; attempt: number; exitCode: number | null;
  durationMs: number; command: Cat | null; err: string;
};
export type StepGroup = {
  node: string; mapped: boolean; status: string; ok: number; total: number; command: Cat | null; rows: StepRow[];
};
export type Stage = {
  label: Cat | null; state: string; detail: string; ms: number | null; numbers: { key: string; value: number | string }[];
};
export type Run = {
  id: string; workflow: string; logos: string[];
  status: string; attempt: number;
  inFlight: boolean; terminal: boolean; canceling: boolean;
  /** How it was made, as the engine writes it ("schedule", "manual"...). */
  trigger: string; slot: string | null; createdAt: string; startedAt: string | null; finishedAt: string | null;
  durationMs: number | null;
  rerun: "" | "now" | "params";
  err: string;
  phases: { key: string; label: Msg; detail: PhaseDetail | null; state: string }[];
  why: { title: Msg; streak: number; excerpt: string; hasLine: boolean } | null;
  wait: {
    group: string; limit: number; ahead: number; more: number;
    holders: { runId: string; workflow: string; since: string }[];
  } | null;
  step: {
    name: string; status: string; action: string; runtime: string; runtimeInferred: boolean; sdk: string;
    durationMs: number; published: number; stages: Stage[];
  } | null;
  auto: Auto[]; params: KV[]; steps: StepGroup[];
  alerts: {
    tone: string; wording: Msg; channel: string; attempts: number; err: string;
    createdAt: string; deliveredAt: string | null;
  }[];
  context: { step: string; json: string; bytes: number; note: Msg | null }[];
  log: RunLog;
};

export const cancelRun = (id: string) => postJSON<{ outcome: string; status: string }>(`/api/runs/${id}/cancel`);
