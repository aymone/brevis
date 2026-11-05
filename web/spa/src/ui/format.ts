import { useSession } from "../api/session";
import { useFormat, useT } from "../i18n";

/** A wait rounded to what a person plans by (the server's untilText): "<1m", "9h 28m", "3d". */
export function until(ms: number): string {
  const min = Math.floor(ms / 60_000);
  if (ms < 60_000) return "<1m";
  if (ms < 3_600_000) return `${min}m`;
  if (ms < 48 * 3_600_000) {
    const h = Math.floor(min / 60);
    return min % 60 > 0 ? `${h}h ${min % 60}m` : `${h}h`;
  }
  return `${Math.floor(ms / 86_400_000)}d`;
}

/** Number, percentage and duration formatters in the reader's language. */
export function useNumbers() {
  const { lang } = useSession();
  const one = new Intl.NumberFormat(lang, { minimumFractionDigits: 1, maximumFractionDigits: 1 });
  return {
    /** part of whole as "87.5%" */
    percent: (part: number, whole: number) => `${one.format((100 * part) / whole)}%`,
    /** A duration as the server's Elapsed writes it: 340ms, 1.5s, 2m 05s, 1h 02m. */
    elapsed: (ms: number) => {
      if (ms < 1000) return `${Math.round(ms)}ms`;
      if (ms < 60_000) return `${one.format(ms / 1000)}s`;
      const s = Math.floor(ms / 1000);
      if (ms < 3_600_000) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
      return `${Math.floor(s / 3600)}h ${String(Math.floor(s / 60) % 60).padStart(2, "0")}m`;
    },
  };
}

/** "3 min ago" up to a day, the date after that (the server's i18n.Since). */
export function useAgo() {
  const { t } = useT();
  const f = useFormat();
  return (iso: string, now = Date.now()) => {
    const d = now - new Date(iso).getTime();
    const span = (ms: number) => (ms < 3_600_000 ? `${Math.floor(ms / 60_000)} min` : ms < 86_400_000 ? `${Math.floor(ms / 3_600_000)}h` : `${Math.floor(ms / 86_400_000)}d`);
    if (d < 0) return t("time.in", { span: span(-d) });
    if (d < 60_000) return t("time.now");
    if (d < 86_400_000) return t("time.ago", { span: span(d) });
    return f.date(iso);
  };
}

/** A slot as the server's brief layout writes it, in UTC: "Mar 2 04:00", "2 out 04:00". */
export function useBrief() {
  const { lang } = useSession();
  const month = new Intl.DateTimeFormat(lang, { month: "short", timeZone: "UTC" });
  const clock = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZone: "UTC" });
  return (iso: string) => {
    const d = new Date(iso);
    const m = month.format(d).replace(".", "");
    return lang.startsWith("en") ? `${m} ${d.getUTCDate()} ${clock.format(d)}` : `${d.getUTCDate()} ${m} ${clock.format(d)}`;
  };
}

/** The server's UTCStamp: "2026-11-03 04:57:38 UTC". */
export const stamp = (iso: string) => `${new Date(iso).toISOString().slice(0, 19).replace("T", " ")} UTC`;

/** The server's i18n.Since: "now", "3 min ago", "in 5 min" up to a day, then the short date ("Nov 03 04:57"). */
export function useSince() {
  const { t } = useT();
  const { lang } = useSession();
  const month = new Intl.DateTimeFormat(lang, { month: "short" });
  const clock = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  return (iso: string, now: string | number = Date.now()) => {
    const at = new Date(iso);
    const d = new Date(now).getTime() - at.getTime();
    const span = (ms: number) => (ms < 3_600_000 ? `${Math.floor(ms / 60_000)} min` : ms < 86_400_000 ? `${Math.floor(ms / 3_600_000)}h` : `${Math.floor(ms / 86_400_000)}d`);
    if (d < 0) return t("time.in", { span: span(-d) });
    if (d < 60_000) return t("time.now");
    if (d < 86_400_000) return t("time.ago", { span: span(d) });
    const m = month.format(at).replace(".", "");
    const day = String(at.getDate()).padStart(2, "0");
    return lang.startsWith("en") ? `${m} ${day} ${clock.format(at)}` : `${day} ${m} ${clock.format(at)}`;
  };
}

/** The server's Timestamp (a title): the instant in the reader's zone, "2026-11-03 04:57:38 GMT-3". */
export function localStamp(iso: string): string {
  const d = new Date(iso);
  const p = (n: number) => String(n).padStart(2, "0");
  const zone = new Intl.DateTimeFormat("en", { timeZoneName: "short" }).formatToParts(d).find((x) => x.type === "timeZoneName")?.value ?? "";
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())} ${zone}`;
}
