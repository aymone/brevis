import { useLayoutEffect } from "react";

/**
 * A fill page (Runs, SQL) takes the whole viewport and scrolls its panes itself,
 * as the server's `Fill: true` pages do: <main data-fill> turns off the page's own
 * scroll and makes its last child one flex column (rules in the server's sheet,
 * main[data-fill]). Without it the panes grow with their content and the whole
 * page gets as tall as the longest list (Runs was 1663px tall at 900px).
 */
export function useFill() {
  // Layout effect: before the first paint. As a passive effect the first frame drew the
  // panes at their full content height (a scrolling page), then snapped to the viewport.
  useLayoutEffect(() => {
    const main = document.querySelector("main");
    main?.setAttribute("data-fill", "");
    return () => main?.removeAttribute("data-fill");
  }, []);
}
