import type { ReactNode } from "react";
import { useFormat } from "../../i18n";
import "./filters.css";

/**
 * "3 of 12 workflows": a polite live region that always exists (a region added late is not announced),
 * in the bar's reserved slot. While the first answer is pending only the figures ghost, in the width of
 * the real text; with kept-previous data it shows the last numbers (the list under it is dimmed).
 */
export function ResultCount({ pending, shown, tail }: { pending?: boolean; shown: number; tail: ReactNode }) {
  const { number } = useFormat();
  return (
    <p className="flt-count" role="status" aria-live="polite" aria-atomic="true" aria-busy={pending || undefined}>
      {pending ? (
        <span className="sk-ghost" aria-hidden="true"><b>00</b> <span>{tail}</span></span>
      ) : (
        <><b>{number(shown)}</b> {tail}</>
      )}
    </p>
  );
}
