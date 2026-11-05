import { lazy, Suspense, type ReactNode } from "react";
import { useFormat, useMsg, useT } from "../i18n";
import type { Run, Stage, StepRow } from "../api/run";
import { useAutoValue } from "../ui/format";
import { Icon } from "../ui/icon";
import { langLabel } from "../ui/lang";
import { KV, Panel, PanelEmpty } from "../ui/panel";
import { Skeleton } from "../ui/skeleton";
import { dagHeight } from "../ui/dag/height";
import { Retry, Status, StatusMark } from "../ui/status";

// The graph's chunk (React Flow) loads only when the Steps tab is first opened.
const Dag = lazy(() => import("../ui/dag/dag"));

// The step table's columns, shared by its header and its rows.
const GRID = "grid grid-cols-[minmax(9rem,1.3fr)_6.5rem_2.5rem_3rem_4.5rem_minmax(0,3fr)] items-center gap-x-3 px-3.5";
const HEAD = "list-none hover:bg-surface-2 [&::-webkit-details-marker]:hidden cursor-pointer h-[34px]";
const ROW = "h-[34px] border-b border-line-soft last:border-b-0";
const chev = (g: string) => <Icon name="chevron-right" className={`h-3 w-3 text-faint transition-transform ${g}`} />;

function Cells({ r, opens }: { r: StepRow; opens: boolean }) {
  const { duration } = useFormat();
  const msg = useMsg();
  return (
    <>
      <span role="cell" className="flex min-w-0 items-center gap-1.5">
        {opens ? chev("group-open/row:rotate-90") : <span aria-hidden="true" className="w-3 shrink-0" />}
        {r.item ? <span className="truncate pl-4 font-mono text-[12px] text-ink">{r.item}</span> : <span className="truncate font-mono text-ink">{r.node}</span>}
      </span>
      <span role="cell">
        <Status kind={r.status} />
      </span>
      <span role="cell">{r.attempt > 0 && <Retry attempt={r.attempt} />}</span>
      <span role="cell" className="font-mono text-[12px] text-muted">
        {r.exitCode ?? ""}
      </span>
      <span role="cell" className="text-right font-mono text-[12px] text-muted tabular-nums">
        {r.durationMs > 0 ? duration(r.durationMs) : ""}
      </span>
      <span role="cell" title={msg(r.command)} className="min-w-0 truncate font-mono text-[12px] text-muted">
        {msg(r.command)}
      </span>
    </>
  );
}

// A row with an error opens on it; one without is a plain row.
function Row({ r }: { r: StepRow }) {
  return r.err ? (
    <details role="rowgroup" className="group/row border-b border-line-soft last:border-b-0">
      <summary role="row" className={`${GRID} ${HEAD}`}>
        <Cells r={r} opens />
      </summary>
      <p className="mx-3.5 mb-2 rounded-card bg-state-failed/5 px-3 py-2 text-[12px] text-state-failed">{r.err}</p>
    </details>
  ) : (
    <div role="row" className={`${GRID} ${ROW}`}>
      <Cells r={r} opens={false} />
    </div>
  );
}

function StepsTable({ run }: { run: Run | undefined }) {
  const { t, n } = useT();
  // A run with no step rows has no table; while the run is on its way the table's frame waits for them.
  if (run && run.steps.length === 0) return null;
  return (
    <section className="mt-3">
      <Panel title={t("run.steps.output")} count={run ? n("run.steps.count", run.steps.length) : <Skeleton w="3.5rem" h="0.7rem" />}>
        <div className="overflow-x-auto">
          <div role="table" aria-label={t("run.steps.output")} className="min-w-[44rem] text-[12.5px]">
            <div role="row" className={`h-[30px] border-b border-line-soft text-[11.5px] font-medium text-faint ${GRID}`}>
              <span role="columnheader">{t("run.steps.step")}</span>
              <span role="columnheader">{t("run.steps.state")}</span>
              <span role="columnheader">
                <span className="sr-only">{t("run.steps.retry")}</span>
              </span>
              <span role="columnheader">{t("run.steps.exit")}</span>
              <span role="columnheader" className="text-right">
                {t("run.steps.duration")}
              </span>
              <span role="columnheader">{t("run.steps.command")}</span>
            </div>
            {!run
              ? [0, 1, 2].map((i) => (
                  <div key={i} aria-hidden="true" className={`${GRID} ${ROW}`}>
                    <Skeleton w="70%" h="0.8rem" />
                    <Skeleton w="4rem" h="0.8rem" />
                    <span />
                    <span />
                    <Skeleton w="2.5rem" h="0.8rem" />
                    <Skeleton w="60%" h="0.8rem" />
                  </div>
                ))
              : run.steps.map((g) =>
                  g.mapped ? (
                    <details key={g.node} role="rowgroup" className="group/map border-b border-line-soft last:border-b-0">
                      <summary role="row" className={`${GRID} ${HEAD}`}>
                        <span role="cell" className="flex min-w-0 items-center gap-1.5 font-mono text-ink">
                          {chev("shrink-0 group-open/map:rotate-90")}
                          <span className="truncate">{g.node}</span>
                        </span>
                        <span role="cell">
                          <Status kind={g.status} />
                        </span>
                        <span role="cell" />
                        <span role="cell" />
                        <span role="cell" />
                        <span role="cell" className="font-mono text-[12px] text-muted">
                          {t("run.steps.ok", { ok: g.ok, total: g.total })}
                        </span>
                      </summary>
                      <div className="bg-canvas/40">
                        {g.rows.map((r) => (
                          <Row key={`${r.node}#${r.item}#${r.attempt}`} r={r} />
                        ))}
                      </div>
                    </details>
                  ) : (
                    g.rows.map((r) => <Row key={`${r.node}#${r.attempt}`} r={r} />)
                  ),
                )}
          </div>
        </div>
      </Panel>
    </section>
  );
}

