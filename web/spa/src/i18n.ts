import { useSuspenseQuery } from "@tanstack/react-query";
import { getJSON } from "./api/client";
import { useSession } from "./api/session";
import { category } from "./plural";
import { formatDuration } from "./ui/duration";

type Message = string | { one: string; other: string; few?: string; many?: string };
type Bundle = { lang: string; messages: Record<string, Message> };
export type Vars = Record<string, string | number>;

// Same rule as the server's fill (web/i18n): a named placeholder is replaced
// when given, and left as written when not, so a missing variable is visible.
function fill(text: string, vars?: Vars): string {
  if (!vars) return text;
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => (name in vars ? String(vars[name]) : whole));
}

// The catalog is the server's own (/api/messages, English filling what a
// language lacks), keyed by language so a change of language loads the new one.
// Revalidated on load (the server answers 304 when nothing changed).
export function useT() {
  const { lang } = useSession();
  const { messages } = useSuspenseQuery({
    queryKey: ["messages", lang],
    queryFn: () => getJSON<Bundle>("/api/messages"),
    staleTime: Infinity,
  }).data;

  const pick = (key: string, count?: number): string => {
    const m = messages[key];
    if (m === undefined) return `[${key}]`; // the server's mark for a missing key
    if (typeof m === "string") return m;
    // few and many exist only where the language has them (plural.ts); a missing form reads as other
    if (count === undefined) return m.other;
    const c = category(lang, count);
    return c === "one" ? m.one : c === "few" ? (m.few ?? m.other) : c === "many" ? (m.many ?? m.other) : m.other;
  };

  return {
    lang,
    /** t("nav.projects"), t("run.wait.since", { at }) */
    t: (key: string, vars?: Vars) => fill(pick(key), vars),
    /** n("nav.catalog.attention", 3): the plural form, with {count} filled. */
    n: (key: string, count: number, vars?: Vars) => fill(pick(key, count), { count, ...vars }),
  };
}

/** Numbers, dates and "x ago" in the reader's language (Intl, not the server's format.go). */
export function useFormat() {
  const { lang } = useSession();
  return {
    number: (v: number) => new Intl.NumberFormat(lang).format(v),
    date: (iso: string) => new Intl.DateTimeFormat(lang, { dateStyle: "medium" }).format(new Date(iso)),
    dateTime: (iso: string) => new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso)),
    /** The wall-clock time of an instant, in the reader's zone ("10:12"); with its day when it is not today. */
    clock: (iso: string) => {
      const d = new Date(iso);
      const sameDay = d.toDateString() === new Date().toDateString();
      return new Intl.DateTimeFormat(lang, sameDay ? { timeStyle: "short" } : { dateStyle: "medium", timeStyle: "short" }).format(d);
    },
    /** The instant to the second, in UTC (a run's slot is a UTC schedule). */
    utc: (iso: string) =>
      `${new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "medium", timeZone: "UTC" }).format(new Date(iso))} UTC`,
    /** 400ms, 1.5s, 2m 05s, 1h 04m: a duration in milliseconds, its units in the reader's language. */
    duration: (ms: number) => formatDuration(lang, ms),
    /** An instant in one of the console's shapes, 24-hour: short "Oct 02 09:05", brief "Oct 2 09:05", day, month, weekday. */
    stamp: (iso: string, layout: string, zone = "UTC") => stamp(lang, iso, layout, zone),
    since: (iso: string, now = Date.now()) => {
      const s = Math.round((new Date(iso).getTime() - now) / 1000);
      const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "auto" });
      const abs = Math.abs(s);
      if (abs < 60) return rtf.format(s, "second");
      if (abs < 3600) return rtf.format(Math.round(s / 60), "minute");
      if (abs < 86400) return rtf.format(Math.round(s / 3600), "hour");
      return rtf.format(Math.round(s / 86400), "day");
    },
  };
}

const STAMPS: Record<string, Intl.DateTimeFormatOptions> = {
  short: { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" },
  brief: { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" },
  day: { weekday: "short", month: "short", day: "numeric", year: "numeric" },
  month: { month: "short" },
  weekday: { weekday: "short" },
};

function stamp(lang: string, iso: string, layout: string, zone: string): string {
  const opts = STAMPS[layout];
  if (!opts) return iso;
  const say = (timeZone: string) => new Intl.DateTimeFormat(lang, { ...opts, timeZone }).format(new Date(iso));
  let out: string;
  try {
    out = say(zone);
  } catch {
    out = say("UTC");
  }
  // English wrote "Oct 02 09:05", not "Oct 02, 09:05": the comma Intl adds before the time is not the console's.
  return lang.startsWith("en") && opts.hour ? out.replace(",", "") : out;
}

// A MESSAGE FROM THE SERVER. The server has no language (web/i18n/msg.go): it sends the catalog key of a sentence and
// the data that fills it, and the app says it. A value is a string, a number, another message, or a typed value the
// app formats the way it formats everything else (number, duration, instant).
export type Msg = { k: string; n?: number; a?: Record<string, MsgArg> };
export type MsgArg =
  | string
  | number
  | boolean
  | null
  | Msg
  | { $: "num"; v: number }
  | { $: "dur"; ms: number }
  | { $: "at"; v: string; layout: string; zone: string }
  | Cat;

/** Words put together by the app: messages and plain strings (data, a separator), said one after the other. */
export type Cat = { $: "cat"; v: (string | Msg)[] };

type Say = {
  t: (key: string, vars?: Vars) => string;
  n: (key: string, count: number, vars?: Vars) => string;
  fmt: Pick<ReturnType<typeof useFormat>, "number" | "duration" | "stamp">;
};

const isMsg = (v: MsgArg): v is Msg => typeof v === "object" && v !== null && "k" in v;

/** A Cat of plain strings: placeholder data for a screen drawn before its data arrives. */
export const cat = (...v: (string | Msg)[]): Cat => ({ $: "cat", v });

/** Says a Cat: each message in turn, each string as it is. */
export function renderCat(c: Cat, say: Say): string {
  return c.v.map((p) => (typeof p === "string" ? p : renderMsg(p, say))).join("");
}

/** Says a message: pure, so the rule is tested without a component. */
export function renderMsg(m: Msg, say: Say): string {
  const vars: Vars = {};
  for (const [name, v] of Object.entries(m.a ?? {})) {
    if (v === null || v === undefined) vars[name] = "";
    else if (typeof v === "string" || typeof v === "number") vars[name] = v;
    else if (typeof v === "boolean") vars[name] = String(v);
    else if (isMsg(v)) vars[name] = renderMsg(v, say);
    else if (v.$ === "num") vars[name] = say.fmt.number(v.v);
    else if (v.$ === "dur") vars[name] = say.fmt.duration(v.ms);
    else if (v.$ === "at") vars[name] = say.fmt.stamp(v.v, v.layout, v.zone);
    else if (v.$ === "cat") vars[name] = renderCat(v, say);
  }
  return m.n === undefined ? say.t(m.k, vars) : say.n(m.k, m.n, vars);
}

/** useMsg()(m): a message from the server, said in the reader's language ("" for none). */
export function useMsg() {
  const { t, n } = useT();
  const fmt = useFormat();
  return (m: Msg | Cat | null | undefined): string => {
    if (!m) return "";
    return "$" in m ? renderCat(m, { t, n, fmt }) : renderMsg(m, { t, n, fmt });
  };
}
