import type { ReactNode } from "react";

// A page's own header and body, as layouts/base.templ draws them: one 20px title
// with the page's verbs beside it, the content below in the page's gutters. The
// title is static text, so it is always drawn, never a skeleton.
export function PageFrame({ title, actions, aside, mono, children }: { title: ReactNode; actions?: ReactNode; aside?: ReactNode; mono?: boolean; children: ReactNode }) {
  return (
    <>
      <header className="relative px-3 pt-3 pb-1 min-[860px]:px-6">
        <div className="flex min-h-8 flex-wrap items-center justify-between gap-x-6 gap-y-3">
          <div className="flex min-w-0 items-center gap-2">
            {mono ? (
              <h1 className="font-mono text-[18px] leading-tight font-medium tracking-[-0.01em] break-all text-ink">{title}</h1>
            ) : (
              <h1 className="truncate text-[20px] leading-tight font-semibold tracking-[-0.018em] text-ink">{title}</h1>
            )}
            {aside}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
        </div>
      </header>
      <div className="px-3 pb-10 min-[860px]:px-6">{children}</div>
    </>
  );
}
