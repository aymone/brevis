import { Suspense, useEffect, useRef } from "react";
import { Outlet, useLocation } from "react-router";
import { useConfig } from "./api/session";
import { useT } from "./i18n";
import { metaFor, useCurrentCrumb } from "./shell/nav";
import { Palette } from "./shell/palette";
import { setDrawer, useDrawerOpen } from "./shell/prefs";
import { installHints } from "./shell/hint";
import { Shortcuts, useShortcuts } from "./shell/shortcuts";
import { Sidebar } from "./shell/sidebar";
import { Toasts } from "./shell/toast";
import { Topbar } from "./shell/topbar";
import { UpdateNotice, VersionModal } from "./shell/version";
import "./shell/shell.css";

// The console frame: sidebar, topbar, and beside
// them the screen. Everything here is static or already known (the session, the
// config and the catalog are read before the first render), so nothing in the
// frame waits; only the screen below it may show a skeleton.
export function Shell() {
  const { brand } = useConfig();
  const { pathname } = useLocation();
  useShortcuts();
  useEffect(() => installHints(), []);

  // The phone's drawer, as the server pages do it (assets/ui.js) and a little
  // more: Escape closes it, focus goes into it and back to its button, and while
  // it is open the page behind is inert (no Tab out of it, no scrolling it).
  const drawer = useDrawerOpen();
  const was = useRef(false);
  useEffect(() => {
    const frame = document.querySelector<HTMLElement>("[data-app-main]");
    if (frame) frame.inert = drawer;
    if (drawer) document.querySelector<HTMLElement>("#app-sidebar [data-drawer-close]")?.focus();
    else if (was.current) document.querySelector<HTMLElement>("[data-drawer-open]")?.focus();
    was.current = drawer;
    if (!drawer) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setDrawer(false);
    // Past 860px the sidebar is not a drawer any more: nothing may stay inert.
    const wide = matchMedia("(min-width: 860px)");
    const grow = () => wide.matches && setDrawer(false);
    document.addEventListener("keydown", esc);
    wide.addEventListener("change", grow);
    return () => {
      document.removeEventListener("keydown", esc);
      wide.removeEventListener("change", grow);
    };
  }, [drawer]);

  // A new page closes the phone's drawer; the title follows the brand.
  useEffect(() => setDrawer(false), [pathname]);
  // The tab reads as the old pages did: "<page> · <brand>".
  const { t } = useT();
  const meta = metaFor(pathname);
  const crumb = useCurrentCrumb();
  const page = meta ? (meta.named && crumb ? crumb.text : t(meta.title)) : "";
  useEffect(() => {
    document.title = page ? `${page} · ${brand.title}` : brand.title;
  }, [page, brand.title]);

  return (
    <>
      <div className="flex h-full">
        <Sidebar />
        <div data-app-main className="flex h-full min-w-0 flex-1 flex-col">
          <Topbar />
          <main className={`flex-1 ${drawer ? "overflow-hidden" : "overflow-y-auto"}`}>
            {/* The page draws its own header and its own body gutters, as base.templ
                does (header px-3 pt-3, body px-3 pb-10): padding here would double
                them. No skeleton as a fallback either: every screen draws its static
                frame at once and skeletons only its data. */}
            <Suspense fallback={null}>
              <Outlet />
            </Suspense>
          </main>
        </div>
      </div>
      <Palette />
      <Shortcuts />
      <VersionModal />
      <UpdateNotice />
      <Toasts />
    </>
  );
}
