import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import "./filters.css";

const SHEET = "(max-width: 640px)";

/**
 * A panel anchored to a button, in a portal on <body>: the bar scrolls sideways, which would clip an
 * absolute child, and a portal can never push the page. It sits under the anchor, flips above when
 * the room below is short, and is clamped to the viewport; on a phone (<=640px) CSS turns it into a bottom
 * sheet with a scrim. Esc closes and returns focus to the anchor; a press outside closes without stealing it.
 */
export function Popover({ anchor, onClose, label, children, width = 256 }: { anchor: RefObject<HTMLElement | null>; onClose: (refocus: boolean) => void; label: string; children: ReactNode; width?: number }) {
  const box = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  // Placed from the first render (never hidden-then-shown): a hidden element cannot take the focus the list asks for.
  const where = (): CSSProperties => {
    const a = anchor.current;
    if (!a || window.matchMedia(SHEET).matches) return {};
    const r = a.getBoundingClientRect();
    const below = window.innerHeight - r.bottom - 12;
    const above = r.top - 12;
    const up = below < 220 && above > below;
    const left = Math.max(8, Math.min(r.left, window.innerWidth - width - 8));
    return up ? { left, bottom: window.innerHeight - r.top + 4, maxHeight: Math.min(380, above) } : { left, top: r.bottom + 4, maxHeight: Math.min(380, below) };
  };
  const [style, setStyle] = useState<CSSProperties>(where);

  useLayoutEffect(() => {
    const place = () => {
      const a = anchor.current;
      if (a && a.getClientRects().length === 0) return close.current(false); // the anchor was folded away by a resize
      setStyle(where());
    };
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [anchor, width]);

  useEffect(() => {
    const down = (e: PointerEvent) => {
      const t = e.target as Node;
      if (box.current?.contains(t) || anchor.current?.contains(t)) return;
      close.current(false);
    };
    const key = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      close.current(true);
    };
    document.addEventListener("pointerdown", down);
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("pointerdown", down);
      document.removeEventListener("keydown", key);
    };
  }, [anchor]);

  return createPortal(
    <>
      <div className="flt-scrim" aria-hidden="true" />
      <div ref={box} role="dialog" aria-label={label} className="flt-pop" style={{ width, ...style }}>
        {children}
      </div>
    </>,
    document.body,
  );
}
