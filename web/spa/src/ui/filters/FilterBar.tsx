import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useT } from "../../i18n";
import { Icon } from "../icon";
import { DropdownFilter, optionsOf, pickFrom, selectedOf, type FilterDef } from "./DropdownFilter";
import { FilterChips, type Chip } from "./FilterChips";
import { OptionList } from "./OptionList";
import { Popover } from "./Popover";
import { useEdgeFade } from "./use-fade";
import "./filters.css";

/*
 * THE FILTER KIT: why it looks and behaves like this.
 *
 * One row, left to right: [search] [primary segmented] [secondary dropdowns] [spacer] [chips + Clear all]
 * [actions]. The eye learns one place per job, on every screen.
 *
 * - One control shape (.flt-ctl): 32px tall like the app's buttons (40px when the BAR is <=640px wide, the
 *   touch minimum), the card radius, line/surface tokens, ONE focus ring (the buttons' accent outline), one
 *   hover and one disabled state. The Runs list's controls were the reference of taste; this is them,
 *   made the rule instead of the exception.
 * - Segmented = a recessed track with a RAISED chosen segment (surface + border + shadow + bold, with a
 *   hidden bold twin so widths never move). Accent stays reserved for the primary action. Counts are 12px
 *   tabular figures in muted/ink-2 (>=4.5:1 on the track), in a min-width slot so they arrive in place.
 *   A radiogroup (arrows, one tab stop) everywhere: one exclusive choice IS a radio group, and a filter that
 *   is a URL changes by script, not by link, so a button role is honest.
 * - Secondary filters are dropdown buttons that show "Label  value" or "Label  N". They fold into ONE
 *   "Filters" button with a count badge when the row MEASURES wider than the BAR (a size container, not
 *   the window, so long German or Russian words fold earlier than English), instead of being cut off. Under 640px the row scrolls sideways with edge fades and no scrollbar; it
 *   never wraps and never shows half a control.
 * - Applied filters are removable chips of ONE style in the result line under the bar, next to the count
 *   (what am I looking at, in one place; chips in the row would clip its controls or push them out). One
 *   "Clear all": in the row's tail while the bar is >=1100px, else after the chips. Chip buttons name themselves.
 * - The result line is a reserved slot (aria-live=polite): numbers arriving or changing never move the page.
 * - The bar is static UI: never a skeleton, never late. Only counts ghost. The URL stays the source of truth
 *   (callers navigate with the same parameter names), so filters stay shareable.
 * - Tabs (underline, Tabs.tsx) and filters (this) are two families on purpose: tabs change WHAT you look at,
 *   filters change WHICH of it. The same family on every screen.
 */

/** The folded "Filters" button: every secondary group in one popover, with the count of what is active. */
function FoldedFilters({ defs }: { defs: FilterDef[] }) {
  const btn = useRef<HTMLButtonElement>(null);
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const active = defs.reduce((n, d) => n + (d.required ? 0 : d.value.length), 0);
  return (
    <>
      <button ref={btn} type="button" className="flt-ctl flt-dd flt-narrow" data-active={active > 0} aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className="flt-dd-label">{t("filters.title")}</span>
        {active > 0 && <span className="flt-badge" aria-label={`${active}`}>{active}</span>}
        <Icon name="chevron-down" className="flt-chev h-3.5 w-3.5" />
      </button>
      {open && (
        <Popover anchor={btn} label={t("filters.title")} width={288} onClose={(refocus) => { setOpen(false); if (refocus) btn.current?.focus(); }}>
          {defs.map((d, i) => (
            <section key={d.id}>
              <h3 className="flt-pop-head">{d.label}</h3>
              <OptionList
                label={d.label}
                options={optionsOf(d)}
                selected={selectedOf(d)}
                multiple={!!d.multiple}
                searchLabel={d.searchLabel}
                noneLabel={d.noneLabel}
                pending={d.pending}
                autoFocus={i === 0}
                onPick={(k) => { const next = pickFrom(d, k); if (next) d.onChange(next); }}
              />
            </section>
          ))}
        </Popover>
      )}
    </>
  );
}

export type FilterBarProps = {
  search?: ReactNode;
  /** The primary segmented control(s). */
  primary?: ReactNode;
  /** Secondary filters: dropdown buttons, folded into "Filters" when the bar is narrow. */
  filters?: FilterDef[];
  chips?: Chip[];
  /** Any filter at all is on (search and segmented included): shows "Clear all". */
  clearable?: boolean;
  onClearAll?: () => void;
  /** Right-aligned verbs (wrap, copy, download...). */
  actions?: ReactNode;
  /** The ResultCount, drawn in the reserved line under the bar. */
  count?: ReactNode;
  className?: string;
};

export function FilterBar({ search, primary, filters = [], chips = [], clearable = false, onClearAll = () => {}, actions, count, className = "mb-3" }: FilterBarProps) {
  const { t, lang } = useT();
  const bar = useRef<HTMLDivElement>(null);
  const row = useRef<HTMLDivElement>(null);
  useEdgeFade(row);
  // Fold by MEASURING: try everything unfolded; if the row is wider than the bar, fold the secondary filters, and
  // if it still is, let the search shrink. Re-tried when the width, the words (language) or a chosen value
  // (a longer button) change. A layout effect, so the first paint is already the settled one.
  const [fold, setFold] = useState(false);
  const [tight, setTight] = useState(false);
  const retry = `${lang}|${filters.map((d) => d.value.join(",")).join("|")}`;
  useLayoutEffect(() => { setFold(false); setTight(false); }, [retry]);
  useEffect(() => {
    const el = bar.current;
    if (!el) return;
    let w = Math.round(el.getBoundingClientRect().width);
    const ro = new ResizeObserver(() => {
      const next = Math.round(el.getBoundingClientRect().width);
      if (next === w) return;
      w = next;
      setFold(false);
      setTight(false);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  useLayoutEffect(() => {
    const el = row.current;
    if (!el || el.scrollWidth <= el.clientWidth + 1) return;
    if (!fold && filters.length > 0) setFold(true);
    else if (!tight) setTight(true);
  });
  const tail = chips.length > 0 || clearable;
  return (
    <div ref={bar} className={`flt-bar ${className}`} data-fold={fold} data-tight={tight}>
      <div ref={row} className="flt-row">
        {search}
        {primary}
        {filters.length > 0 && (
          <>
            <span className="flt-wide">{filters.map((d) => <DropdownFilter key={d.id} def={d} />)}</span>
            <FoldedFilters defs={filters.filter((d) => !d.required)} />
          </>
        )}
        <span className="flt-gap" />
        {clearable && <button type="button" className="flt-link flt-clear-row" onClick={onClearAll}>{t("workflows.clear")}</button>}
        {actions}
      </div>
      {(count || tail) && (
        <div className="flt-meta">
          {count}
          {tail && <FilterChips chips={chips} clearable={clearable} onClear={onClearAll} />}
        </div>
      )}
    </div>
  );
}
