import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import "./widgets.css";

const ROW = 28;
const HEAD = 30;
const NUM = 56;
const OVERSCAN = 6;

// Same rule as the server's pages.Numeric: what the cell will SAY looks like a
// number (BigQuery sends every scalar as a string).
const NUMBER = /^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$/;

/** A cell as the grid shows it: null is the word, not the empty string. */
export function cellText(v: unknown): string {
  if (v === null || v === undefined) return "null";
  if (typeof v === "string") return v;
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

type Props = {
  columns: string[];
  rows: unknown[][];
  label: string;
  rowLabel: string;
  emptyText: string;
  colWidth?: number;
};

// A table that draws only the rows in view (rows have a fixed height), so a
// hundred thousand of them cost what a screenful does. One focusable grid:
// arrows, Home/End and PageUp/PageDown move the active cell, whose full value
// shows underneath -- which is also how a truncated cell is read in full.
export function DataGrid({ columns, rows, label, rowLabel, emptyText, colWidth = 192 }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const uid = useId();
  const [top, setTop] = useState(0);
  const [view, setView] = useState({ w: 0, h: 480 });
  const [at, setAt] = useState<{ r: number; c: number } | null>(null);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setView({ w: el.clientWidth, h: el.clientHeight }));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  // A new answer starts at the top, with nothing selected.
  useEffect(() => {
    setAt(null);
    box.current?.scrollTo(0, 0);
  }, [rows]);

  const width = NUM + columns.length * colWidth;
  const first = Math.max(0, Math.floor(top / ROW) - OVERSCAN);
  const last = Math.min(rows.length, Math.ceil((top + view.h) / ROW) + OVERSCAN);
  const cellId = (r: number, c: number) => `${uid}-${r}-${c}`;

  const move = (r: number, c: number) => {
    const el = box.current;
    if (!el || rows.length === 0 || columns.length === 0) return;
    r = Math.min(rows.length - 1, Math.max(0, r));
    c = Math.min(columns.length - 1, Math.max(0, c));
    setAt({ r, c });
    const body = view.h - HEAD;
    if (r * ROW < el.scrollTop) el.scrollTop = r * ROW;
    else if ((r + 1) * ROW > el.scrollTop + body) el.scrollTop = (r + 1) * ROW - body;
    const x = NUM + c * colWidth;
    if (x < el.scrollLeft + NUM) el.scrollLeft = x - NUM;
    else if (x + colWidth > el.scrollLeft + view.w) el.scrollLeft = x + colWidth - view.w;
  };

  const onKey = (e: KeyboardEvent) => {
    const { r, c } = at ?? { r: 0, c: 0 };
    const page = Math.max(1, Math.floor((view.h - HEAD) / ROW) - 1);
    const to: Record<string, [number, number]> = {
      ArrowDown: [r + 1, c], ArrowUp: [r - 1, c], ArrowRight: [r, c + 1], ArrowLeft: [r, c - 1],
      PageDown: [r + page, c], PageUp: [r - page, c],
      Home: e.ctrlKey ? [0, 0] : [r, 0], End: e.ctrlKey ? [rows.length - 1, columns.length - 1] : [r, columns.length - 1],
    };
    const dest = to[e.key];
    if (!dest) return;
    e.preventDefault();
    move(...dest);
  };

  const active = at ? rows[at.r]?.[at.c] : undefined;
  const slice = [];
  for (let r = first; r < last; r++) {
    const row = rows[r] ?? [];
    slice.push(
      <div key={r} role="row" aria-rowindex={r + 2} className="dg-row" style={{ top: r * ROW, height: ROW, width }}>
        <span role="rowheader" className="dg-num">{r + 1}</span>
        {columns.map((_, c) => {
          const v = row[c];
          const num = v !== null && v !== undefined && NUMBER.test(cellText(v));
          return (
            <span
              key={c}
              id={cellId(r, c)}
              role="gridcell"
              aria-colindex={c + 2}
              aria-selected={at?.r === r && at.c === c}
              className={`dg-cell${num ? " dg-numeric" : ""}${v === null || v === undefined ? " dg-null" : ""}`}
              style={{ width: colWidth }}
              onClick={() => {
                setAt({ r, c });
                box.current?.focus();
              }}
            >
              {cellText(v)}
            </span>
          );
        })}
      </div>,
    );
  }

  return (
    <div className="dg">
      <div
        ref={box}
        className="dg-scroll"
        role="grid"
        tabIndex={0}
        aria-label={label}
        aria-rowcount={rows.length + 1}
        aria-colcount={columns.length + 1}
        aria-activedescendant={at && at.r >= first && at.r < last ? cellId(at.r, at.c) : undefined}
        onScroll={(e) => setTop(e.currentTarget.scrollTop)}
        onKeyDown={onKey}
      >
        <div role="row" aria-rowindex={1} className="dg-head" style={{ width, height: HEAD }}>
          <span role="columnheader" className="dg-num"><span className="sr-only">{rowLabel}</span></span>
          {columns.map((c, i) => (
            <span key={i} role="columnheader" aria-colindex={i + 2} className="dg-cell" style={{ width: colWidth }} title={c}>
              {c}
            </span>
          ))}
        </div>
        <div className="dg-body" style={{ height: rows.length * ROW, width }}>
          {slice}
        </div>
        {rows.length === 0 && <p className="dg-empty muted">{emptyText}</p>}
      </div>
      {at && (
        <pre className="dg-viewer" aria-label={columns[at.c]}>
          <strong>{columns[at.c]}</strong> {cellText(active)}
        </pre>
      )}
    </div>
  );
}
