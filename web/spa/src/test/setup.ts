import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// jsdom has no layout engine: the few components that measure (the filter bar's
// fade) get a no-op observer. Layout itself is checked in e2e/, in a real browser.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
// RTL's own auto-cleanup needs vitest `globals`; unmount explicitly instead.
afterEach(cleanup);
// The popovers ask whether the viewport is phone-sized; jsdom has no media queries: "no match" = desktop.
window.matchMedia ??= ((query: string) => ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, onchange: null, dispatchEvent: () => false })) as typeof window.matchMedia;
