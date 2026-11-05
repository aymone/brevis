import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useT } from "../i18n";
import { linkClass } from "./card";
import { useAgo, useBrief, useNumbers } from "./format";
import { Logos } from "./logo";
import { RetryMark, Status, runKind } from "./status";

/** A run as /api/overview and /api/runs send it. */
export type RunRow = {
  id: string;
  workflow: string;
  status: string;
  trigger: string;
  attempt: number;
  logicalDate: string | null;
  createdAt: string;
  startedAt: string | null;
  durationMs: number | null;
  error: string;
  logos: string[];
};

/** How a run was made, in words ("schedule" is "Schedule"); a kind this version does not know is shown as the engine wrote it. */
export function useTrigger() {
  const { t } = useT();
  return (kind: string) => {
    const key = `trigger.${kind}`;
    const word = t(key);
    return word === `[${key}]` ? kind : word;
  };
}

// The failure behind a run, in the browser's own modal (components.ErrorDialog):
// Esc, focus trap and return of focus come with <dialog>.
function ErrorDialog({ run, onClose }: { run: RunRow | null; onClose: () => void }) {
  const { t } = useT();
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (run && d && !d.open) d.showModal();
  }, [run]);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby="rt-err-title"
      className="w-[min(680px,92vw)] rounded-card border border-line bg-surface p-0 text-ink shadow-warm backdrop:bg-ink/40 backdrop:backdrop-blur-[2px]"
    >
      {run && (
        <>
          <div className="flex items-start justify-between gap-4 border-b border-line px-6 py-4">
            <div>
              <p className="text-[11.5px] text-muted">{t("run.error.title")}</p>
              <p id="rt-err-title" className="display mt-1 text-2xl">{run.workflow}</p>
            </div>
            <form method="dialog">
              <button
                autoFocus
                aria-label={t("dialog.close")}
                className="rounded-card border border-line px-3 py-1 text-xs text-muted transition-colors hover:border-line-strong hover:text-ink focus-visible:text-ink"
              >
                {t("dialog.close_word")}
              </button>
            </form>
          </div>
          <div className="px-6 py-5">
            <div className="mb-4 flex flex-wrap items-center gap-3 text-xs">
              <Status kind={runKind(run.status)} />
              <Link to={`/runs/${run.id}`} className={linkClass}>{t("run.error.open")}</Link>
              <span className="font-mono text-[0.7rem] text-muted/70">{run.id}</span>
            </div>
            <pre className="max-h-[50vh] overflow-auto rounded-card border border-state-failed/20 bg-state-failed/5 px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre-wrap text-state-failed">{run.error}</pre>
          </div>
        </>
      )}
    </dialog>
  );
}

const th = (extra = "") => `px-2.5 text-[11.5px] font-medium whitespace-nowrap text-faint ${extra}`;
const td = "px-3 py-3";

type Props = {
  /** undefined while the runs are on their way: the header stays, the rows are skeletons. */
  runs?: RunRow[];
  /** Show the workflow column; a workflow's own page leaves it out. */
  workflow?: boolean;
  /** Bare: the table sits inside a panel and draws no card of its own. */
  bare?: boolean;
  /** How many ghost rows to draw while loading. */
  rows?: number;
  /** Names the table for assistive technology. */
  label: string;
  /** The pager under the rows, inside the card. */
  footer?: React.ReactNode;
};

