import { useState } from "react";
import { useT } from "../i18n";
import { Icon } from "./icon";
import { btn } from "./kit";

/** Copies `text` to the clipboard and says so for a moment. A blocked clipboard leaves the button inert. */
function useCopy(text: string) {
  const [done, setDone] = useState(false);
  const copy = () => {
    navigator.clipboard
      ?.writeText(text)
      .then(() => {
        setDone(true);
        setTimeout(() => setDone(false), 1500);
      })
      .catch(() => {});
  };
  return [done, copy] as const;
}

/**
 * Copy: the icon button of a block (no word) or, `primary`, the labelled verb
 * of a footer.
 */
export function CopyButton({ text, label, primary }: { text: string; label: string; primary?: boolean }) {
  const [done, copy] = useCopy(text);
  const icon = <Icon name={done ? "check" : "copy"} className={primary ? "h-3.5 w-3.5" : "h-4 w-4"} />;
  const live = (
    <span aria-live="polite" className="sr-only">
      {done ? "✓" : ""}
    </span>
  );
  if (primary)
    return (
      <button type="button" className={btn(true)} onClick={copy}>
        {icon}
        {label}
        {live}
      </button>
    );
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={copy}
      className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-card text-muted transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong"
    >
      {icon}
      {live}
    </button>
  );
}

/** A command or a block of text to copy; it wraps rather than scrolls (so its end is never hidden) unless `scroll`, as the server's did. */
export function CodeBlock({ text, id, scroll }: { text: string; id?: string; scroll?: boolean }) {
  const { t } = useT();
  return (
    <div className="relative">
      <pre id={id} className={`overflow-x-auto rounded-card border border-line bg-canvas-subtle p-3 pr-11 font-mono text-[0.75rem] leading-relaxed text-ink${scroll ? "" : " break-all whitespace-pre-wrap"}`}>
        <code>{text}</code>
      </pre>
      <span className="absolute top-1.5 right-1.5">
        <CopyButton text={text} label={t("copy.label")} />
      </span>
    </div>
  );
}

/** The 18px Copy beside a value (web/components CopyIcon); the caller decides when it shows. */
export function CopyIcon({ value, label }: { value: string; label: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={() =>
        void navigator.clipboard?.writeText(value).then(() => {
          setDone(true);
          setTimeout(() => setDone(false), 1500);
        })
      }
      className="flex h-[18px] w-[18px] items-center justify-center rounded-[4px] text-muted hover:bg-surface-2 hover:text-ink"
    >
      <Icon name={done ? "check" : "copy"} className="h-3 w-3" />
    </button>
  );
}