const chip = "rounded-[4px] border border-line-strong px-1.5 font-mono text-[11px] text-ink-2";
const STAGE_TONE: Record<string, string> = { done: "success", running: "running", failed: "failed", aborted: "canceled" };

function StageItem({ st }: { st: Stage }) {
  const { duration, number } = useFormat();
  const msg = useMsg();
  return (
    <li className="min-w-0 bg-surface px-3.5 py-2">
      <div className="flex items-center gap-1.5 text-[12.5px]">
        <StatusMark kind={STAGE_TONE[st.state] ?? "pending"} />
        <span className="font-medium text-ink">{msg(st.label)}</span>
        <span className="ml-auto font-mono text-[11.5px] text-faint">{st.ms !== null ? duration(st.ms) : "—"}</span>
      </div>
      <div className="mt-0.5 flex min-w-0 items-baseline gap-2 text-[11.5px] text-muted">
        {st.numbers.map((x, i) => (
          <span key={x.key} className="contents">
            {i > 0 && (
              <span aria-hidden="true" className="text-faint">
                ·
              </span>
            )}
            <span className="shrink-0">
              <b className="font-mono font-medium text-ink-2">{typeof x.value === "number" ? number(x.value) : x.value}</b> {x.key}
            </span>
          </span>
        ))}
        {st.detail && (
          <span title={st.detail} className="min-w-0 truncate font-mono text-faint">
            {st.detail}
          </span>
        )}
      </div>
    </li>
  );
}

// A one-step run draws its step as a line where a larger run has its graph.
function StepLine({ run }: { run: Run }) {
  const { t, n } = useT();
  const { duration } = useFormat();
  const s = run.step;
  if (!s) return null;
  return (
    <section className="mb-3">
      <Panel title={t("run.tab.steps")} count="1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 px-3.5 py-2.5 text-[12.5px]">
          {s.status === "" ? <span aria-hidden="true" className="inline-block h-2.5 w-2.5 rounded-full border border-line-strong" /> : <StatusMark kind={s.status} />}
          <span className="font-mono font-medium text-ink">{s.name}</span>
          {s.action && (
            <span title={s.action} className="max-w-[28rem] min-w-0 truncate font-mono text-[12px] text-muted">
              {s.action}
            </span>
          )}
          {s.runtime && (
            <span title={s.runtimeInferred ? t("run.step.inferred") : undefined} className={`${chip}${s.runtimeInferred ? " border-dashed" : ""}`}>
              {langLabel(s.runtime)}
            </span>
          )}
          {s.sdk && <span className={chip}>SDK {s.sdk}</span>}
          {s.status !== "" && (
            <span className="ml-auto shrink-0 text-[11.5px] text-faint">
              {s.published > 0 && `${n("graph.published", s.published)} · `}
              {s.durationMs > 0 ? duration(s.durationMs) : "—"}
            </span>
          )}
        </div>
        {s.stages.length > 0 && (
          <ol className="grid grid-cols-1 gap-px border-t border-line-soft bg-line-soft min-[640px]:grid-cols-2 min-[1100px]:auto-cols-fr min-[1100px]:grid-flow-col min-[1100px]:grid-cols-none">
            {s.stages.map((st) => (
              <StageItem key={JSON.stringify(st.label)} st={st} />
            ))}
          </ol>
        )}
      </Panel>
    </section>
  );
}

function AlertLine({ a }: { a: Run["alerts"][number] }) {
  const { t, n } = useT();
  const { dateTime } = useFormat();
  const say = useMsg();
  return (
    <>
      <Status kind={a.tone} />
      <span className="min-w-0 truncate text-ink">{say(a.wording)}</span>
      <span className="font-mono text-[12px] text-muted">{a.channel}</span>
      {a.attempts > 0 && <span className="text-[11.5px] text-muted">{n("run.alert.tries", a.attempts)}</span>}
      <span className="ml-auto shrink-0 text-[11.5px] text-faint">{a.deliveredAt ? dateTime(a.deliveredAt) : `${t("run.alert.raised")} ${dateTime(a.createdAt)}`}</span>
    </>
  );
}

