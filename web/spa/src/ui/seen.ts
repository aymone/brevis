import { useEffect, useMemo } from "react";

// A list screen draws its loading state from what it showed the last time: how
// many rows, which filters, which counts. Placeholders of the right size mean
// the answer lands without moving anything. localStorage can be absent, full or
// refused (a private window): every touch is guarded, and the fallback is used.
const key = (k: string) => `brevis.seen.${k}`;

/** What the screen showed last time under `k`, over `fallback` for the first visit. */
export function recall<T extends object>(k: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key(k));
    return raw ? { ...fallback, ...(JSON.parse(raw) as Partial<T>) } : fallback;
  } catch {
    return fallback;
  }
}

/** Forget everything remembered: a sign-out must not leave anything of this operator's screens behind. */
export function clearSeen() {
  try {
    for (const k of Object.keys(localStorage)) if (k.startsWith("brevis.seen.")) localStorage.removeItem(k);
  } catch {
    /* storage refused: nothing was kept */
  }
}

/** Another operator in this browser starts from nothing. Only a number derived from the name is stored, never the name. */
export function bindOperator(operator: string) {
  let h = 7;
  for (const c of operator) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  try {
    if (localStorage.getItem(key("operator")) !== String(h)) {
      clearSeen();
      localStorage.setItem(key("operator"), String(h));
    }
  } catch {
    /* storage refused: nothing is kept either */
  }
}

/** Keep `v` as what the screen showed under `k`. */
export function remember(k: string, v: object) {
  try {
    localStorage.setItem(key(k), JSON.stringify(v));
  } catch {
    /* nothing to remember it in */
  }
}

/**
 * The shape to draw: the live one once there is an answer (remembered for next time), the
 * remembered one before it. `live` is small by contract (counts, never rows).
 */
export function useSeen<T extends object>(k: string, live: T | undefined, fallback: T): T {
  const saved = live === undefined ? undefined : JSON.stringify(live);
  useEffect(() => {
    if (saved === undefined) return;
    try {
      localStorage.setItem(key(k), saved);
    } catch {
      /* nothing to remember it in; the next visit draws the fallback */
    }
  }, [k, saved]);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const before = useMemo(() => recall(k, fallback), [k]);
  return live ?? before;
}