// web/pages runsTableWith: the rows are data; the header and its columns are not,
// so they are drawn whether the runs have arrived or not.
export function RunsTable({ runs, workflow = true, bare = false, rows = 5, label, footer }: Props) {
  const { t } = useT();
  const ago = useAgo();
  const brief = useBrief();
  const trigger = useTrigger();
  const { elapsed } = useNumbers();
  const [failed, setFailed] = useState<RunRow | null>(null);
  // While the runs are on their way the table draws the REAL row with placeholder
  // values (a ghost, see .sk-ghost): same height by construction.
  const ghosts = Array.from({ length: rows }, (_, i): RunRow => ({
    id: `ghost-${i}`, workflow: "workflow_name", status: "success", trigger: "schedule", attempt: 1, logicalDate: "2026-01-01T00:00:00Z",
    createdAt: "2026-01-01T00:00:00Z", startedAt: null, durationMs: 1000, error: "", logos: [],
  }));
  const rowsToDraw = runs ?? ghosts;
  const longest = Math.max(0, ...(runs ?? []).map((r) => r.durationMs ?? 0));

  return (
    <>
      <div className={bare ? "overflow-x-auto" : "overflow-hidden rounded-card border border-line bg-surface shadow-warm-sm"}>
        <div className="overflow-x-auto">
          <table className="ct w-full text-sm" aria-label={label} aria-busy={runs === undefined}>
            <thead>
              <tr className="border-b border-line bg-parchment-soft/60 text-left">
                <th className={th("pl-6")}>{t("runs.col.state")}</th>
                {workflow && <th className={th()}>{t("runs.col.workflow")}</th>}
                <th className={th()}>{t("runs.col.trigger")}</th>
                <th className={th()}>{t("runs.col.slot")}</th>
                <th className={th()}>{t("runs.col.duration")}</th>
                <th className={th("text-right")}>{t("runs.col.created")}</th>
                <th className={th("w-16 pr-6")}>
                  <span className="sr-only">{t("run.error.button")}</span>
                </th>
              </tr>
            </thead>
            <tbody className={`divide-y divide-line-soft ${runs ? "" : "sk-ghost"}`} aria-hidden={runs ? undefined : true}>
              {rowsToDraw.map((r) => (
                    <tr key={r.id} data-run-id={r.id} data-status={r.status} className="transition-colors hover:bg-hover">
                      <td className={`${td} pl-6`}>
                        {workflow ? (
                          <Status kind={runKind(r.status)} />
                        ) : (
                          <Link to={`/runs/${r.id}`} className="hover:underline">
                            <Status kind={runKind(r.status)} />
                          </Link>
                        )}
                        <RetryMark attempt={r.attempt} />
                      </td>
                      {workflow && (
                        <td className={td}>
                          <span className="inline-flex items-center gap-1.5">
                            <Logos keys={r.logos} />
                            <Link to={`/runs/${r.id}`} className="font-medium text-ink hover:underline">{r.workflow}</Link>
                          </span>
                        </td>
                      )}
                      <td className={`${td} text-xs text-muted`}>{trigger(r.trigger)}</td>
                      <td className={`${td} text-xs text-muted tabular-nums`}>
                        {r.logicalDate ? <span title={r.logicalDate} className="whitespace-nowrap tabular-nums">{brief(r.logicalDate)}</span> : <span className="text-faint">—</span>}
                      </td>
                      <td className={`${td} text-xs text-muted tabular-nums`}>
                        {r.durationMs === null ? "—" : elapsed(r.durationMs)}
                        {r.durationMs !== null && longest > 0 && (
                          <span className="mt-1 block h-1 w-24 rounded-full bg-surface-2">
                            <span className="block h-1 rounded-full bg-bar" style={{ width: `${Math.max(1, Math.floor((r.durationMs / longest) * 100))}%` }} />
                          </span>
                        )}
                      </td>
                      <td className={`${td} text-right text-xs text-muted`} title={r.createdAt}>{ago(r.createdAt)}</td>
                      <td className={`${td} pr-6 text-right`}>
                        {r.error && (
                          <button
                            type="button"
                            title={t("run.error.see")}
                            onClick={() => setFailed(r)}
                            className="inline-flex items-center gap-1 rounded-card border border-state-failed/30 bg-state-failed/5 px-2.5 py-1 text-[0.7rem] font-medium text-state-failed transition-colors hover:bg-state-failed/10"
                          >
                            <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden="true">
                              <circle cx="6" cy="6" r="4.6" />
                              <path d="M6 3.4v3M6 8.4v.2" strokeLinecap="round" />
                            </svg>
                            {t("run.error.button")}
                          </button>
                        )}
                      </td>
                    </tr>
              ))}
            </tbody>
          </table>
        </div>
        {footer}
      </div>
      <ErrorDialog run={failed} onClose={() => setFailed(null)} />
    </>
  );
}
