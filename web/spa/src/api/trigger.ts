import { ApiError, postJSON } from "./client";
import type { Vars } from "../i18n";

// POST /api/workflows/{slug}/trigger (internal/api/trigger_api.go). A list
// travels as an array; every other value as the string the field holds.
export type TriggerBody = { params: Record<string, string | string[]>; slot?: string; from_run?: string };
export type Triggered = { runId: string; href: string };

export const triggerRun = (slug: string, body: TriggerBody) =>
  postJSON<Triggered>(`/api/workflows/${encodeURIComponent(slug)}/trigger`, body) as Promise<Triggered>;

/**
 * One refused field as the server sends it: a code and the data that makes the sentence (it speaks no language).
 * `item` is the 1-based position of the list item at fault; slot and nearest are RFC 3339 instants, zone an IANA name.
 */
export type FieldError = {
  code: string;
  value?: string;
  item?: number;
  allowed?: string[];
  pattern?: string;
  slot?: string;
  cron?: string;
  nearest?: string;
  zone?: string;
};

/** The 422's per-field refusals ("slot" is the picker's), empty for any other failure. */
export function fieldErrors(e: unknown): Record<string, FieldError> {
  if (!(e instanceof ApiError) || e.status !== 422) return {};
  const f = (e.body as unknown as { fields?: Record<string, FieldError | string> } | undefined)?.fields;
  if (!f || typeof f !== "object") return {};
  return Object.fromEntries(Object.entries(f).map(([k, v]) => [k, typeof v === "object" && v ? v : { code: "" }]));
}

const minute = (iso: string, zone: string) => {
  const p = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
      .formatToParts(new Date(iso))
      .map((x) => [x.type, x.value]),
  );
  return `${p.year}-${p.month}-${p.day} ${p.hour}:${p.minute}`;
};

/** A slot the way the schedule names it: UTC minute, the zone's beside it when the schedule has one. */
const slotText = (iso: string, zone?: string) => {
  const utc = `${minute(iso, "UTC")} UTC`;
  return zone && zone !== "UTC" ? `${utc} (${minute(iso, zone)} ${zone})` : utc;
};

/** The catalog key and variables that say one field's refusal; a code this app does not know reads as a plain refusal. */
export function fieldMessage(f: FieldError): { key: string; vars: Vars; item?: number } {
  const value = f.value ?? "";
  const vars: Vars = { value };
  // An item's refusal is the same sentence, with its position said in front.
  const item = f.item && f.code !== "item_empty" ? f.item : undefined;
  switch (f.code) {
    case "required":
      return { key: "drawer.check.required", vars };
    case "not_integer":
      return { key: "drawer.err.not_integer", vars, item };
    case "not_boolean":
      return { key: "drawer.err.not_boolean", vars, item };
    case "not_one_of":
      return { key: "drawer.err.not_one_of", vars: { value, allowed: (f.allowed ?? []).join(", ") }, item };
    case "no_match":
      return { key: "drawer.err.no_match", vars: { value, pattern: f.pattern ?? "" }, item };
    case "unsafe_chars":
      return { key: "drawer.err.unsafe_chars", vars, item };
    case "item_empty":
      return { key: "drawer.err.item_empty", vars: { n: f.item ?? 0 } };
    case "item_duplicate":
      return { key: "drawer.err.item_duplicate", vars };
    case "undeclared":
      return { key: "drawer.err.undeclared", vars };
    case "bad_value_type":
      return { key: "drawer.err.bad_value_type", vars };
    case "nested_list":
      return { key: "drawer.err.nested_list", vars };
    case "slot_not_a_time":
      return { key: "drawer.err.slot_not_a_time", vars };
    case "slot_in_future":
      return { key: "drawer.err.slot_in_future", vars: { slot: slotText(f.slot ?? "", "UTC") } };
    case "slot_no_schedule":
      return { key: "drawer.err.slot_no_schedule", vars };
    case "slot_not_on_schedule": {
      const vs = { slot: slotText(f.slot ?? "", "UTC"), cron: f.cron ?? "", nearest: f.nearest ? slotText(f.nearest, f.zone) : "" };
      return { key: f.nearest ? "drawer.err.slot_nearest" : "drawer.err.slot_not_on_schedule", vars: vs };
    }
  }
  return { key: "drawer.err.invalid", vars };
}

/** One field's refusal said in the reader's language: `say` is useT()'s t. */
export function fieldText(say: (key: string, vars?: Vars) => string, f: FieldError): string {
  const { key, vars, item } = fieldMessage(f);
  const text = say(key, vars);
  return item ? say("drawer.err.item", { n: item, message: text }) : text;
}