// One 34px line per alert, its error opening under it like the run's.
function Alerts({ run }: { run: Run }) {
  const { t } = useT();
  if (run.alerts.length === 0) return null;
  return (
    <section className="mt-3">
      <Panel title={t("run.alerts")} count={String(run.alerts.length)}>
        {run.alerts.map((a, i) =>
          a.err ? (
            <details key={i} className="group/alert border-b border-line-soft last:border-b-0">
              <summary className="flex h-[34px] cursor-pointer list-none items-center gap-3 px-3.5 text-[12.5px] hover:bg-surface-2 [&::-webkit-details-marker]:hidden">
                <AlertLine a={a} />
                {chev("shrink-0 group-open/alert:rotate-90")}
              </summary>
              <pre className="mx-3.5 mb-2.5 overflow-auto rounded-card border border-state-failed/20 bg-state-failed/5 px-3 py-2 font-mono text-[12px] whitespace-pre-wrap text-state-failed">{a.err}</pre>
            </details>
          ) : (
            <div key={i} className="flex h-[34px] items-center gap-3 border-b border-line-soft px-3.5 text-[12.5px] last:border-b-0">
              <AlertLine a={a} />
              <span aria-hidden="true" className="w-3 shrink-0" />
            </div>
          ),
        )}
      </Panel>
    </section>
  );
}

export function StepsPanel({ run, active, id }: { run: Run | undefined; active: boolean; id: string }) {
  const graph = (
    <div className="mb-3">
      <Suspense fallback={<div aria-busy="true" style={{ height: dagHeight(`/api/runs/${id}/graph`) }} className="rounded-card border border-line bg-surface"><Skeleton h="100%" /></div>}>
        <Dag runId={id} />
      </Suspense>
    </div>
  );
  return (
    <>
      {/* Mounted only while the tab shows: React Flow measures its box, and a hidden one has no size. */}
      {run?.step ? <StepLine run={run} /> : active && graph}
      <StepsTable run={run} />
      {run && <Alerts run={run} />}
    </>
  );
}

const KVSkeleton = ({ cells }: { cells: number }) => (
  <div aria-hidden="true" className="grid grid-cols-1 gap-px bg-line-soft min-[860px]:grid-cols-3">
    {Array.from({ length: cells }, (_, i) => (
      <div key={i} className="grid gap-1.5 bg-surface px-3.5 py-2">
        <Skeleton w="40%" h="0.7rem" />
        <Skeleton w="70%" h="0.8rem" />
      </div>
    ))}
  </div>
);

export function ParamsPanel({ run }: { run: Run | undefined }) {
  const { t } = useT();
  const say = useMsg();
  const value = useAutoValue();
  return (
    <>
      <section>
        <Panel title={t("run.params.auto")} count={t("run.params.auto_hint")}>
          {run ? <KV items={run.auto.map((a) => ({ key: a.key, value: value(a), note: say(a.note) }))} /> : <KVSkeleton cells={6} />}
          <p className="border-t border-line-soft px-3.5 py-2 text-[12px] text-faint">{t("run.params.auto_note")}</p>
        </Panel>
      </section>
      <section className="mt-3">
        <Panel title={t("run.params.used")}>
          {!run ? <KVSkeleton cells={3} /> : run.params.length === 0 ? <PanelEmpty>{t("run.params.none")}</PanelEmpty> : <KV items={run.params} />}
        </Panel>
      </section>
    </>
  );
}

function ContextBox({ title, aside, children }: { title: ReactNode; aside?: ReactNode; children: ReactNode }) {
  return (
    <div className="mb-2 overflow-hidden rounded-card border border-line bg-surface">
      <div className="flex items-center justify-between gap-3 border-b border-line-soft px-3.5 py-2 text-[12.5px]">
        <span className="font-mono text-ink">{title}</span>
        {aside}
      </div>
      {children}
    </div>
  );
}

export function ContextPanel({ run }: { run: Run | undefined }) {
  const { t, n } = useT();
  const say = useMsg();
  if (!run)
    return (
      <ContextBox title={<Skeleton w="7rem" h="0.8rem" />}>
        <div aria-hidden="true" className="grid gap-2 px-3.5 py-2.5">
          <Skeleton w="50%" h="0.8rem" />
          <Skeleton w="35%" h="0.8rem" />
        </div>
      </ContextBox>
    );
  if (run.context.length === 0) return <p className="rounded-card border border-line bg-surface px-3.5 py-3 text-[12.5px] text-muted">{t("run.log.empty")}</p>;
  return (
    <>
      {run.context.map((c) => (
        <ContextBox key={c.step} title={c.step} aside={c.json ? <span className="text-[11.5px] text-faint">{n("run.context.bytes", c.bytes)}</span> : undefined}>
          {c.json ? <pre className="max-h-[24rem] overflow-auto px-3.5 py-2.5 font-mono text-[12px] leading-relaxed text-ink">{c.json}</pre> : <p className="px-3.5 py-2.5 text-[12.5px] text-muted">{say(c.note)}</p>}
        </ContextBox>
      ))}
    </>
  );
}
