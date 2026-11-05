import { Link, useLocation } from "react-router";
import { useConfig } from "../api/session";
import { useT } from "../i18n";
import { Icon } from "../ui/icon";
import { AccountMenu } from "./account-menu";
import { SectionBadge } from "./badges";
import { openDialog } from "./dialogs";
import { Kbd } from "./kbd";
import { groups, metaFor, projectsSection, type Section } from "./nav";
import { hideHint } from "./hint";
import { setDrawer, toggleNavMode, useNavMode } from "./prefs";

const DEFAULT_LOGO = "/assets/logo.svg";

// The embedded mark inlined, so it takes the theme's colour (currentColor).
function Mark() {
  return (
    <svg viewBox="0 0 24 24" fill="none" role="img" aria-label="Brevis">
      <path d="M4 10.4C4 15.2 20 15.2 20 10.4" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

function NavLink({ s, active }: { s: Section; active: boolean }) {
  const { t } = useT();
  const name = t(`nav.${s.key}`);
  return (
    <Link
      to={s.href}
      aria-current={active ? "page" : undefined}
      data-tip={t(`nav.${s.key}.tip`)}
      data-name={name}
      data-go-key={s.goKey}
      className={
        active
          ? "sb-item flex h-8 items-center gap-2.5 rounded-card border border-line bg-surface-2 px-2.5 text-[0.85rem] font-medium text-ink shadow-[inset_2px_0_0_var(--color-accent-strong)]"
          : "sb-item flex h-8 items-center gap-2.5 rounded-card border border-transparent px-2.5 text-[0.85rem] text-muted transition-colors hover:bg-surface-2 hover:text-ink"
      }
    >
      <Icon name={s.icon} className="h-4 w-4" />
      <span data-collapse>{name}</span>
      <SectionBadge section={s.key} />
    </Link>
  );
}

export function Sidebar() {
  const { t } = useT();
  const { brand, sql } = useConfig();
  const { pathname } = useLocation();
  const active = metaFor(pathname)?.section;
  const proj = projectsSection;
  const projName = t("nav.projects");

  return (
    <>
      <aside id="app-sidebar" className="app-sidebar flex h-full w-[236px] shrink-0 flex-col border-r border-line-soft bg-canvas-subtle">
        <div className="sb-head flex h-13 shrink-0 items-center border-b border-line-soft pr-2">
          <Link to="/" data-name={brand.title} className="sb-item sb-logo flex min-w-0 flex-1 items-center gap-2.5 px-4">
            {brand.logo === DEFAULT_LOGO ? (
              <span className="block h-6 w-6 shrink-0 text-accent">
                <Mark />
              </span>
            ) : (
              brand.logo && <img src={brand.logo} alt={brand.title} className="h-6 w-6 shrink-0 rounded-card object-contain" />
            )}
            <span data-collapse className="display block truncate text-[1.05rem] leading-none text-ink">
              {brand.title}
            </span>
          </Link>
          <span className="sb-fold hidden shrink-0 min-[860px]:block">
            <FoldButton />
          </span>
          <button
            type="button"
            data-drawer-close
            aria-label={t("shell.nav_close")}
            onClick={() => setDrawer(false)}
            className="flex h-8 w-8 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink min-[860px]:hidden"
          >
            <Icon name="x" className="h-4 w-4" />
          </button>
        </div>
        <div className="sb-pad px-2.5 pt-3">
          <Link
            to={proj.href}
            data-project-switcher
            data-name={projName}
            data-tip={t("nav.projects.tip")}
            aria-current={active === proj.key ? "page" : undefined}
            className="sb-item flex h-8 w-full items-center gap-2 rounded-card border border-line px-2.5 text-[0.8rem] text-ink-2 transition-colors hover:border-line-strong hover:text-ink"
          >
            <Icon name={proj.icon} className="h-3.5 w-3.5" />
            <span data-collapse className="flex-1 truncate">
              {projName}
            </span>
            <span data-collapse>
              <Icon name="chevron-right" className="h-3.5 w-3.5" />
            </span>
          </Link>
        </div>
        <span className="sb-pad block px-2.5 pt-3">
          <button
            type="button"
            data-palette-open
            data-name={t("shell.search_open")}
            aria-haspopup="dialog"
            aria-controls="palette"
            onClick={() => openDialog("palette")}
            className="sb-item flex h-8 w-full items-center gap-2 rounded-card border border-line bg-surface px-2.5 text-[0.8rem] text-muted transition-colors hover:border-line-strong hover:text-ink"
          >
            <Icon name="search" className="h-3.5 w-3.5" />
            <span data-collapse className="flex-1 truncate text-left">
              {t("shell.search_open")}
            </span>
            <span data-collapse>
              <Kbd>⌘K</Kbd>
            </span>
          </button>
        </span>
        <nav aria-label={t("shell.sections")} className="sb-pad flex-1 overflow-y-auto px-2.5 py-3">
          {groups(sql).map((g) => (
            <div key={g.key} role="group" aria-label={t(`nav.group.${g.key}`)} className="sb-group mb-4 flex flex-col gap-0.5">
              <p data-collapse aria-hidden="true" className="px-2.5 pb-1 text-[0.68rem] font-medium tracking-[0.08em] text-faint uppercase">
                {t(`nav.group.${g.key}`)}
              </p>
              {g.sections.map((s) => (
                <NavLink key={s.key} s={s} active={active === s.key} />
              ))}
            </div>
          ))}
        </nav>
        <div className="sb-pad shrink-0 border-t border-line-soft p-2">
          <AccountMenu />
        </div>
      </aside>
      <div className="app-drawer-backdrop" aria-hidden="true" onClick={() => setDrawer(false)} />
    </>
  );
}

// Its hint says what a click does, and the label follows the state.
function FoldButton() {
  const { t } = useT();
  const short = useNavMode() === "short";
  const label = short ? t("shell.expand") : t("shell.collapse");
  return (
    <button
      type="button"
      data-nav-fold
      data-tip={label}
      data-name={label}
      aria-label={label}
      aria-expanded={!short}
      onClick={() => {
        hideHint();
        toggleNavMode();
      }}
      className="flex h-8 w-8 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink"
    >
      <span className="sb-when-full">
        <Icon name="panel-left-close" className="h-4 w-4" />
      </span>
      <span className="sb-when-short">
        <Icon name="panel-left-open" className="h-4 w-4" />
      </span>
    </button>
  );
}
