import { useEffect, useRef, useState, type FormEvent } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { getJSON } from "../api/client";
import { safeNext, signIn } from "../api/login";
import { useConfig, useSession, type Config, type Session } from "../api/session";
import { bindOperator } from "../ui/seen";
import { useT } from "../i18n";
import "./login.css";

const DEFAULT_LOGO = "/assets/logo.svg";
const field =
  "h-10 w-full min-w-0 rounded-card border border-line bg-surface px-3 text-sm text-ink transition-colors outline-none placeholder:text-muted/70 focus-visible:border-accent focus-visible:ring-1 focus-visible:ring-accent/40";

function Mark() {
  // The embedded mark inlined (as sidebar.tsx and the server page do), so it takes the theme's accent.
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M4 10.4C4 15.2 20 15.2 20 10.4" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

// The sign-in screen: outside the Shell (its sidebar links lead to screens this
// visitor cannot open yet), on a flat ground, with one job.
export default function LoginRoute() {
  const { t } = useT();
  const words = {
    title: t("login.title"),
    username: t("login.username"),
    password: t("login.password"),
    show: t("login.show"),
    hide: t("login.hide"),
    locked: t("login.locked"),
    wait: (n: number) => t("login.wait", { n }),
  };
  const { brand } = useConfig();
  const session = useSession();
  const [params] = useSearchParams();
  const next = safeNext(params.get("next"));
  const navigate = useNavigate();
  const client = useQueryClient();

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [lockedUntil, setLockedUntil] = useState(0);
  const [left, setLeft] = useState(0);
  const [retry, setRetry] = useState(0);
  const [reveal, setReveal] = useState(false);
  const alertRef = useRef<HTMLDivElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    document.title = `${words.title} · ${brand.title}`;
  }, [brand.title, words.title]);

  // Focus goes to the message when one appears: a keyboard or screen-reader
  // user who pressed Enter would otherwise get no sign of what happened.
  useEffect(() => {
    if (error) alertRef.current?.focus();
  }, [error]);

  // The lockout countdown. It ends by itself: the server decides, this only shows.
  useEffect(() => {
    if (!lockedUntil) return;
    const tick = () => {
      const n = Math.max(0, Math.ceil((lockedUntil - Date.now()) / 1000));
      setLeft(n);
      if (n === 0) setLockedUntil(0);
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [lockedUntil]);

  // To where the visitor was going: a route of this app.
  const go = (to: string) => void navigate(safeNext(to), { replace: true });

  // Already signed in, or an open console (no credential, so no login): there
  // is nothing to ask. `anonymous` is what boot sets when /api/session said 401.
  if (!session.anonymous) return <Navigate to={next} replace />;

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (busy || left > 0) return;
    const form = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    // The password is read from the field when sent and is never in state; the
    // field is emptied as soon as the answer is in.
    const res = await signIn(String(form.get("username") ?? ""), String(form.get("password") ?? ""), next);
    if (passwordRef.current) passwordRef.current.value = "";
    if (!res.ok) {
      setBusy(false);
      setError(res.error || t("login.failed"));
      setRetry(0);
      if (res.status === 429 && res.retryAfter) {
        setRetry(res.retryAfter);
        setLockedUntil(Date.now() + res.retryAfter * 1000);
      }
      return;
    }
    // The cookie is set; the cached session AND config are still the anonymous
    // ones (the config a stranger gets has no version and no services), so both are
    // replaced by what this operator may see before the app draws: without the
    // config the SQL entry and the version row would be missing until a reload.
    try {
      const [real, config] = await Promise.all([getJSON<Session>("/api/session"), getJSON<Config>("/api/config")]);
      bindOperator(real.operator);
      client.setQueryData(["session"], real);
      client.setQueryData(["config"], config);
    } catch {
      // The sign-in worked; a full load re-reads everything.
      location.assign(res.next);
      return;
    }
    go(res.next);
  };

  return (
    <main className="flex min-h-full items-center justify-center bg-canvas p-6 font-sans text-ink antialiased">
      <div className="w-full max-w-[19rem]">
        <div className="mb-8 flex flex-col items-center text-center">
          {brand.logo === DEFAULT_LOGO ? (
            <span className="block h-10 w-10 text-accent">
              <Mark />
            </span>
          ) : (
            brand.logo && <img src={brand.logo} alt={brand.title} className="h-10 w-10 rounded-card object-contain" />
          )}
          <h1 className="display mt-4 text-xl text-ink">{brand.title}</h1>
          {brand.subtitle && <p className="mt-1 text-sm text-muted">{brand.subtitle}</p>}
        </div>
        {error && (
          <div
            ref={alertRef}
            tabIndex={-1}
            role="alert"
            aria-live="assertive"
            className="mb-4 rounded-card border border-state-failed/30 bg-state-failed/5 px-3 py-2.5 text-sm text-state-failed outline-none"
          >
            {retry > 0 ? words.locked : error}
            {left > 0 && (
              <>
                {" "}
                {/* The number ticks every second: it is not announced (aria-hidden), the sentence is read once. */}
                <span className="sr-only">{words.wait(retry)}</span>
                <span aria-hidden="true">{words.wait(left)}</span>
              </>
            )}
          </div>
        )}
        <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-3">
          <label htmlFor="username" className="sr-only">
            {words.username}
          </label>
          <input id="username" name="username" type="text" required autoFocus autoComplete="username" placeholder={words.username} className={field} />
          <label htmlFor="password" className="sr-only">
            {words.password}
          </label>
          <div className="login-pw">
            <input
              id="password"
              name="password"
              ref={passwordRef}
              type={reveal ? "text" : "password"}
              required
              autoComplete="current-password"
              placeholder={words.password}
              className={field}
            />
            <button type="button" className="login-eye" aria-label={reveal ? words.hide : words.show} aria-pressed={reveal} onClick={() => setReveal((v) => !v)}>
              <svg viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true">
                <path d="M1.5 8s2.3-4.5 6.5-4.5S14.5 8 14.5 8 12.2 12.5 8 12.5 1.5 8 1.5 8z" />
                <circle cx="8" cy="8" r="2" />
                {reveal && <path d="M2.5 13.5l11-11" />}
              </svg>
            </button>
          </div>
          <button
            type="submit"
            disabled={busy || left > 0}
            className="mt-1 inline-flex h-10 w-full items-center justify-center rounded-card bg-accent-strong px-4 text-sm font-medium text-on-accent transition-colors hover:bg-accent-strong/90 disabled:opacity-60"
          >
            {words.title}
          </button>
        </form>
        <p className="mt-8 text-center text-xs tracking-wide text-muted">{brand.attribution}</p>
      </div>
    </main>
  );
}
