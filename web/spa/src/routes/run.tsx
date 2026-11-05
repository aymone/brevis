import { Component, type ReactNode, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, getJSON } from "../api/client";
import { cancelRun, type Run } from "../api/run";
import { useFallbackInterval, useLive } from "../live/hooks";
import { useFormat, useMsg, useT } from "../i18n";
import { buttonClass } from "../ui/buttons";
import { CopyIcon } from "../ui/copy";
import { Icon } from "../ui/icon";
import { LogView } from "../ui/log-view";
import { Ghost, Skeleton } from "../ui/skeleton";
import { Retry, Status } from "../ui/status";
import { Tabs } from "../ui/tabs";
import { useCrumb } from "../shell/nav";
import "../ui/bar.css";
import { Logos } from "../ui/logo";
import { usePhaseDetail } from "../ui/format";
import { useTrigger } from "../ui/runs-table";
import { ContextPanel, ParamsPanel, StepsPanel } from "./run-panels";
import { useQuickRun, useRunDrawer } from "./run-now";
import { ErrorText } from "../api/errors";

const TABS = ["logs", "steps", "params", "context"] as const;
type Tab = (typeof TABS)[number];

// The strip of the phases' bars, by how each went (web/pages phaseBar).
const BAR: Record<string, string> = { done: "bg-state-success", current: "bg-state-running animate-pulse", failed: "bg-state-failed" };

function Why({ run, onGoto }: { run: Run; onGoto: () => void }) {
  const { t, n } = useT();
  const say = useMsg();
  if (run.why) {
    return (
      <section aria-label={t("run.error.title")} className="mb-3 rounded-card border border-state-failed/40 bg-state-failed/5 px-4 py-3">
        <div className="flex items-start gap-2.5">
          <Icon name="triangle-alert" className="mt-0.5 h-4 w-4 text-state-failed" />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
              <h3 className="text-[14px] font-medium text-ink">{say(run.why.title)}</h3>
              {run.why.hasLine && (
                <button type="button" onClick={onGoto} className={buttonClass("secondary", "small")}>
                  {t("run.why.goto")}
                </button>
              )}
            </div>
            {run.why.streak >= 2 && <p className="mt-0.5 text-[12.5px] text-muted">{n("run.why.streak", run.why.streak)}</p>}
            <pre className="mt-2 overflow-x-auto font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-state-failed">{run.why.excerpt}</pre>
          </div>
        </div>
      </section>
    );
  }
  if (!run.err) return null;
  // One line that opens, where there is no cause to put in its place: a canceled run's error is who stopped it.
  return (
    <details className="mb-3 rounded-card border border-state-failed/30 bg-state-failed/5">
      <summary className="flex h-[34px] cursor-pointer list-none items-center gap-2 px-3 text-[12.5px] [&::-webkit-details-marker]:hidden">
        <Icon name="triangle-alert" className="h-3.5 w-3.5 text-state-failed" />
        <Retry attempt={run.attempt} />
        <span className="min-w-0 truncate font-mono text-[12px] text-state-failed">{run.err.split("\n")[0]}</span>
        <span className="ml-auto shrink-0 text-[11.5px] text-faint">{t("run.error.details")}</span>
      </summary>
      <pre className="overflow-auto border-t border-state-failed/20 px-3 py-2.5 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-state-failed">{run.err}</pre>
    </details>
  );
}

