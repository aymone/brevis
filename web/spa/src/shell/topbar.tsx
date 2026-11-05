import { Link, useLocation } from "react-router";
import { useConfig } from "../api/session";
import { useT } from "../i18n";
import { Icon } from "../ui/icon";
import { Ghost } from "../ui/skeleton";
import { groups, metaFor, projectsSection, useCurrentCrumb } from "./nav";
import { setDrawer, useDrawerOpen } from "./prefs";

// Where the page is: the section's group, the section, and the page's own name
// when it is deeper than the section. The section links back to its list.
export function Topbar() {
  const { t } = useT();
  const { sql } = useConfig();
  const { pathname } = useLocation();
  const crumb = useCurrentCrumb();
  const drawer = useDrawerOpen();
  const meta = metaFor(pathname);

  let group = "";
  let section = meta?.section === projectsSection.key ? projectsSection : undefined;
  if (!section) {
    for (const g of groups(sql)) {
      const s = g.sections.find((x) => x.key === meta?.section);
      if (s) {
        group = t(`nav.group.${g.key}`);
        section = s;
      }
    }
  }
  const title = meta?.detail && crumb ? crumb : null;
  // A detail page's name arrives with its data. The trail is drawn as it will end ("Runs / ...") with the last
  // segment's box held open, so the section word does not change colour and position when the name lands.
  const waiting = !!meta?.detail && !meta.unnamed && !crumb;

  return (
    <header className="app-topbar flex h-13 shrink-0 items-center gap-3 border-b border-line-soft px-4 min-[860px]:px-6">
      <button
        type="button"
        data-drawer-open
        aria-controls="app-sidebar"
        aria-expanded={drawer}
        aria-label={t("shell.nav_open")}
        onClick={() => setDrawer(true)}
        className="flex h-8 w-8 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink min-[860px]:hidden"
      >
        <Icon name="menu" className="h-4 w-4" />
      </button>
      {section && (
        <nav aria-label={t("shell.breadcrumb")} className="flex min-w-0 items-center gap-1.5 text-[0.82rem]">
          {group && (
            <>
              <span className="shrink-0 text-muted">{group}</span>
              <span aria-hidden="true" className="text-faint">
                /
              </span>
            </>
          )}
          {title || waiting ? (
            <>
              <Link to={section.href} className="shrink-0 text-muted transition-colors hover:text-ink">
                {t(`nav.${section.key}`)}
              </Link>
              <span aria-hidden="true" className="text-faint">
                /
              </span>
              {title ? (
                <span data-crumb-current className={`truncate text-ink${title.mono ? " font-mono text-[0.78rem]" : ""}`}>
                  {title.text}
                </span>
              ) : (
                <Ghost>000000000000000</Ghost>
              )}
            </>
          ) : (
            <span className="truncate text-ink">{t(`nav.${section.key}`)}</span>
          )}
        </nav>
      )}
    </header>
  );
}
