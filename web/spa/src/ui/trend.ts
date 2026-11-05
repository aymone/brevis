import { useSession } from "../api/session";
import { fixed, roundEven } from "../fixed";
import { formatUnit } from "./duration";
import { useT } from "../i18n";

// THE "WHAT IT LOADS" PANEL'S WORDS AND NUMBERS. The server sends numbers and codes (web/components/trend.go); the
// reader's language writes them: the decimal mark, the month, the labels and the delta.

/** A value in its unit: counts as 1.5k / 2.0M, sizes as 1.2 MB / 200 B, seconds as 0.4s / 12s / 2m 5s (the unit in the reader's language). */
export function trendValue(lang: string, unit: string, v: number): string {
  const one = (x: number) => fixed(lang, x, 1);
  const whole = (x: number) => fixed(lang, x, 0);
  if (unit === "count") {
    if (v >= 1e9) return `${one(v / 1e9)}B`;
    if (v >= 1e6) return `${one(v / 1e6)}M`;
    if (v >= 1e3) return `${one(v / 1e3)}k`;
    return whole(v);
  }
  if (unit === "size") {
    if (v >= 2 ** 30) return `${one(v / 2 ** 30)} GB`;
    if (v >= 2 ** 20) return `${one(v / 2 ** 20)} MB`;
    if (v >= 2 ** 10) return `${one(v / 2 ** 10)} kB`;
    return `${whole(v)} B`;
  }
  // Seconds, the unit written the reader's way (a panel of small numbers: 12s, 0.4s, 2m 5s in English).
  if (v >= 60) return `${formatUnit(lang, "minute", Math.floor(v / 60))} ${formatUnit(lang, "second", Math.floor(v) % 60)}`;
  if (v > 0 && v < 1) return formatUnit(lang, "second", Number(roundEven(v, 1)), { minimumFractionDigits: 1, maximumFractionDigits: 1 });
  return formatUnit(lang, "second", Number(roundEven(v, 0)));
}

/** A UTC day as "2 Jan" (English keeps the day first, as the panel always wrote it), with its year when asked. */
export function trendDay(lang: string, date: string, withYear = false): string {
  const d = new Date(`${date}T00:00:00Z`);
  if (lang.startsWith("en")) {
    const month = new Intl.DateTimeFormat("en-US", { month: "short", timeZone: "UTC" }).format(d);
    return `${d.getUTCDate()} ${month}${withYear ? ` ${d.getUTCFullYear()}` : ""}`;
  }
  return new Intl.DateTimeFormat(lang, { day: "numeric", month: "short", ...(withYear ? { year: "numeric" } : {}), timeZone: "UTC" }).format(d);
}

export function useTrend() {
  const { lang } = useSession();
  const { t } = useT();
  return {
    value: (unit: string, v: number) => trendValue(lang, unit, v),
    /** "2 Jan to 5 Mar 2026"; one day is just the day. The year is said once when both ends share it. */
    window: (from: string, to: string) => {
      if (from === to) return trendDay(lang, from);
      const sameYear = from.slice(0, 4) === to.slice(0, 4);
      return t("workflow.load.window", { from: trendDay(lang, from, !sameYear), to: trendDay(lang, to, true) });
    },
    day: (date: string) => trendDay(lang, date),
    delta: (kind: string, pct: number) => (kind === "new" ? t("workflow.load.new") : kind === "steady" ? t("workflow.load.steady") : kind === "pct" ? `${pct > 0 ? "+" : ""}${pct}%` : ""),
    label: (key: string) => t(`workflow.load.${key}`),
    note: (key: string) => t(`workflow.load.note.${key}`),
  };
}