function Phases({ run }: { run: Run | undefined }) {
  const { t } = useT();
  const say = useMsg();
  const detail = usePhaseDetail();
  // The four steps are the run's own words and values: they wait for it, in the strip's own shape.
  const ps = run?.phases ?? [0, 1, 2, 3].map((i) => ({ key: String(i), label: { k: "" }, detail: null, state: "" }));
  return (
    <ol aria-label={t("run.timeline")} aria-busy={!run} className="mb-3 grid grid-cols-2 overflow-hidden rounded-card border border-line bg-surface @2xl:grid-cols-4">
      {ps.map((p) => (
        <li key={p.key} aria-current={p.state === "current" ? "step" : undefined} className="relative min-w-0 border-b border-line-soft px-3.5 pt-3 pb-2.5 odd:border-r @2xl:border-b-0 @2xl:not-last:border-r">
          <span aria-hidden="true" className={`absolute top-0 right-0 left-0 h-[2px] ${BAR[p.state] ?? "bg-line-strong"}`} />
          {run ? (
            <>
              <p className={p.state === "current" ? "text-[11.5px] font-medium text-ink" : "text-[11.5px] text-muted"}>{say(p.label)}</p>
              <p title={detail(p.detail)} className="mt-px truncate font-mono text-[12.5px] text-ink-2 tabular-nums">
                {detail(p.detail) || <span className="text-faint">—</span>}
              </p>
            </>
          ) : (
            // The loaded lines with ghosts in them: the strip is as tall as the real one by construction.
            <div aria-label={t("run.tabs")}>
              <p className="text-[11.5px] text-muted"><Ghost>0000000</Ghost></p>
              <p className="mt-px truncate font-mono text-[12.5px] text-ink-2"><Ghost>00000000</Ghost></p>
            </div>
          )}
        </li>
      ))}
    </ol>
  );
}

function Wait({ run }: { run: Run }) {
  const { t, n } = useT();
  const { utc } = useFormat();
  const w = run.wait;
  if (!w) return null;
  return (
    <p className="mb-3 text-xs text-muted">
      <span className="text-state-queued">{t("run.wait.for")}</span> {w.group ? `${t("run.wait.group")} ` : t("run.wait.own")}
      {w.group && <code className="font-mono text-ink">{w.group}</code>} ({t("run.wait.limit", { n: w.limit })})
      {w.holders.length > 0 && <> — {t("run.wait.held_by")} </>}
      {w.holders.map((h, i) => (
        <span key={h.runId}>
          {i > 0 && ", "}
          <a href={`/runs/${h.runId}`} className="text-ink underline decoration-line-strong underline-offset-2 hover:decoration-ink">
            {h.workflow}
          </a>{" "}
          <span className="font-mono">{t("run.wait.since", { at: utc(h.since) })}</span>
        </span>
      ))}
      {w.more > 0 && <> {t("run.wait.more", { n: w.more })}</>}
      {w.ahead > 0 && <> · {n("run.wait.ahead", w.ahead)}</>}
    </p>
  );
}

