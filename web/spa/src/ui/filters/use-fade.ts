import { useEffect, type RefObject } from "react";

/**
 * Edge fades for a row that scrolls sideways: `data-fade-l` / `data-fade-r` say whether there is more to
 * the left or right, and the CSS masks that edge. It tells the thumb "keep going" without a scrollbar.
 * Written straight to the element (no re-render), re-measured on scroll, on resize and after each render.
 */
export function useEdgeFade(ref: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const sync = () => {
      el.dataset.fadeL = String(el.scrollLeft > 2);
      el.dataset.fadeR = String(el.scrollLeft + el.clientWidth < el.scrollWidth - 2);
    };
    sync();
    el.addEventListener("scroll", sync, { passive: true });
    const ro = new ResizeObserver(sync);
    ro.observe(el);
    [...el.children].forEach((c) => ro.observe(c));
    return () => {
      el.removeEventListener("scroll", sync);
      ro.disconnect();
    };
  });
}
