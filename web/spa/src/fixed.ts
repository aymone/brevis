/**
 * x with a fixed number of decimals, the way the server wrote it before the app did (Go's strconv): the exact
 * value of the double, and a tie to the even digit (1.25 is "1.2", not "1.3"), then the reader's decimal mark.
 * Intl would round the tie up; a duration of 1250ms would read differently from the one the console always said.
 */
export function fixed(lang: string, x: number, digits: number): string {
  const mark = new Intl.NumberFormat(lang).formatToParts(1.1).find((s) => s.type === "decimal")?.value ?? ".";
  return roundEven(x, digits).replace(".", mark);
}

/** x rounded to `digits` decimals as plain digits with a dot (tie to the even digit): the number, not yet the reader's. */
export function roundEven(x: number, digits: number): string {
  let text = x.toFixed(digits);
  // toFixed works on the exact value and rounds a tie up: only an exact tie (the digits after the cut are 5 and
  // zeros all the way, which only a dyadic number has) needs the even digit, so read it off the full expansion.
  const full = Math.abs(x).toFixed(40);
  const dot = full.indexOf(".");
  const rest = full.slice(dot + 1 + digits);
  if (/^50*$/.test(rest)) {
    const kept = digits === 0 ? full.slice(0, dot) : full.slice(0, dot + 1 + digits);
    if (Number(kept.slice(-1)) % 2 === 0) text = (x < 0 ? "-" : "") + kept;
  }
  return text;
}
