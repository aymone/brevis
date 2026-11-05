import { useEffect, useRef, type KeyboardEvent, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router";
import { useT } from "../i18n";
import { Icon } from "./icon";

/**
 * A drawer's open state lives in the URL hash (`#publicar`), as the server's
 * links did: a link opens it, the opener only sets the hash, and Back/close
 * clears it. The opener and the drawer each call this with the same id.
 */
export function useDrawer(id: string) {
  const { pathname, search, hash } = useLocation();
  const navigate = useNavigate();
  const open = hash === `#${id}`;
  const go = (h: string) => navigate({ pathname, search, hash: h }, { replace: true });
  return { open, show: () => go(`#${id}`), hide: () => open && go("") };
}

const FIRST_FIELD = "[data-dialog-body] :is(input, select, textarea, button, a[href], [tabindex]):not([disabled]):not([tabindex='-1'])";

/**
 * The side drawer: a native modal <dialog>, so focus is trapped and returned,
 * Escape and the backdrop close it. `plain` is the server's components.Drawer
 * (head, body, optional foot, blurred backdrop); otherwise the "how it is
 * made" frame with a lead under the title, as in the streams and endpoints
 * drawers. Below 560px both are a full-screen sheet (width is min(…, 100vw)).
 */
export function Drawer({
  id, title, lead, kicker, width = 560, plain, foot, bodyClass = "text-[12.5px]", head, open: openProp, onHide, children,
}: {
  id: string;
  title: string;
  lead?: string;
  kicker?: ReactNode;
  width?: number;
  plain?: boolean;
  foot?: ReactNode;
  /** Type size of the body: the "how it is made" drawers set 12.5px, the test event inherits. */
  bodyClass?: string;
  /** A drawer that lives at a path, not a hash (the load's): the caller says whether it is open and what closing does. */
  open?: boolean;
  onHide?: () => void;
  /** Replaces the plain head's title; it must carry the `${id}-title` id. */
  head?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useT();
  const hash = useDrawer(id);
  const open = openProp ?? hash.open;
  const hide = onHide ?? hash.hide;
  const ref = useRef<HTMLDialogElement>(null);
  const opener = useRef<Element | null>(null);

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      opener.current = document.activeElement;
      d.showModal();
      // The reader starts on the body's first field, not on the close button.
      if (plain) d.querySelector<HTMLElement>(FIRST_FIELD)?.focus();
    }
    if (!open && d.open) d.close();
  }, [open, plain]);

  // Browsers that leave the focus on <body> after close would send a keyboard
  // user back to the top of the page.
  const closed = () => {
    hide();
    const from = opener.current;
    opener.current = null;
    if (from instanceof HTMLElement && from.isConnected && document.activeElement === document.body) from.focus();
  };

  // A modal <dialog> does not stop the page behind it from scrolling.
  useEffect(() => {
    if (!open) return;
    const root = document.documentElement;
    const was = root.style.overflow;
    root.style.overflow = "hidden";
    return () => {
      root.style.overflow = was;
    };
  }, [open]);

  const closeBtn = (
    <button
      type="button"
      onClick={() => ref.current?.close()}
      aria-label={t("dialog.close")}
      className="flex h-8 w-8 shrink-0 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink"
    >
      <Icon name="x" className="h-4 w-4" />
    </button>
  );
  // The native trap lets Tab walk out of a modal dialog into the browser's own
  // chrome; wrapping at the ends keeps the focus in the drawer.
  const trap = (e: KeyboardEvent<HTMLDialogElement>) => {
    if (e.key !== "Tab") return;
    const all = [...e.currentTarget.querySelectorAll<HTMLElement>("button, a[href], input, select, textarea, [tabindex]:not([tabindex='-1'])")].filter((n) => !n.hasAttribute("disabled"));
    const [first, last] = [all[0], all[all.length - 1]];
    if (!first || !last) return;
    if (e.shiftKey && document.activeElement === first) (e.preventDefault(), last.focus());
    else if (!e.shiftKey && document.activeElement === last) (e.preventDefault(), first.focus());
  };
  const titleId = `${id}-title`;
  return (
    <dialog
      ref={ref}
      id={id}
      aria-labelledby={titleId}
      onClose={closed}
      onKeyDown={trap}
      onClick={(e) => e.target === ref.current && ref.current?.close()}
      style={plain ? undefined : { width: `min(${width}px, 100vw)` }}
      className={
        plain
          ? "drawer"
          : "run-drawer fixed inset-y-0 right-0 left-auto m-0 h-full max-h-none border-l border-line bg-surface p-0 text-ink backdrop:bg-ink/40"
      }
    >
      {plain ? (
        <>
          <header className="dialog-head">
            {head ?? <h2 id={titleId} className="text-[0.95rem] font-semibold text-ink">{title}</h2>}
            {closeBtn}
          </header>
          <div className="dialog-body" data-dialog-body>{children}</div>
          {foot && <footer className="dialog-foot">{foot}</footer>}
        </>
      ) : (
        <div className="flex h-full flex-col">
          <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
            <div className="min-w-0">
              {kicker}
              <p id={titleId} className="text-[15px] font-semibold text-ink">{title}</p>
              {lead && <p className="mt-1 text-[12.5px] text-muted">{lead}</p>}
            </div>
            {closeBtn}
          </div>
          <div className={`flex-1 space-y-4 overflow-y-auto px-5 py-4 ${bodyClass}`}>{children}</div>
        </div>
      )}
    </dialog>
  );
}
