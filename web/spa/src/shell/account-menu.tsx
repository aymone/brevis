import { useEffect, useRef } from "react";
import { postJSON } from "../api/client";
import { clearSeen } from "../ui/seen";
import { useConfig, useSession } from "../api/session";
import { useT } from "../i18n";
import { Icon } from "../ui/icon";
import { openDialog } from "./dialogs";
import { langs } from "./langs";
import { chooseLang, setTheme, useThemePref, type ThemePref } from "./prefs";
import { versionLabel, useRelease } from "./version";

const MENUS = "details.account-menu[open], details[data-popover][open]";
const closeMenus = () => document.querySelectorAll(MENUS).forEach((d) => d.removeAttribute("open"));

const glyph = "h-4 w-4 shrink-0";
const row =
  "flex w-full items-center gap-2.5 px-3 py-2 text-left text-[0.82rem] text-muted transition-colors hover:bg-surface-2 hover:text-ink";

// A <details> already gives the open state and the keyboard; this adds Escape and
// closing when a click lands elsewhere.
function useMenuDismiss() {
  useEffect(() => {
    const click = (e: MouseEvent) => {
      document.querySelectorAll(MENUS).forEach((d) => {
        if (!d.contains(e.target as Node)) d.removeAttribute("open");
      });
    };
    const key = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      document.querySelectorAll(MENUS).forEach((d) => {
        d.removeAttribute("open");
        d.querySelector("summary")?.focus();
      });
    };
    document.addEventListener("click", click);
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("click", click);
      document.removeEventListener("keydown", key);
    };
  }, []);
}

// Sign out, then start over from the login screen with nothing of this operator's left in
// the browser: the remembered shapes and (by a full load, not a route change) the query cache.
// If the server did not confirm, the session is still alive and the page stays where it is.
async function signOut() {
  try {
    await postJSON("/api/logout", {});
  } catch {
    return;
  }
  clearSeen();
  location.assign("/login");
}

function ThemeChoice() {
  const { t } = useT();
  const pref = useThemePref();
  const opts: [ThemePref, string, string][] = [
    ["auto", "monitor", t("theme.auto")],
    ["light", "sun", t("theme.light")],
    ["dark", "moon", t("theme.dark")],
  ];
  return (
    <div className="flex items-center justify-between gap-2 px-3 py-1.5 text-[0.82rem] text-muted" role="group" aria-label={t("theme.label")}>
      <span>{t("theme.label")}</span>
      <span className="flex items-center gap-0.5">
        {opts.map(([p, icon, label]) => (
          <button
            key={p}
            type="button"
            aria-pressed={pref === p}
            aria-label={label}
            title={label}
            onClick={() => setTheme(p)}
            className="flex h-6 w-7 items-center justify-center rounded-[5px] text-muted transition-colors hover:text-ink aria-pressed:bg-surface-2 aria-pressed:text-ink"
          >
            <Icon name={icon} className="h-3.5 w-3.5" />
          </button>
        ))}
      </span>
    </div>
  );
}

function LangChoice() {
  const { t, lang } = useT();
  const current = langs.find((l) => l.lang === lang);
  return (
    <details className="relative" data-popover>
      <summary className="flex cursor-pointer list-none items-center justify-between gap-2.5 px-3 py-2 text-[0.82rem] text-muted transition-colors hover:bg-surface-2 hover:text-ink">
        <span className="flex items-center gap-2.5">
          <Icon name="globe" className="h-4 w-4" />
          {t("lang.label")}
        </span>
        <span className="font-mono text-[0.7rem] text-faint">{current?.name ?? lang}</span>
      </summary>
      <div
        data-lang-form
        className="absolute right-2 bottom-full left-2 z-30 mb-1 max-h-56 overflow-y-auto rounded-card border border-line bg-surface py-1 shadow-warm"
      >
        {langs.map((l) => (
          <button
            key={l.lang}
            type="button"
            lang={l.lang}
            aria-current={l.lang === lang ? "true" : undefined}
            onClick={() => void chooseLang(l.lang)}
            className="flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left text-[0.82rem] text-muted transition-colors hover:bg-surface-2 hover:text-ink aria-[current=true]:text-ink"
          >
            <span>{l.name}</span>
            <span className="font-mono text-[0.7rem] text-faint">{l.code}</span>
          </button>
        ))}
      </div>
    </details>
  );
}

