import { useSyncExternalStore } from "react";

// The visitor's own choices, kept in localStorage and mirrored on <html> (the
// boot script in index.html reads them before the first paint): theme as
// data-theme-pref / data-theme, sidebar as data-nav-mode (the compiled sheet
// folds the sidebar from it), the mobile drawer as data-drawer.

const root = document.documentElement;
const subs = new Set<() => void>();
const emit = () => subs.forEach((s) => s());
const subscribe = (s: () => void) => (subs.add(s), () => void subs.delete(s));

function remember(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private mode: not kept */
  }
}

export type ThemePref = "auto" | "light" | "dark";
const systemLight = matchMedia("(prefers-color-scheme: light)");

function applyTheme(pref: ThemePref) {
  root.dataset.themePref = pref;
  root.dataset.theme = pref === "auto" ? (systemLight.matches ? "light" : "dark") : pref;
  emit();
}
systemLight.addEventListener("change", () => root.dataset.themePref === "auto" && applyTheme("auto"));

export function setTheme(pref: ThemePref) {
  remember("brevis_theme", pref);
  applyTheme(pref);
}
export const toggleTheme = () => setTheme(root.dataset.theme === "light" ? "dark" : "light");
export const useThemePref = () => useSyncExternalStore(subscribe, () => (root.dataset.themePref ?? "auto") as ThemePref);

export const useNavMode = () => useSyncExternalStore(subscribe, () => (root.dataset.navMode === "short" ? "short" : "full"));
export const isFolded = () => root.dataset.navMode === "short";
export function toggleNavMode() {
  const next = isFolded() ? "full" : "short";
  root.dataset.navMode = next;
  remember("brevis_sidebar", next);
  emit();
}

export const useDrawerOpen = () => useSyncExternalStore(subscribe, () => root.dataset.drawer === "open");
export function setDrawer(open: boolean) {
  if (open) root.dataset.drawer = "open";
  else delete root.dataset.drawer;
  emit();
}

/** The language is what the SERVER draws from (a cookie): post it, then reload so the catalog follows. */
export async function chooseLang(lang: string) {
  remember("brevis_lang", lang);
  // `next` is a tiny JSON answer: the redirect the endpoint ends in costs nothing.
  await fetch("/preferences/lang", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({ lang, next: "/api/session" }),
  }).catch(() => undefined);
  location.reload();
}
