import type { ReactNode } from "react";
import { useT } from "../i18n";
import { CopyButton } from "./copy";
import { Drawer } from "./drawer";

/** A stream or an endpoint in three steps, for somebody who has not set one up. */
export function HowSteps({ steps }: { steps: { kicker: string; title: string; text: ReactNode }[] }) {
  return (
    <ol className="mt-4 grid grid-cols-1 overflow-hidden rounded-card border border-line bg-surface min-[860px]:grid-cols-3">
      {steps.map((s) => (
        <li key={s.kicker} className="border-line-soft px-4 py-3.5 not-first:border-t min-[860px]:not-first:border-t-0 min-[860px]:not-first:border-l">
          <p className="text-[11px] font-medium tracking-[0.08em] text-muted uppercase">{s.kicker}</p>
          <p className="mt-1 text-[13px] font-semibold text-ink">{s.title}</p>
          <p className="mt-1 text-[12.5px] text-muted">{s.text}</p>
        </li>
      ))}
    </ol>
  );
}

/** A titled block of text to copy: a file, a command. */
function CopyBlock({ title, text }: { title: string; text: string }) {
  const { t } = useT();
  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-muted">{title}</span>
        <CopyButton text={text} label={t("loads.drawer.copy")} />
      </div>
      <pre className="overflow-x-auto rounded-card border border-line-soft bg-canvas px-3 py-2 font-mono text-[12px] text-ink-2">
        <code>{text}</code>
      </pre>
    </div>
  );
}

/**
 * A side drawer that says how a thing is made and makes nothing: the thing is
 * declared in a file and published from there, so a form here would be a
 * second source of truth.
 */
export function HowDrawer({
  id, title, lead, blocks, note,
}: {
  id: string;
  title: string;
  lead: string;
  blocks: { title: string; text: string }[];
  note: string;
}) {
  return (
    <Drawer id={id} title={title} lead={lead}>
      {blocks.map((b) => (
        <CopyBlock key={b.title} {...b} />
      ))}
      <p className="text-muted">{note}</p>
    </Drawer>
  );
}
