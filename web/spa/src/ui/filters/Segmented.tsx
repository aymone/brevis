import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { useFormat } from "../../i18n";
import { DropdownFilter } from "./DropdownFilter";
import "./filters.css";

export type SegItem = { key: string; label: string; icon?: ReactNode; count?: number | null; title?: string };

/**
 * ONE exclusive choice drawn as one control. A `radiogroup`: a single tab stop, arrows (and Home/End)
 * move AND choose, like native radios. The chosen segment is raised, bordered and bolder (never colour
 * alone). On a bar too narrow for its words it shows a compact dropdown twin instead (CSS, see FilterBar). Counts sit in a reserved, tabular slot and ghost until `pending` clears, so they arrive in place.
 */
export function Segmented({ label, items, value, onChange, pending = false }: { label: string; items: SegItem[]; value: string; onChange: (key: string) => void; pending?: boolean }) {
  const { number } = useFormat();
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const at = Math.max(0, items.findIndex((i) => i.key === value));
  const move = (e: KeyboardEvent, i: number) => {
    const next = e.key === "ArrowRight" || e.key === "ArrowDown" ? i + 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? i - 1 : e.key === "Home" ? 0 : e.key === "End" ? items.length - 1 : null;
    if (next === null) return;
    e.preventDefault();
    const to = (next + items.length) % items.length;
    refs.current[to]?.focus();
    const item = items[to];
    if (item) onChange(item.key);
  };
  return (
    <>
    <div role="radiogroup" aria-label={label} className="flt-seg">
      {items.map((it, i) => (
        <button
          key={it.key}
          ref={(el) => { refs.current[i] = el; }}
          type="button"
          role="radio"
          aria-checked={it.key === value}
          tabIndex={i === at ? 0 : -1}
          title={it.title}
          className="flt-seg-item"
          onClick={() => onChange(it.key)}
          onKeyDown={(e) => move(e, i)}
        >
          {it.icon}
          <span className="flt-seg-label" data-text={it.label}>{it.label}</span>
          {it.count !== undefined && it.count !== null && (
            <>
              <span className="sr-only">, </span>
              <span className={`flt-n ${pending ? "sk-ghost" : ""}`}>{pending ? "0" : number(it.count)}</span>
            </>
          )}
        </button>
      ))}
    </div>
    {/* The compact twin, shown instead when the bar measures too narrow for the words (see FilterBar). */}
    <span className="flt-seg-compact">
      <DropdownFilter
        def={{
          id: label, label, required: true, keepLabel: true, pending,
          options: items.map((i) => ({ key: i.key, label: i.label, icon: i.icon, count: i.count ?? undefined })),
          value: items.some((i) => i.key === value) ? [value] : [], onChange: (v) => v[0] !== undefined && onChange(v[0]),
        }}
      />
    </span>
    </>
  );
}
