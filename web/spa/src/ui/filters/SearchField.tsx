import { useEffect, useRef } from "react";
import { useT } from "../../i18n";
import { Icon } from "../icon";
import "./filters.css";

/**
 * The bar's search: a labeled `type=search` inside a `role=search` form. `/` focuses it from anywhere
 * that is not already a field (the hint hides itself while typing, and on touch screens); Escape clears,
 * then leaves. Debouncing is the caller's (see useUrlSearch), so a local search (the log) can be instant.
 */
export function SearchField({ value, onChange, label, hotkey = true }: { value: string; onChange: (v: string) => void; label: string; hotkey?: boolean }) {
  const { t } = useT();
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!hotkey) return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey || el?.closest("input, textarea, select, [contenteditable], dialog")) return;
      e.preventDefault();
      input.current?.focus();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [hotkey]);
  return (
    <form role="search" className="flt-search" onSubmit={(e) => e.preventDefault()}>
      <div className="flt-search-box">
        <span className="flt-search-icon"><Icon name="search" className="h-4 w-4" /></span>
        <input
          ref={input}
          type="search"
          className="flt-input"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Escape") return;
            if (value) onChange("");
            else input.current?.blur();
          }}
          placeholder={label}
          aria-label={label}
          aria-keyshortcuts={hotkey ? "/" : undefined}
          maxLength={200}
          autoComplete="off"
        />
        <span className="flt-search-end">
          {value !== "" && (
            <button type="button" className="flt-search-clear" aria-label={t("runs.remove_filter")} onClick={() => { onChange(""); input.current?.focus(); }}>
              <Icon name="x" className="h-3.5 w-3.5" />
            </button>
          )}
          {hotkey && <kbd aria-hidden="true" className="flt-kbd">/</kbd>}
        </span>
      </div>
    </form>
  );
}
