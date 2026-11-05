import type { ReactNode } from "react";

/** One block of a page (components.PanelLink): a 38px heading -- a 13px title, a faint count -- with a link at its end. */
export function Panel({ title, count, action, children }: { title: string; count?: ReactNode; action?: ReactNode; children?: ReactNode }) {
  return (
    <section className="overflow-hidden rounded-card border border-line bg-surface">
      <header className="flex h-[38px] items-center gap-2 border-b border-line-soft pr-3 pl-3.5">
        <h2 className="text-[13px] font-semibold tracking-[-0.005em] whitespace-nowrap text-ink">{title}</h2>
        {count !== undefined && <span className="text-[11.5px] text-faint tabular-nums">{count}</span>}
        {action && <span className="ml-auto text-[12px]">{action}</span>}
      </header>
      {children}
    </section>
  );
}

/** The link style of a panel's heading and of "see all" lines. */
export const linkClass = "text-ink underline decoration-line-strong underline-offset-2 hover:decoration-ink";

/** What a list says when it has nothing to list, with the command that fills it (components.Empty). */
export function Empty({ text, hint }: { text: string; hint?: string }) {
  return (
    <div className="rounded-card border border-dashed border-line px-6 py-12 text-center">
      <p className="text-sm text-muted">{text}</p>
      {hint && <p className="mt-2 font-mono text-xs text-muted">{hint}</p>}
    </div>
  );
}
