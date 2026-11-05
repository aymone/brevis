import { Fragment, type ReactNode } from "react";
import { Link } from "react-router";
import { stamp, useBrief } from "./format";
import { Icon } from "./icon";

// The server's small parts (web/components), as one set: buttons, a titled
// card, the empty state, the method pill, the lead line. Classes are the
// server sheet's own.

const BASE =
  "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-card font-medium whitespace-nowrap transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong disabled:pointer-events-none disabled:opacity-50 h-8 px-3 text-[0.82rem]";
/** A button's look; `primary` is the one thing a screen most wants done. */
export const btn = (primary = false) =>
  `${BASE} ${primary ? "bg-accent-strong text-on-accent hover:bg-accent-strong/90" : "border border-line bg-surface text-ink hover:bg-surface-2"}`;

export function ButtonLink({ to, icon, primary, children }: { to: string; icon?: string; primary?: boolean; children: ReactNode }) {
  return (
    <Link to={to} className={btn(primary)}>
      {icon ? <Icon name={icon} className="h-3.5 w-3.5" /> : null}
      {children}
    </Link>
  );
}

/** A titled card, with a count (or note) beside the title and `action` at the end of the header. */
export function Card({ title, count, action, id, children }: { title: ReactNode; count?: ReactNode; action?: ReactNode; id?: string; children: ReactNode }) {
  return (
    <section id={id} className="overflow-hidden rounded-card border border-line bg-surface">
      <header className="flex h-[38px] items-center gap-2 border-b border-line-soft pr-3 pl-3.5">
        <h2 className="text-[13px] font-semibold tracking-[-0.005em] whitespace-nowrap text-ink">{title}</h2>
        {count !== undefined && count !== "" ? <span className="text-[11.5px] text-faint tabular-nums">{count}</span> : null}
        {action ? <span className="ml-auto">{action}</span> : null}
      </header>
      {children}
    </section>
  );
}

/** Absence of data without looking like an error. */
export function Empty({ msg, hint, children }: { msg: ReactNode; hint?: ReactNode; children?: ReactNode }) {
  return (
    <div className="rounded-card border border-dashed border-line px-6 py-12 text-center">
      <p className="text-sm text-muted">{msg}</p>
      {hint ? <p className="mt-1 font-mono text-xs text-faint">{hint}</p> : null}
      {children}
    </div>
  );
}

export function MethodPill({ method }: { method: string }) {
  return (
    <span
      className={`inline-flex h-5 shrink-0 items-center rounded-card px-1.5 font-mono text-[0.68rem] font-semibold ${
        method === "GET" ? "bg-state-success/13 text-state-success" : "bg-surface-2 text-muted"
      }`}
    >
      {method}
    </span>
  );
}

/** The one-line lead under a screen's title, its middle word in ink. */
export function Lead({ before, strong, after, cls = "mb-3" }: { before: string; strong: string; after: string; cls?: string }) {
  return (
    <p className={`${cls} max-w-3xl text-[13.5px] text-muted`}>
      {before} <strong className="font-medium text-ink">{strong}</strong>
      {after}
    </p>
  );
}

/**
 * The page's own header (the server layout's): an optional eyebrow above, one
 * title, and the page's verbs at the right. The row is always a button tall,
 * so a page with actions and one without put the title on the same line.
 */
export function PageHead({ title, mono, eyebrow, actions }: { title: ReactNode; mono?: boolean; eyebrow?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="relative pt-3 pb-1">
      <div className="flex min-h-8 flex-wrap items-center justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          {eyebrow ? <div className="absolute top-0.5 left-0 flex flex-wrap items-center gap-2 leading-none">{eyebrow}</div> : null}
          {mono ? (
            <h1 className="font-mono text-[18px] leading-tight font-medium tracking-[-0.01em] break-all text-ink">{title}</h1>
          ) : (
            <h1 className="truncate text-[20px] leading-tight font-semibold tracking-[-0.018em] text-ink">{title}</h1>
          )}
        </div>
        {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
      </div>
    </header>
  );
}

export const linkUnderline = "text-ink underline decoration-line-strong underline-offset-2 hover:decoration-ink";

export type Def = { term: ReactNode; value: ReactNode; mono?: boolean };

/** Terms and values: the terms take what the longest needs, a machine's value is mono and may break anywhere. */
export function DefinitionList({ rows }: { rows: Def[] }) {
  return (
    <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-2 text-[0.82rem]">
      {rows.map((r, i) => (
        <Fragment key={i}>
          <dt className="text-muted">{r.term}</dt>
          <dd className={r.mono ? "font-mono text-[0.78rem] break-all text-ink" : "text-ink"}>{r.value}</dd>
        </Fragment>
      ))}
    </dl>
  );
}

/** A strip of numbers: cells of at least 150px, as many to a row as fit, hairlines being the gap. */
export function StatCells({ items }: { items: { label: string; icon?: string; value: ReactNode }[] }) {
  return (
    <dl className="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-px overflow-hidden rounded-card border border-line bg-line-soft">
      {items.map((s) => (
        <div key={s.label} className="min-w-0 bg-surface px-3.5 py-2.5">
          <dt className="flex items-center gap-1.5 text-[11.5px] whitespace-nowrap text-muted">
            {s.icon ? <Icon name={s.icon} className="h-3.5 w-3.5" /> : null}
            <span>{s.label}</span>
          </dt>
          <dd className="mt-[3px] flex flex-wrap items-baseline gap-x-1.5 text-[18px] leading-[1.2] font-semibold tracking-[-0.02em] text-ink tabular-nums">{s.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/** A date as a table cell shows it: `Oct 16 02:23`, in UTC, the full UTC in the title. */
export function ShortStamp({ at }: { at: string | null }) {
  const brief = useBrief();
  if (!at) return <span className="text-faint">—</span>;
  return (
    <span title={stamp(at)} className="tabular-nums whitespace-nowrap">
      {brief(at)}
    </span>
  );
}

/**
 * The page's frame, with the gutters base.templ gives a page (px-3, px-6 from 860px,
 * and pb-10 under the content); the shell adds none. It never asks for more than the
 * column, so a wide table or filter row scrolls inside itself.
 */
export function Page({ children }: { children: ReactNode }) {
  return <div className="w-0 min-w-full px-3 pb-10 min-[860px]:px-6">{children}</div>;
}
