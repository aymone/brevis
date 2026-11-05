import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { getJSON } from "../api/client";
import { useMsg, useT, type Msg } from "../i18n";
import { Icon } from "../ui/icon";
import { openDialog, useOpenDialog } from "./dialogs";
import { Kbd } from "./kbd";
import { chooseLang, toggleTheme } from "./prefs";

// The command palette: a native <dialog> opened on Cmd/Ctrl-K, filled from
// GET /api/palette (fetched on the first open, filtered here). Ranked, not
// fuzzy: the label starting with the query, then a word in it, then anywhere.
// The server sends what each entry is (a name, or a sentence as a message); what a search looks in is built here from
// what is said, in the reader's language.
type Wire = { action?: string; group: string; label: string | Msg; hint?: string | Msg; href: string; icon: string };
type Item = { action?: string; group: string; groupLabel: string; label: string; hint?: string; href: string; icon: string; text: string };

const ORDER = ["Runs", "Sections", "Workflows", "Destinations", "Endpoints", "Projects"];
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function rank(it: Item, q: string) {
  const label = it.label.toLowerCase();
  if (label.startsWith(q)) return 0;
  if (label.split(/[^\p{L}\p{N}]+/u).some((w) => w.startsWith(q))) return 1;
  return it.text.includes(q) ? 2 : -1;
}

function search(index: Item[] | undefined, raw: string, openRun: (id: string) => Item): Item[] {
  const q = raw.trim().toLowerCase();
  const out: Item[] = UUID.test(q) ? [openRun(q)] : [];
  if (!index) return out;
  if (q === "") return out.concat(index.filter((it) => it.group === "Sections"));
  const groups: Record<string, { it: Item; r: number; n: number }[]> = {};
  index.forEach((it, n) => {
    const r = rank(it, q);
    if (r >= 0) (groups[it.group] ??= []).push({ it, r, n });
  });
  const names = Object.keys(groups);
  names.forEach((g) => groups[g]!.sort((a, b) => a.r - b.r || a.n - b.n));
  names.sort((a, b) => groups[a]![0]!.r - groups[b]![0]!.r || ORDER.indexOf(a) - ORDER.indexOf(b));
  names.forEach((g) => groups[g]!.slice(0, 8).forEach((x) => out.push(x.it)));
  return out.slice(0, 40);
}