// The head: the state, the name, the verbs this state allows, how it was started, how long it took, then why it failed and the timeline.
// The labels are static and always drawn; the values and the verbs wait for the run.
function RunHead({ id, run, onGoto }: { id: string; run: Run | undefined; onGoto: () => void }) {
  const { t } = useT();
  const { utc, duration } = useFormat();
  const trigger = useTrigger();
  const qc = useQueryClient();
  const cancel = useMutation({
    mutationFn: () => cancelRun(id),
    onSettled: () => qc.invalidateQueries({ queryKey: ["run", id] }),
  });
  const canceling = !!run && (run.canceling || cancel.isPending || (cancel.isSuccess && run.inFlight));
  const wf = run ? `/workflows/${encodeURIComponent(run.workflow)}` : undefined;
  const link = "inline-flex items-center gap-1 text-[13px] text-muted transition-colors hover:text-ink";
  const rerunTitle = t("run.rerun_title");
  const quick = useQuickRun();
  const drawer = useRunDrawer();

  return (
    <>
      <div className="mb-2 flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <div className="mb-1 flex h-[18px] items-center text-sm">
            {run ? (
              <>
                <Status kind={run.status} />
                <Retry attempt={run.attempt} />
              </>
            ) : (
              <Skeleton w="4.5rem" h="0.9rem" />
            )}
          </div>
          <h2 style={{ minHeight: 28 }} className="flex min-w-0 items-center gap-2 font-mono text-[22px] leading-tight font-medium text-ink">
            {run ? (
              <>
                <Logos keys={run.logos} size={28} />
                <span className="break-all">{run.workflow}</span>
              </>
            ) : (
              <Skeleton w="14rem" h="1.6rem" />
            )}
          </h2>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          {wf ? (
            <a href={wf} className={link}>
              {t("run.view_workflow")}
              <span aria-hidden="true">→</span>
            </a>
          ) : (
            <span className={link}>
              {t("run.view_workflow")}
              <span aria-hidden="true">→</span>
            </span>
          )}
          {!run ? (
            <Skeleton w="5.5rem" h="2rem" />
          ) : run.inFlight ? (
            canceling ? (
              <span role="status" className="ms-auto text-xs text-muted">
                {t("run.canceling")}
              </span>
            ) : (
              <button type="button" className={`ms-auto ${buttonClass("danger", "regular")}`} onClick={() => window.confirm(t("run.cancel_confirm")) && cancel.mutate()}>
                <Icon name="square" className="h-3 w-3" />
                {t("run.cancel")}
              </button>
            )
          ) : run.rerun ? (
            // With no parameters to ask for, one click; with some, the drawer, filled with this run's values.
            <button
              type="button"
              disabled={quick.pending !== null}
              aria-haspopup={run.rerun === "params" ? "dialog" : undefined}
              title={run.slot ? t("run.rerun_note", { slot: utc(run.slot) }) : rerunTitle}
              className={buttonClass()}
              onClick={(e) =>
                run.rerun === "now"
                  ? void quick.run(run.workflow, {}, run.id)
                  : drawer.open({ slug: run.workflow, rerun: { runId: run.id, params: Object.fromEntries(run.params.map((kv) => [kv.key, kv.value])), slot: run.slot } }, e.currentTarget)
              }
            >
              <Icon name="refresh-cw" className="h-3.5 w-3.5" />
              {t(run.rerun === "now" ? "run.rerun" : "run.rerun_more")}
            </button>
          ) : null}
        </div>
      </div>
      {cancel.isError && (
        <p role="alert" className="mb-2 text-[12.5px] text-state-failed">
          <ErrorText error={cancel.error} />
        </p>
      )}
      {drawer.drawer}

      <dl className="mb-3 flex flex-wrap items-center gap-x-5 gap-y-1 text-[13px]">
        <div className="flex items-baseline gap-1.5">
          <dt className="text-muted">{t("run.trigger")}</dt>
          <dd className="text-ink">{run ? trigger(run.trigger) : <Ghost>000000</Ghost>}</dd>
        </div>
        {run?.slot && (
          <div className="flex items-baseline gap-1.5">
            <dt className="text-muted">{t("run.slot")}</dt>
            <dd className="font-mono text-[12.5px] text-ink-2">{utc(run.slot)}</dd>
          </div>
        )}
        <div className="flex items-baseline gap-1.5">
          <dt className="text-muted">{t("run.duration")}</dt>
          <dd className="text-ink">{!run ? <Ghost>00s</Ghost> : run.inFlight ? t("run.in_progress") : run.durationMs !== null ? duration(run.durationMs) : "—"}</dd>
        </div>
        <div className="flex items-center gap-1">
          <dt className="sr-only">ID</dt>
          {/* The id is the address: known before the run arrives. */}
          <dd title={id} className="font-mono text-[12px] text-muted">
            {id.slice(0, 8)}
          </dd>
          <CopyIcon value={id} label={t("run.copy_id")} />
        </div>
      </dl>

      {run && <Why run={run} onGoto={onGoto} />}
      <Phases run={run} />
      {run && <Wait run={run} />}
    </>
  );
}

