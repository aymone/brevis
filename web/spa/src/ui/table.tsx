import { useLayoutEffect, useRef, type ReactNode } from "react";
import { recall, remember } from "./seen";

// The console's table, drawn as the server draws it: a bordered card, the
// `ct` rows (34px, hairlines between), columns that give way by width and
// never turn into cards. Rows are whole links to their page through `Row`.

/**
 * The card around a table; `foot` is the honest line under it; `ghost` draws the rows as placeholders
 * (see .sk-ghost). With `memo`, the table remembers how tall each loaded row was and gives the ghost
 * rows those heights and the columns those widths, so rows that wrap or carry a note do not move what
 * is under them, nor the cells sideways, on arrival.
 */
export function Table({ id, head, children, foot, catalog, ghost, memo, cls = "" }: { id?: string; head: ReactNode; children: ReactNode; foot?: ReactNode; catalog?: boolean; ghost?: boolean; memo?: string; cls?: string }) {
  const grid = useRef<HTMLTableElement>(null);
  const saved = useRef("");
  useLayoutEffect(() => {
    const tbl = grid.current;
    if (!memo || !tbl) return;
    const k = `rows.${memo}`;
    const heads = Array.from(tbl.tHead?.rows.item(0)?.cells ?? []);
    const rows = tbl.tBodies.item(0)?.rows;
    if (!rows) return;
    if (ghost) {
      const m = recall<{ h: number[]; w: number[]; tw: number }>(k, { h: [], w: [], tw: 0 });
      for (let i = 0; i < rows.length; i++) if (m.h[i]) rows.item(i)!.style.height = `${m.h[i]}px`;
      // Same table width as last time: pin the columns where they were, so the cells do not slide sideways on arrival.
      if (m.w.length === heads.length && Math.abs(tbl.offsetWidth - m.tw) < 2) {
        heads.forEach((th, i) => (th.style.width = `${m.w[i]}px`));
        tbl.classList.add("sk-pinned");
      }
      return;
    }
    tbl.classList.remove("sk-pinned");
    heads.forEach((th) => th.style.removeProperty("width"));
    const h = Array.from(rows, (r) => +r.getBoundingClientRect().height.toFixed(2));
    const w = heads.map((th) => +th.getBoundingClientRect().width.toFixed(2));
    const now = `${h.join(",")}|${w.join(",")}`;
    if (now !== saved.current) {
      saved.current = now;
      remember(k, { h, w, tw: tbl.offsetWidth });
    }
  });
  return (
    <div className={`overflow-hidden rounded-card border border-line bg-surface ${catalog ? "shadow-warm-sm" : ""}`}>
      <div className="overflow-x-auto">
        <table ref={grid} id={id} className={`ct w-full ${catalog ? "text-sm" : "text-[13px]"} ${cls}`.trim()}>
          <thead>
            <tr className={`border-b border-line text-left text-[11.5px] font-medium ${catalog ? "bg-parchment-soft/60 text-faint" : "text-muted"}`}>{head}</tr>
          </thead>
          <tbody className={`divide-y divide-line-soft ${ghost ? "sk-ghost" : ""}`} aria-hidden={ghost || undefined}>{children}</tbody>
        </table>
      </div>
      {foot ? <p className="border-t border-line-soft px-4 py-2.5 text-[12px] text-faint">{foot}</p> : null}
    </div>
  );
}

/** A column header. `edge` pads the first or last column to the card's edge; `cls` carries the width it gives way at. */
export function Th({ children, edge, cls = "" }: { children?: ReactNode; edge?: "first" | "last"; cls?: string }) {
  const pad = edge === "first" ? "pl-4" : edge === "last" ? "pr-4" : "";
  return <th className={`px-2.5 py-2 font-medium ${pad} ${cls}`.replace(/\s+/g, " ").trim()}>{children}</th>;
}

export function Td({ children, edge, cls = "", title }: { children?: ReactNode; edge?: "first" | "last"; cls?: string; title?: string }) {
  const pad = edge === "first" ? "pl-4" : edge === "last" ? "pr-4" : "";
  return (
    <td title={title} className={`px-2.5 py-3 ${pad} ${cls}`.replace(/\s+/g, " ").trim()}>
      {children}
    </td>
  );
}

/** A row that opens its page when clicked anywhere; its own links stay real links. */
export function Row({ onOpen, children }: { onOpen: () => void; children: ReactNode }) {
  return (
    <tr
      className="cursor-pointer align-top transition-colors hover:bg-hover"
      onClick={(e) => {
        if (!(e.target as HTMLElement).closest("a,button")) onOpen();
      }}
    >
      {children}
    </tr>
  );
}
