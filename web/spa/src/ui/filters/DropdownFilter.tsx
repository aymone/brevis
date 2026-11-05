import { useRef, useState } from "react";
import { useT } from "../../i18n";
import { Icon } from "../icon";
import { OptionList, type Option } from "./OptionList";
import { Popover } from "./Popover";
import "./filters.css";

/**
 * One secondary filter, as data. `value` is the chosen keys (empty = no filter). `multiple` = checkboxes;
 * otherwise one choice, with `allLabel` as the "no filter" row. `required` is a plain select (the SQL
 * connection): it always has a value, never looks "filtered" and has no clear.
 */
export type FilterDef = {
  id: string; label: string; icon?: string; options: Option[]; value: string[]; onChange: (next: string[]) => void;
  multiple?: boolean; allLabel?: string; required?: boolean; keepLabel?: boolean; mono?: boolean; pending?: boolean; searchLabel?: string; noneLabel?: string;
};

/** The options a def shows: its "all" row first (single choice only). */
export const optionsOf = (d: FilterDef): Option[] => (d.multiple || d.required || !d.allLabel ? d.options : [{ key: "", label: d.allLabel }, ...d.options]);
export const pickFrom = (d: FilterDef, key: string): string[] | null => {
  if (d.multiple) return d.value.includes(key) ? d.value.filter((k) => k !== key) : [...d.value, key];
  return key === "" ? [] : [key];
};
export const selectedOf = (d: FilterDef) => (d.value.length === 0 && !d.multiple && !d.required && d.allLabel ? [""] : d.value);
export const labelOf = (d: FilterDef, key: string) => d.options.find((o) => o.key === key)?.label ?? key;

/** The label and, when active, the chosen value or a "N" badge; opens a listbox popover. */
export function DropdownFilter({ def, className = "" }: { def: FilterDef; className?: string }) {
  const { t } = useT();
  const btn = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const active = !def.required && def.value.length > 0;
  const many = def.multiple && def.value.length > 1;
  const first = def.value[0];
  const close = (refocus: boolean) => {
    setOpen(false);
    if (refocus) btn.current?.focus();
  };
  return (
    <>
      <button
        ref={btn}
        type="button"
        className={`flt-ctl flt-dd ${className}`}
        data-active={active}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        onKeyDown={(e) => { if (e.key === "ArrowDown" && !open) { e.preventDefault(); setOpen(true); } }}
      >
        {def.icon && <Icon name={def.icon} className="h-3.5 w-3.5" />}
        {def.required && !def.keepLabel ? <span className="sr-only">{def.label}: </span> : <span className="flt-dd-label">{def.label}</span>}
        {def.required && def.pending ? <span className="sk-ghost flt-dd-value">connection</span> : null}
        {first !== undefined && !many && !(def.required && def.pending) && <span className={`flt-dd-value ${def.mono ? "font-mono" : ""}`}>{labelOf(def, first)}</span>}
        {many && <span className="flt-badge" aria-label={`${def.value.length}`}>{def.value.length}</span>}
        <Icon name="chevron-down" className="flt-chev h-3.5 w-3.5" />
      </button>
      {open && (
        <Popover anchor={btn} label={def.label} onClose={close}>
          <OptionList
            label={def.label}
            options={optionsOf(def)}
            selected={selectedOf(def)}
            multiple={!!def.multiple}
            searchLabel={def.searchLabel}
            noneLabel={def.noneLabel}
            pending={def.pending}
            autoFocus
            onPick={(k) => {
              const next = pickFrom(def, k);
              if (next) def.onChange(next);
              if (!def.multiple) close(true);
            }}
          />
          {def.multiple && def.value.length > 0 && (
            <div className="flt-pop-foot">
              <button type="button" className="flt-link" onClick={() => def.onChange([])}>{t("workflows.clear")}</button>
            </div>
          )}
        </Popover>
      )}
    </>
  );
}