function RunScreen({ id }: { id: string }) {
  const { t } = useT();
  const navigate = useNavigate();
  const { hash } = useLocation();
  const [goto, setGoto] = useState(0);
  // The server reports `run:<id>` whenever the run (or its log) moves, so a
  // running run needs no cadence; the fallback polls only while the stream is
  // down and the run is not terminal. A missing run or a failed read throws to
  // the boundary; until then the page is drawn, its values waiting.
  useLive([`run:${id}`], [["run", id]]);
  const fallback = useFallbackInterval();
  const { data: run } = useQuery({
    queryKey: ["run", id],
    queryFn: () => getJSON<Run>(`/api/runs/${id}`),
    refetchInterval: (q) => (q.state.data?.terminal ? false : fallback),
    staleTime: 0,
    throwOnError: true,
    retry: (n, e) => !(e instanceof ApiError && e.status === 404) && n < 1,
  });

  // As the old page: "<workflow> · <first 8 of the id>".
  useCrumb(run ? `${run.workflow} · ${id.slice(0, 8)}` : "", { mono: true });
  const wanted = hash.slice(1);
  const tab: Tab = (TABS as readonly string[]).includes(wanted) ? (wanted as Tab) : "logs";
  const pick = (k: string) => void navigate({ hash: `#${k}` }, { replace: true });
  const count = (n: number | undefined) => (n === undefined ? <Ghost>0</Ghost> : n);
  const panel = (k: Tab, body: ReactNode) => (
    <div role="tabpanel" id={`run-tab-panel-${k}`} aria-labelledby={`run-tab-${k}`} hidden={tab !== k} className="mb-3">
      {body}
    </div>
  );

  return (
    <div className="@container">
      <RunHead
        id={id}
        run={run}
        onGoto={() => {
          pick("logs");
          setGoto((g) => g + 1);
        }}
      />
      <p role="status" className="mb-3 flex items-center gap-1.5 text-[0.7rem] text-muted">
        <span className={`h-1.5 w-1.5 rounded-full ${run?.terminal ? "bg-state-pending" : "bg-state-running"}`} />
        {run ? (run.terminal ? t("run.live.finished") : t("run.live.following")) : <Ghost>00000000000</Ghost>}
      </p>
      <div className="mb-3">
        <Tabs
          label={t("run.tabs")}
          idPrefix="run-tab"
          active={tab}
          onChange={pick}
          tabs={[
            { key: "logs", label: t("run.tab.logs"), count: count(run?.log.lines) },
            { key: "steps", label: t("run.tab.steps"), count: count(run?.steps.length) },
            { key: "params", label: t("run.tab.params"), count: count(run ? run.auto.length + run.params.length : undefined) },
            { key: "context", label: t("run.tab.context") },
          ]}
        />
      </div>
      {/* Panels stay mounted (a search in the log survives a tab change); the graph mounts only when shown. */}
      {panel("logs", <LogView log={run?.log} live={!run?.terminal} downloadHref={`/runs/${id}/log.txt`} gotoCause={goto} />)}
      {panel("steps", <StepsPanel run={run} active={tab === "steps"} id={id} />)}
      {panel("params", <ParamsPanel run={run} />)}
      {panel("context", <ContextPanel run={run} />)}
    </div>
  );
}

// A missing run or a failed read: the message, not a blank page.
class Boundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };
  static getDerivedStateFromError(error: Error) {
    return { error };
  }
  render() {
    const e = this.state.error;
    if (!e) return this.props.children;
    return (
      <section role="alert" className="rounded-card border border-line bg-surface p-4">
        <RunError missing={e instanceof ApiError && e.status === 404} />
      </section>
    );
  }
}

// Apart from the class above because a class cannot call the hook.
function RunError({ missing }: { missing: boolean }) {
  const { t } = useT();
  return (
    <>
      <p className="m-0 text-sm text-ink">{t(missing ? "run.error.not_found" : "run.error.failed")}</p>
      <a href="/runs" className="mt-1 inline-block text-[13px] text-muted underline hover:text-ink">
        {t("nav.runs")}
      </a>
    </>
  );
}

export default function RunRoute() {
  const { id = "" } = useParams();
  return (
    <Boundary key={id}>
      <RunScreen id={id} />
    </Boundary>
  );
}
