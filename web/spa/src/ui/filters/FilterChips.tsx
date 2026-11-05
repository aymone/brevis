import { useRef } from "react";
import { useT } from "../../i18n";
import { Icon } from "../icon";
import { useEdgeFade } from "./use-fade";
import "./filters.css";

export type Chip = { id: string; group: string; value: string; mono?: boolean; onRemove: () => void };

/**
 * The applied filters as removable chips, all in ONE style (the result line is their reserved home: they
 * can never clip a control of the row), and the one "Clear all". Each chip's
 * button names itself ("Remove this filter: Language Go"); the chips scroll sideways rather than clip,
 * while "Clear all" stays put.
 */
export function FilterChips({ chips, onClear, clearable }: { chips: Chip[]; onClear: () => void; clearable: boolean }) {
  // `clearable` shows the meta line's own copy of "Clear all" (the row's tail has the other, see FilterBar).
  const { t } = useT();
  const scroller = useRef<HTMLUListElement>(null);
  useEdgeFade(scroller);
  if (chips.length === 0 && !clearable) return null;
  return (
    <div className="flt-chips">
      <ul ref={scroller} className="flt-chips-scroll" style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {chips.map((c) => (
          <li key={c.id} className="flt-chip">
            <span className="flt-chip-k">{c.group}</span>
            <span className={`flt-chip-v ${c.mono ? "font-mono" : ""}`}>{c.value}</span>
            <button type="button" className="flt-chip-x" aria-label={`${t("runs.remove_filter")}: ${c.group} ${c.value}`} onClick={c.onRemove}>
              <Icon name="x" className="h-3 w-3" />
            </button>
          </li>
        ))}
      </ul>
      {clearable && <button type="button" className="flt-link flt-clear-meta" onClick={onClear}>{t("workflows.clear")}</button>}
    </div>
  );
}
