import type { ReactNode } from "react";
import { useT } from "../i18n";
import { CopyIcon } from "./copy";

type PanelProps = { title: string; count?: ReactNode; children: ReactNode };

/** One block of a page (web/components Panel): a 38px heading, a title and a faint count, over its content, which brings its own padding. */
export function Panel({ title, count, children }: PanelProps) {
  return (
    <section className="overflow-hidden rounded-card border border-line bg-surface">
      <header className="flex h-[38px] items-center gap-2 border-b border-line-soft pr-3 pl-3.5">
        <h2 className="text-[13px] font-semibold tracking-[-0.005em] whitespace-nowrap text-ink">{title}</h2>
        {count !== undefined && count !== "" && <span className="text-[11.5px] text-faint tabular-nums">{count}</span>}
      </header>
      {children}
    </section>
  );
}

export function PanelEmpty({ children }: { children: ReactNode }) {
  return <p className="px-3.5 py-3 text-[12.5px] text-muted">{children}</p>;
}

type KVProps = { items: { key: string; value: string; note?: string }[] };

/** A dense key-value grid (web/components KV): three to a row, one on a phone, a mono key, its value, a faint note and Copy on the cell's hover or focus. */
export function KV({ items }: KVProps) {
  const { t } = useT();
  return (
    <div className="grid grid-cols-1 gap-px bg-line-soft min-[860px]:grid-cols-3">
      {items.map((it) => (
        <div key={it.key} className="group min-w-0 bg-surface px-3.5 py-2">
          <div className="flex items-center gap-1.5 font-mono text-[11px] tracking-[0.02em] text-faint">
            {it.key}
            <span className="ml-auto opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
              <CopyIcon value={it.value} label={t("copy.key", { key: it.key })} />
            </span>
          </div>
          <div title={it.value} className="mt-0.5 truncate font-mono text-[12.5px] text-ink">
            {it.value}
          </div>
          {it.note && (
            <div title={it.note} className="mt-px truncate text-[11.5px] text-faint">
              {it.note}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
