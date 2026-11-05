import type { KeyboardEvent, ReactNode } from "react";
import "./filters/filters.css";

export type TabItem = { key: string; label: string; count?: ReactNode };

// The UNDERLINE family (filters are the segmented family, see filters/FilterBar.tsx): components.Tabs as an ARIA tablist: arrows move between tabs, the selected one
// is the only tab stop. The panels are the caller's (id `panel-<key>`).
export function TabBar({ label, tabs, current, onSelect }: { label: string; tabs: TabItem[]; current: string; onSelect: (key: string) => void }) {
  const move = (e: KeyboardEvent, i: number) => {
    const next = e.key === "ArrowRight" ? i + 1 : e.key === "ArrowLeft" ? i - 1 : e.key === "Home" ? 0 : e.key === "End" ? tabs.length - 1 : null;
    if (next === null) return;
    e.preventDefault();
    const tab = tabs[(next + tabs.length) % tabs.length];
    if (tab) {
      onSelect(tab.key);
      document.getElementById(`tab-${tab.key}`)?.focus();
    }
  };
  return (
    <div role="tablist" aria-label={label} className="flt-tabs">
      {tabs.map((tab, i) => (
        <button
          key={tab.key}
          id={`tab-${tab.key}`}
          role="tab"
          type="button"
          aria-selected={tab.key === current}
          aria-controls={`panel-${tab.key}`}
          tabIndex={tab.key === current ? 0 : -1}
          onClick={() => onSelect(tab.key)}
          onKeyDown={(e) => move(e, i)}
          className="flt-tab"
        >
          <span className="flt-seg-label" data-text={tab.label}>{tab.label}</span>
          {tab.count !== undefined && <span className="flt-n">{tab.count}</span>}
        </button>
      ))}
    </div>
  );
}
