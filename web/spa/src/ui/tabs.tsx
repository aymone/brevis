import "./bar.css";
import type { KeyboardEvent, ReactNode } from "react";

// `count` is any node: a number, or a skeleton while the number is on its way.
export type TabItem = { key: string; label: string; count?: ReactNode };
type Props = { label: string; tabs: TabItem[]; active: string; onChange: (key: string) => void; idPrefix?: string };

/** The console's tablist (web/components Tabs) with arrows, Home and End. The caller owns the selection, so it can live in the URL. */
export function Tabs({ label, tabs, active, onChange, idPrefix = "tab" }: Props) {
  const move = (e: KeyboardEvent<HTMLDivElement>) => {
    const at = tabs.findIndex((t) => t.key === active);
    const next = { ArrowRight: at + 1, ArrowLeft: at - 1, Home: 0, End: tabs.length - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    const tab = tabs[(next + tabs.length) % tabs.length];
    if (!tab) return;
    onChange(tab.key);
    document.getElementById(`${idPrefix}-${tab.key}`)?.focus();
  };
  return (
    <div role="tablist" aria-label={label} onKeyDown={move} className="bar-row flex shrink-0 items-center gap-1 overflow-x-auto border-b border-line-soft">
      {tabs.map((t) => (
        <button
          key={t.key}
          id={`${idPrefix}-${t.key}`}
          role="tab"
          type="button"
          aria-selected={t.key === active}
          aria-controls={`${idPrefix}-panel-${t.key}`}
          tabIndex={t.key === active ? 0 : -1}
          onClick={() => onChange(t.key)}
          className="-mb-px shrink-0 border-b-2 border-transparent px-3 py-2 text-[0.82rem] whitespace-nowrap text-muted transition-colors duration-150 hover:text-ink focus-visible:outline-2 focus-visible:outline-accent-strong aria-selected:border-ink aria-selected:text-ink"
        >
          {t.label}
          {t.count !== undefined && <span className="ml-1.5 font-mono text-[11px] text-faint tabular-nums">{t.count}</span>}
        </button>
      ))}
    </div>
  );
}
