import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useFormat } from "../../i18n";
import { Icon } from "../icon";
import "./filters.css";

export type Option = { key: string; label: string; icon?: ReactNode; count?: number; disabled?: boolean; mono?: boolean };

const MANY = 8; // past this a list gets its own small search

/**
 * The choices of a dropdown, with listbox semantics: arrows, Home/End and typeahead move the focus
 * (real focus, so a screen reader follows), Enter or Space chooses, a small search appears when there are
 * many options. It is also what the combined "Filters" panel shows for each group.
 */
export function OptionList({ label, options, selected, multiple, onPick, searchLabel, noneLabel, autoFocus, pending }: {
  label: string; options: Option[]; selected: string[]; multiple: boolean; onPick: (key: string) => void;
  searchLabel?: string; noneLabel?: string; autoFocus?: boolean; pending?: boolean;
}) {
  const { number } = useFormat();
  const id = useId();
  const [find, setFind] = useState("");
  const list = useRef<HTMLUListElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const typed = useRef({ text: "", at: 0 });
  const shown = useMemo(() => options.filter((o) => o.label.toLowerCase().includes(find.trim().toLowerCase())), [options, find]);
  const searchable = !!searchLabel && options.length > MANY;
  const items = () => [...(list.current?.querySelectorAll<HTMLElement>('[role="option"]:not([aria-disabled="true"])') ?? [])];

  useEffect(() => {
    if (!autoFocus) return;
    if (search.current) return search.current.focus();
    (list.current?.querySelector<HTMLElement>('[aria-selected="true"]') ?? items()[0])?.focus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const move = (e: KeyboardEvent<HTMLElement>) => {
    const all = items();
    const i = all.indexOf(document.activeElement as HTMLElement);
    let to: number | null = null;
    if (e.key === "ArrowDown") to = i + 1;
    else if (e.key === "ArrowUp") to = i - 1;
    else if (e.key === "Home") to = 0;
    else if (e.key === "End") to = all.length - 1;
    else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey && e.key !== " ") {
      // Typeahead: keys typed within 600ms build one prefix; the next option that starts with it takes focus.
      const now = Date.now();
      typed.current = { text: (now - typed.current.at > 600 ? "" : typed.current.text) + e.key.toLowerCase(), at: now };
      const from = typed.current.text.length > 1 ? Math.max(i, 0) : i + 1;
      const hit = [...all.slice(from), ...all.slice(0, from)].find((el) => el.textContent?.trim().toLowerCase().startsWith(typed.current.text));
      if (hit) { e.preventDefault(); hit.focus(); }
      return;
    }
    if (to === null) return;
    e.preventDefault();
    if (to < 0) return search.current ? search.current.focus() : all[0]?.focus();
    all[Math.min(to, all.length - 1)]?.focus();
  };

  return (
    <>
      {searchable && (
        <div className="flt-pop-search">
          <input
            ref={search}
            type="search"
            className="flt-input"
            aria-label={searchLabel}
            placeholder={searchLabel}
            autoComplete="off"
            value={find}
            onChange={(e) => setFind(e.target.value)}
            onKeyDown={(e) => { if (e.key === "ArrowDown") { e.preventDefault(); items()[0]?.focus(); } }}
          />
        </div>
      )}
      <div className="flt-pop-body">
        <ul ref={list} role="listbox" aria-label={label} aria-multiselectable={multiple || undefined} className="flt-list" onKeyDown={move} aria-busy={pending || undefined}>
          {shown.map((o) => {
            const on = selected.includes(o.key);
            return (
              <li
                key={o.key}
                id={`${id}-${o.key}`}
                role="option"
                aria-selected={on}
                aria-disabled={o.disabled || undefined}
                tabIndex={-1}
                className="flt-opt"
                onClick={() => !o.disabled && onPick(o.key)}
                onKeyDown={(e) => { if ((e.key === "Enter" || e.key === " ") && !o.disabled) { e.preventDefault(); onPick(o.key); } }}
              >
                <span className="flt-mark" data-kind={multiple ? "box" : "dot"}>{multiple && <Icon name="check" />}</span>
                {o.icon}
                <span className={`flt-opt-text ${o.mono ? "font-mono" : ""}`}>{o.label}</span>
                {o.count !== undefined && <span className="flt-opt-n">{number(o.count)}</span>}
              </li>
            );
          })}
        </ul>
        {shown.length === 0 && !pending && noneLabel && <p className="flt-pop-note">{noneLabel}</p>}
      </div>
    </>
  );
}
