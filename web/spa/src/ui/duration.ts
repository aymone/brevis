import { roundEven } from "../fixed";

// THE UNITS ARE THE READER'S LANGUAGE'S, not "h", "m" and "s" in every screen: a Russian reads "1 ч 04 мин", a Korean
// "1시간 04분". The unit words come from Intl (CLDR), so there is no catalog to keep and no language in the code.
// English keeps the compact form the console always had (400ms, 1.5s, 2m 05s, 1h 04m), the other languages use their
// short unit names.
export type Unit = "millisecond" | "second" | "minute" | "hour" | "day";

const cache = new Map<string, Intl.NumberFormat>();

function unit(lang: string, u: Unit, extra: Intl.NumberFormatOptions = {}): Intl.NumberFormat {
  const display = lang.startsWith("en") ? "narrow" : "short";
  const key = `${lang}|${u}|${display}|${JSON.stringify(extra)}`;
  let f = cache.get(key);
  if (!f) {
    f = new Intl.NumberFormat(lang, { style: "unit", unit: u, unitDisplay: display, ...extra });
    cache.set(key, f);
  }
  return f;
}

const two = { minimumIntegerDigits: 2 };

/** A number with a time unit in the reader's language ("12 мин"): for the places that choose the unit themselves. */
export function formatUnit(lang: string, u: Unit, value: number, extra: Intl.NumberFormatOptions = {}): string {
  return unit(lang, u, extra).format(value);
}

/** A duration in the unit its size calls for: 400ms, 1.5s, 2m 05s, 1h 04m (units in the reader's language). */
export function formatDuration(lang: string, ms: number): string {
  if (ms < 1000) return unit(lang, "millisecond").format(Math.round(ms));
  if (ms < 60_000) {
    // One decimal, a tie to the even digit like Go's strconv (1250ms is 1.2s, not 1.3s), then Intl's own separators.
    const s = Number(roundEven(ms / 1000, 1));
    return unit(lang, "second", { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(s);
  }
  const total = Math.floor(ms / 1000);
  if (ms < 3_600_000) return `${unit(lang, "minute").format(Math.floor(total / 60))} ${unit(lang, "second", two).format(total % 60)}`;
  return `${unit(lang, "hour").format(Math.floor(total / 3600))} ${unit(lang, "minute", two).format(Math.floor(total / 60) % 60)}`;
}

/** A coarse span for "3 min ago" and "in 2d": minutes under an hour, hours under a day, then days. */
export function formatSpan(lang: string, ms: number): string {
  if (ms < 3_600_000) {
    // English says "3 min" here (and "5h", "2d" below): its short minute, the narrow hour and day.
    const f = lang.startsWith("en") ? new Intl.NumberFormat(lang, { style: "unit", unit: "minute", unitDisplay: "short" }) : unit(lang, "minute");
    return f.format(Math.floor(ms / 60_000));
  }
  if (ms < 86_400_000) return unit(lang, "hour").format(Math.floor(ms / 3_600_000));
  return unit(lang, "day").format(Math.floor(ms / 86_400_000));
}

/** A wait rounded to what a person plans by: "<1m", "9h 28m", "3d". */
export function formatUntil(lang: string, ms: number): string {
  const min = Math.floor(ms / 60_000);
  if (ms < 60_000) return `<${unit(lang, "minute").format(1)}`;
  if (ms < 3_600_000) return unit(lang, "minute").format(min);
  if (ms < 48 * 3_600_000) {
    const h = Math.floor(min / 60);
    return min % 60 > 0 ? `${unit(lang, "hour").format(h)} ${unit(lang, "minute").format(min % 60)}` : unit(lang, "hour").format(h);
  }
  return unit(lang, "day").format(Math.floor(ms / 86_400_000));
}
