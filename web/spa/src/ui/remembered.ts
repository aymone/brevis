// A list's skeleton draws as many rows as the screen showed last time, so the
// loaded list lands at the height the placeholder already had. localStorage can
// throw (private window, blocked storage): then the fallback is the server's page size.
// Under brevis.seen.* with the other remembered shapes (ui/seen.ts): one prefix, so signing out or
// switching operator clears them all together. Only a number is ever stored here.
const key = (k: string) => `brevis.seen.rows.${k}`;

export function recalled(k: string, fallback: number, max = 100): number {
  try {
    const n = Number(localStorage.getItem(key(k)));
    return Number.isInteger(n) && n > 0 ? Math.min(n, max) : fallback;
  } catch {
    return fallback;
  }
}

export function remember(k: string, n: number): void {
  if (n <= 0) return;
  try {
    localStorage.setItem(key(k), String(n));
  } catch {
    /* nothing to keep it in: the next load uses the fallback */
  }
}
