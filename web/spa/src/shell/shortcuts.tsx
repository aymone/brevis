import type { ReactNode } from "react";
import { useEffect } from "react";
import { useNavigate } from "react-router";
import { useConfig } from "../api/session";
import { useT } from "../i18n";
import { Modal } from "./dialog";
import { openDialog } from "./dialogs";
import { Kbd } from "./kbd";
import { groups } from "./nav";

// The keyboard's ways around the console. The section letters come from the
// sidebar's own list, so a section that is not drawn has no letter here either.
export function Shortcuts() {
  const { t } = useT();
  const { sql } = useConfig();
  const keyed = groups(sql).flatMap((g) => g.sections).filter((s) => s.goKey);
  const row = (what: string, keys: ReactNode, k: string) => (
    <div key={k} className="contents">
      <dt className="flex items-center gap-1.5">{keys}</dt>
      <dd className="text-ink-2">{what}</dd>
    </div>
  );
  return (
    <Modal name="shortcuts" title={t("shortcuts.title")}>
      <dl className="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-2.5 text-[0.85rem]">
        {row(t("shortcuts.palette"), <Kbd>⌘K</Kbd>, "palette")}
        {row(t("shortcuts.search"), <Kbd>/</Kbd>, "search")}
        {keyed.map((s) =>
          row(
            t("shortcuts.go", { name: t(`nav.${s.key}`) }),
            <>
              <Kbd>g</Kbd>
              <span className="text-faint">{t("shortcuts.then")}</span>
              <Kbd>{s.goKey!}</Kbd>
            </>,
            s.key,
          ),
        )}
        {row(t("shortcuts.list"), <Kbd>?</Kbd>, "list")}
        {row(t("shortcuts.close"), <Kbd>Esc</Kbd>, "close")}
      </dl>
      <p className="mt-4 text-[0.78rem] text-muted">{t("shortcuts.note")}</p>
    </Modal>
  );
}

// Whether a key is going into a field: then it is a character, never a shortcut.
const typing = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || !!t.closest(".CodeMirror"));

/** `?` lists the shortcuts, `g` then a letter goes to a section, `/` focuses the page's search box. */
export function useShortcuts() {
  const navigate = useNavigate();
  const { sql } = useConfig();
  useEffect(() => {
    const go = new Map(groups(sql).flatMap((g) => g.sections).filter((s) => s.goKey).map((s) => [s.goKey!, s.href]));
    let pendingG = 0;
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return;
      if (e.key === "/") {
        const box = document.querySelector<HTMLInputElement>('[data-hotkey="/"]');
        if (!box) return;
        e.preventDefault();
        box.focus();
        box.select?.();
        return;
      }
      if (document.querySelector("dialog[open]")) return;
      if (e.key === "?") {
        e.preventDefault();
        openDialog("shortcuts");
        return;
      }
      if (pendingG && Date.now() - pendingG < 1500) {
        pendingG = 0;
        const to = go.get(e.key.toLowerCase());
        if (to) {
          e.preventDefault();
          navigate(to);
        }
        return;
      }
      pendingG = e.key === "g" ? Date.now() : 0;
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [navigate, sql]);
}
