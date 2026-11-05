import type { ReactNode } from "react";
import type { Place as PlaceData } from "../api/ingest";
import { useT } from "../i18n";
import { Logo, useLogoTitle } from "./logo";

/**
 * Where something lives: a mark and its name, and under it the place, the
 * path giving way to an ellipsis and never the object. `max` is the width
 * class a column allows it.
 */
export function PlaceBlock({ mark, label, where, what, title, max = "max-w-[14rem]" }: { mark?: ReactNode; label?: ReactNode; where?: string; what?: string; title?: string; max?: string }) {
  return (
    <div className={`min-w-0 ${max}`}>
      {(mark || label) && (
        <span className="flex items-center gap-1.5 text-[12px] text-ink-2">
          {mark}
          {label}
        </span>
      )}
      {where !== undefined &&
        (what === undefined ? (
          <p className="mt-0.5 truncate font-mono text-[12px] text-ink" title={title ?? where}>
            {where}
          </p>
        ) : (
          <p className="mt-0.5 flex min-w-0 font-mono text-[12px]" title={title}>
            <span className="truncate text-faint">{where}</span>
            <span className="shrink-0 text-ink">{what}</span>
          </p>
        ))}
    </div>
  );
}

/** A destination as the lists draw it: its kind, and under it where. One the manifest could not name says so. */
export function Place({ p, max }: { p: PlaceData; max?: string }) {
  const { t } = useT();
  const kindName = useLogoTitle();
  const mark = p.kind !== "" ? <Logo k={p.kind} /> : null;
  const label = p.kind !== "" ? kindName(p.kind) : null;
  if (p.uri === "")
    return (
      <div className={`min-w-0 ${max ?? "max-w-[14rem]"}`}>
        {p.kind !== "" && (
          <span className="flex items-center gap-1.5 text-[12px] text-ink-2">
            {mark}
            {label}
          </span>
        )}
        <p className="mt-0.5 text-[12px] text-faint" title={p.note}>
          {t("streams.unidentified")}
        </p>
      </div>
    );
  return <PlaceBlock mark={mark} label={label} where={p.where} what={p.what} title={p.note ? `${p.uri} — ${p.note}` : p.uri} max={max} />;
}
