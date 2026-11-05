import { useState, type KeyboardEvent, type ReactNode } from "react";

export type HashTab = { key: string; label: ReactNode; count?: number; panel: ReactNode };

function fromHash(tabs: HashTab[]): string {
  const h = location.hash.slice(1);
  return tabs.some((t) => t.key === h) ? h : tabs[0]!.key;
}

/** Tabs per the ARIA pattern (arrow keys move, Home/End jump); the open one is kept in the URL hash. */
export function HashTabs({ label, tabs, id, barClass = "mb-3" }: { label: string; tabs: HashTab[]; id: string; barClass?: string }) {
  const [open, setOpen] = useState(() => fromHash(tabs));
  const current = tabs.some((t) => t.key === open) ? open : tabs[0]!.key;

  const pick = (key: string) => {
    setOpen(key);
    history.replaceState(null, "", `#${key}`);
    document.getElementById(`${id}-tab-${key}`)?.focus();
  };
  const onKey = (e: KeyboardEvent) => {
    const i = tabs.findIndex((t) => t.key === current);
    const to = { ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: tabs.length - 1 }[e.key];
    if (to === undefined) return;
    e.preventDefault();
    pick(tabs[(to + tabs.length) % tabs.length]!.key);
  };

  return (
    <div>
      <div className={barClass}>
      <div role="tablist" aria-label={label} onKeyDown={onKey} className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-line-soft">
        {tabs.map((t) => (
          <button
            key={t.key}
            id={`${id}-tab-${t.key}`}
            role="tab"
            type="button"
            aria-selected={t.key === current}
            aria-controls={`${id}-panel-${t.key}`}
            tabIndex={t.key === current ? 0 : -1}
            onClick={() => pick(t.key)}
            className="-mb-px shrink-0 border-b-2 border-transparent px-3 py-2 text-[0.82rem] whitespace-nowrap text-muted transition-colors duration-150 hover:text-ink aria-selected:border-ink aria-selected:text-ink"
          >
            {t.label}
            {t.count ? <span className="ml-1.5 font-mono text-[11px] text-faint tabular-nums">{t.count}</span> : null}
          </button>
        ))}
      </div>
      </div>
      {tabs.map((t) => (
        <div
          key={t.key}
          id={`${id}-panel-${t.key}`}
          role="tabpanel"
          aria-labelledby={`${id}-tab-${t.key}`}
          hidden={t.key !== current}
        >
          {t.panel}
        </div>
      ))}
    </div>
  );
}