export function Palette() {
  const { t } = useT();
  const navigate = useNavigate();
  const dialog = useRef<HTMLDialogElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const returnTo = useRef<Element | null>(null);
  const open = useOpenDialog() === "palette";
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);

  const index = useQuery({
    queryKey: ["palette"],
    enabled: open,
    staleTime: 60_000,
    retry: false,
    queryFn: async () => (await getJSON<{ items: Wire[] }>("/api/palette")).items,
  });

  const msg = useMsg();
  const say = (v: string | Msg | undefined) => (typeof v === "string" ? v : msg(v));
  const items = useMemo(
    () =>
      index.data?.map((w): Item => {
        const label = say(w.label);
        const hint = say(w.hint);
        return { ...w, label, hint, groupLabel: t(`palette.group.${w.group.toLowerCase()}`), text: `${label}\n${hint}`.toLowerCase() };
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [index.data, t],
  );
  const shown = useMemo(
    () =>
      search(items, q, (id) => ({
        group: "Runs",
        groupLabel: t("palette.group.runs"),
        label: t("palette.open_run", { id: id.slice(0, 8) }),
        hint: id,
        href: `/runs/${id}`,
        icon: "history",
        text: id,
      })),
    [items, q, t],
  );
  const cur = shown.length ? Math.min(active, shown.length - 1) : 0;

  useEffect(() => {
    const d = dialog.current;
    if (!d) return;
    if (open && !d.open) {
      returnTo.current = document.activeElement;
      setQ("");
      setActive(0);
      d.showModal();
      input.current?.focus();
    }
    if (!open && d.open) d.close();
  }, [open]);

  // Cmd/Ctrl-K, heard in the capture phase so an editor below cannot swallow it.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey || !e.key) return;
      if (e.key.toLowerCase() === "k") {
        e.preventDefault();
        e.stopPropagation();
        openDialog(open ? null : "palette");
      }
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [open]);

  useEffect(() => {
    document.getElementById(`palette-opt-${cur}`)?.scrollIntoView?.({ block: "nearest" });
  }, [cur, shown]);

  function go(i: number, beside: boolean) {
    const it = shown[i];
    if (!it) return;
    openDialog(null);
    if (it.action === "toggle-theme") return toggleTheme();
    if (it.action?.startsWith("set-lang:")) return void chooseLang(it.action.slice("set-lang:".length));
    if (beside) window.open(it.href, "_blank");
    else navigate(it.href);
  }

  const empty = index.isError ? t("palette.failed") : index.data ? t("palette.empty") : t("palette.loading");
  let last = "";

  return (
    <dialog
      ref={dialog}
      id="palette"
      aria-label={t("palette.label")}
      className="palette rounded-panel border border-line bg-surface p-0 text-ink shadow-warm"
      onClose={() => {
        if (open) openDialog(null);
        (returnTo.current as HTMLElement | null)?.focus?.();
        returnTo.current = null;
      }}
      onClick={(e) => e.target === dialog.current && openDialog(null)}
    >
      <div className="flex items-center gap-2 border-b border-line-soft px-3">
        <span className="text-muted">
          <Icon name="search" className="h-4 w-4" />
        </span>
        <input
          ref={input}
          id="palette-input"
          type="text"
          role="combobox"
          aria-controls="palette-list"
          aria-expanded={shown.length > 0}
          aria-autocomplete="list"
          aria-label={t("palette.label")}
          aria-activedescendant={shown.length ? `palette-opt-${cur}` : undefined}
          placeholder={t("palette.placeholder")}
          autoComplete="off"
          spellCheck={false}
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setActive(0);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              if (shown.length) setActive((cur + 1) % shown.length);
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              if (shown.length) setActive((cur - 1 + shown.length) % shown.length);
            } else if (e.key === "Enter") {
              e.preventDefault();
              go(cur, e.metaKey || e.ctrlKey);
            }
          }}
          className="h-11 min-w-0 flex-1 bg-transparent text-sm text-ink placeholder:text-faint focus:outline-none"
        />
        <Kbd>esc</Kbd>
      </div>
      <div id="palette-list" role="listbox" aria-label={t("palette.results")} className="max-h-[min(60vh,28rem)] overflow-y-auto p-1.5">
        {shown.map((it, i) => {
          const heading = it.group !== last;
          last = it.group;
          return (
            <div key={`${it.group}:${it.href}:${it.label}:${it.action ?? ""}`} className="contents">
              {heading && (
                <div role="presentation" className="px-2.5 pt-2.5 pb-1 text-[0.68rem] font-medium tracking-[0.08em] text-faint uppercase">
                  {it.groupLabel || it.group}
                </div>
              )}
              <div
                id={`palette-opt-${i}`}
                role="option"
                aria-selected={i === cur}
                onMouseMove={() => i !== cur && setActive(i)}
                onClick={(e) => go(i, e.metaKey || e.ctrlKey)}
                className="flex cursor-pointer items-center gap-2.5 rounded-card px-2.5 py-1.5 text-[0.82rem] text-ink-2 aria-selected:bg-surface-2 aria-selected:text-ink"
              >
                <span className="flex shrink-0 text-muted">
                  <Icon name={it.icon} className="h-4 w-4" />
                </span>
                <span className="min-w-0 truncate">{it.label}</span>
                <span className="ml-auto shrink-0 truncate pl-3 font-mono text-[0.72rem] text-faint">{it.hint}</span>
              </div>
            </div>
          );
        })}
        {shown.length === 0 && (
          <div role="presentation" className="px-2.5 pt-2.5 pb-1 text-[0.68rem] font-medium tracking-[0.08em] text-faint uppercase">
            {empty}
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 border-t border-line-soft px-3 py-2 text-[0.72rem] text-muted">
        <span className="flex items-center gap-1">
          <Kbd>↑↓</Kbd>
          <span>{t("palette.navigate")}</span>
        </span>
        <span className="flex items-center gap-1">
          <Kbd>↵</Kbd>
          <span>{t("palette.open")}</span>
        </span>
        <span className="flex items-center gap-1">
          <Kbd>⌘↵</Kbd>
          <span>{t("palette.new_tab")}</span>
        </span>
      </div>
    </dialog>
  );
}