function VersionRow() {
  const { t } = useT();
  const { version } = useConfig();
  const release = useRelease();
  const v = versionLabel(version);
  if (!v) return null;
  const identity = (
    <span className="min-w-0 flex-1">
      <span className="block text-[0.82rem] leading-tight font-semibold text-ink">Brevis</span>
      <span className="mt-1 block font-mono text-[0.72rem] leading-none text-muted">{v}</span>
    </span>
  );
  if (!release) return <div className="flex items-center gap-3 border-t border-line px-3 py-3">{identity}</div>;
  return (
    <button
      type="button"
      title={t("update.notes")}
      onClick={() => {
        closeMenus();
        openDialog("version");
      }}
      className="flex w-full items-center gap-3 border-t border-line px-3 py-3 text-left transition-colors hover:bg-surface-2"
    >
      {identity}
      {release.newer && (
        <span
          className="shrink-0 rounded-full border border-state-running/40 bg-state-running/10 px-2 py-0.5 font-mono text-[0.7rem] font-medium text-state-running"
          title={t("update.available", { version: release.tag })}
        >
          {release.tag}
        </span>
      )}
    </button>
  );
}

// The foot of the sidebar: who you are and how to leave. It opens upwards.
export function AccountMenu() {
  const { t, lang } = useT();
  const { operator } = useSession();
  const release = useRelease();
  const ref = useRef<HTMLDetailsElement>(null);
  useMenuDismiss();
  const name = operator || t("shell.account");
  const docs = `https://brevis.sh/${lang === "en" ? "" : lang === "pt-BR" ? "pt/" : lang === "zh-CN" ? "zh/" : `${lang}/`}docs/`;

  return (
    <details ref={ref} className="account-menu relative">
      <summary
        data-name={name}
        className="sb-item flex cursor-pointer list-none items-center justify-between gap-1.5 rounded-card px-3 py-2 text-[0.82rem] text-muted transition-colors hover:bg-surface-2 hover:text-ink"
      >
        <span className="flex min-w-0 items-center gap-2">
          <svg className="h-4 w-4 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
            <circle cx="8" cy="5.5" r="2.75" />
            <path d="M2.5 14c0-2.6 2.5-4.2 5.5-4.2s5.5 1.6 5.5 4.2" />
          </svg>
          <span data-collapse className="truncate" title={operator || undefined}>
            {name}
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-1.5">
          {release?.newer && (
            <span className="text-state-running" data-sb-badge data-tip={t("update.available", { version: release.tag })} data-update-icon>
              <Icon name="arrow-up-circle" className="h-4 w-4" />
            </span>
          )}
          <svg
            data-collapse
            className="account-menu-arrow h-3.5 w-3.5 shrink-0 transition-transform"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            aria-hidden="true"
          >
            <path d="M4 10l4-4 4 4" />
          </svg>
        </span>
      </summary>
      <div className="absolute right-0 bottom-full left-0 z-20 mb-1 rounded-card border border-line bg-surface shadow-warm">
        <div className="py-1">
          <LangChoice />
          <ThemeChoice />
          {operator && (
            // A POST, not a link: a GET that signs somebody out is a URL anybody can put in an <img>.
            // The body ({}) is what makes it application/json, the only type the server takes (CSRF).
            <button type="button" className={row} onClick={() => void signOut()}>
              <svg className={glyph} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
                <path d="M6.5 13.5H3.5v-11h3" />
                <path d="M9.5 11l2.5-3-2.5-3" />
                <path d="M12 8H6" />
              </svg>
              {t("shell.sign_out")}
            </button>
          )}
          <div className="my-1 border-t border-line-soft" />
          <a href={docs} target="_blank" rel="noreferrer" className={row}>
            <svg className={glyph} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
              <path d="M3 2.5h6.5L13 6v7.5H3z" />
              <path d="M9.5 2.5V6H13" />
            </svg>
            {t("shell.docs")}
          </a>
          <button
            type="button"
            title={t("shortcuts.tip")}
            onClick={() => {
              closeMenus();
              openDialog("shortcuts");
            }}
            className="flex w-full items-center justify-between gap-2.5 px-3 py-2 text-left text-[0.82rem] text-muted transition-colors hover:bg-surface-2 hover:text-ink"
          >
            <span className="flex items-center gap-2.5">
              <Icon name="keyboard" className="h-4 w-4" />
              {t("shortcuts.title")}
            </span>
            <span className="font-mono text-[0.7rem] text-faint">?</span>
          </button>
        </div>
        <VersionRow />
      </div>
    </details>
  );
}
