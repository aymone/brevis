import { useT } from "../i18n";

// THE CONSOLE'S ONE VOCABULARY OF STATE, as web/components/status.go draws it:
// a mark in a shape and the word, both in the state's colour. Filled is settled,
// a ring is waiting, a spinning ring is working, a diamond needs a person. The
// mark itself (.st, .st-<shape>) is the server stylesheet's; the colour is the
// token's text class, written out whole so it stays in the sheet.
const LOOK: Record<string, [shape: string, colour: string]> = {
  success: ["dot", "text-state-success"],
  failed: ["diamond", "text-state-failed"],
  running: ["spin", "text-state-running"],
  queued: ["ring", "text-state-queued"],
  retrying: ["spin-dashed", "text-state-retrying"],
  canceled: ["square", "text-state-canceled"],
  pending: ["ring", "text-state-pending"],
  skipped: ["square", "text-state-skipped"],
  never_ran: ["ring-dashed", "text-state-pending"],
  on_time: ["dot", "text-state-success"],
  late: ["triangle", "text-state-queued"],
  stale: ["diamond-hollow", "text-state-failed"],
  paused: ["square", "text-state-canceled"],
  continuous: ["pulse", "text-state-running"],
  no_schedule: ["ring", "text-muted"],
  unidentified: ["ring-dashed", "text-muted"],
  live: ["pulse", "text-state-success"],
  // Nothing to report is a dashed ring, the vocabulary's shape for "no information" (as unidentified and never_ran).
  unknown: ["ring-dashed", "text-muted"],
  // A stream's measured health. Four shapes, so colour never carries it alone: settled dot, quiet ring, warning
  // triangle (as "late"), failed diamond (as "failed").
  healthy: ["dot", "text-state-success"],
  quiet: ["ring", "text-muted"],
  degraded: ["triangle", "text-state-queued"],
  failing: ["diamond", "text-state-failed"],
};

/** A run's or step's status as a kind: the engine says "" for a node that never ran. */
export const runKind = (s: string) => s || "never_ran";

/** A catalog status as a kind: whatever the catalog does not name has no schedule to be late against. */
export function freshnessKind(s: string): string {
  return ["on_time", "late", "stale", "paused", "continuous", "unidentified"].includes(s) ? s : "no_schedule";
}

/** A kind's word in the reader's language; one nobody declared is its own name. */
export function useStatusWord() {
  const { t } = useT();
  return (kind: string) => (kind in LOOK ? t(`state.${kind}`) : kind);
}

const look = (kind: string) => LOOK[kind] ?? ["ring", "text-muted"];

/** The mark and the word. `label` replaces the word; "" leaves the mark alone, titled. */
export function Status({ kind, label }: { kind: string; label?: string }) {
  const word = useStatusWord()(kind);
  const [shape, colour] = look(kind);
  if (label === "") return <StatusMark kind={kind} />;
  return <span className={`st st-${shape} ${colour}`}>{label ?? word}</span>;
}

/** The mark alone, for a dense column where the word is said elsewhere. */
export function StatusMark({ kind }: { kind: string }) {
  const word = useStatusWord()(kind);
  const [shape, colour] = look(kind);
  return <span className={`st st-mark st-${shape} ${colour}`} title={word} role="img" aria-label={word} />;
}

/** "↻2" on a run on its second attempt or later; attempt counts from 0, so nothing on a first try. */
export function RetryMark({ attempt }: { attempt: number }) {
  const { t } = useT();
  if (attempt < 1) return null;
  return (
    <span
      title={t("run.retry_title", { n: attempt + 1 })}
      className="ml-1.5 inline-flex h-[17px] cursor-help items-center rounded-[4px] bg-state-retrying/13 px-[5px] font-mono text-[11px] font-medium text-state-retrying"
    >
      ↻{attempt + 1}
    </span>
  );
}

/** A destination's freshness (on_time, late, stale, ...): kept for the ingest screens. */
export function StatusBadge({ status }: { status: string }) {
  return <Status kind={freshnessKind(status)} />;
}

// The run and SQL screens were written against these two names; they are the same
// things as RetryMark and useStatusWord, kept as aliases so there is one of each.
export const Retry = RetryMark;
export const useStateWord = useStatusWord;

/** A destination's or writer's status against its schedule (the catalog's names, "unscheduled" included). */
export function Freshness({ status }: { status: string }) {
  return <Status kind={freshnessKind(status)} />;
}

export function FreshnessMark({ status }: { status: string }) {
  return <StatusMark kind={freshnessKind(status)} />;
}
