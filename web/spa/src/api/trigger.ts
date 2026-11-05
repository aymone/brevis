import { ApiError, postJSON } from "./client";

// POST /api/workflows/{slug}/trigger (internal/api/trigger_api.go). A list
// travels as an array; every other value as the string the field holds.
export type TriggerBody = { params: Record<string, string | string[]>; slot?: string; from_run?: string };
export type Triggered = { runId: string; href: string };

export const triggerRun = (slug: string, body: TriggerBody) =>
  postJSON<Triggered>(`/api/workflows/${encodeURIComponent(slug)}/trigger`, body) as Promise<Triggered>;

/** The 422's per-field sentences ("slot" is the picker's), empty for any other failure. */
export function fieldErrors(e: unknown): Record<string, string> {
  if (!(e instanceof ApiError) || e.status !== 422) return {};
  const f = (e.body as unknown as { fields?: Record<string, string> } | undefined)?.fields;
  return f ?? {};
}
