import { Suspense, lazy, useEffect, useRef, useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, getJSON } from "../api/client";
import { useFallbackInterval, useLive } from "../live/hooks";
import { useScheduleToggle, type WorkflowDetail, type WorkflowsList } from "../api/workflows";
import { useFormat, useMsg, useT } from "../i18n";
import { useCrumb } from "../shell/nav";
import { buttonClass } from "../ui/button";
import { linkClass } from "../ui/card";
import { useUntil, useAgo, useBrief, useNumbers } from "../ui/format";
import { Segmented } from "../ui/filters";
import { Icon } from "../ui/icon";
import { Logo, useLogoTitle } from "../ui/logo";
import { useTrend } from "../ui/trend";
import { PageFrame } from "../ui/page-frame";
import { Panel } from "../ui/panel";
import { useRunDrawer } from "./run-now";
import { LoadError } from "../ui/query-boundary";
import { Ghost, SK_REAL, Skeleton } from "../ui/skeleton";
import { dagHeight } from "../ui/dag/height";
import { Stats, type StatProps } from "../ui/stat";
import { Status, runKind } from "../ui/status";
import { TabBar } from "../ui/tab-bar";
import { YearOfRuns } from "./workflow-calendar";
import { ErrorText } from "../api/errors";

// The graph's chunk (React Flow) loads only when the graph is drawn.
const Dag = lazy(() => import("../ui/dag/dag"));

const th = (extra = "") => `px-2.5 text-[11.5px] font-medium whitespace-nowrap text-faint ${extra}`;
const cell = "px-2.5 py-2.5";
const card = "rounded-card border border-line bg-surface shadow-warm-sm";

// A line of text in the place a data-bound line will take.
const Line = ({ w, h = "14px" }: { w: string; h?: string }) => <Skeleton w={w} h={h} />;
const Rows = ({ cols, rows = 4 }: { cols: string[]; rows?: number }) => (
  <>
    {Array.from({ length: rows }, (_, i) => (
      <tr key={i} aria-hidden="true">
        {cols.map((w, j) => (
          <td key={j} className={`${cell} ${j === 0 ? "pl-4" : ""} ${j === cols.length - 1 ? "pr-4" : ""}`}><Line w={w} h="13px" /></td>
        ))}
      </tr>
    ))}
  </>
);

// The classes the app uses are the server sheet's: one it does not hold does nothing, so a one-off size is a style.
const META_MIN = { minHeight: 23 };

// What the line holds for a typical workflow, drawn hidden while the real one loads: it wraps the way the loaded
// line does at this width. Only the fields Meta reads are filled in.
const META_PLACEHOLDER = {
  hasSchedule: true, active: true, project: "project", cron: "0 0 * * *", schedule: { k: "cron.daily", a: { at: "00:00" } }, timezone: "UTC",
  steps: [0, 1], edges: 1, langs: [{ key: "go" }], tags: ["example", "tag"],
} as unknown as WorkflowDetail;

// ---- the line under the title: whether the schedule runs, project, cron, size, tags ----

function Meta({ d }: { d?: WorkflowDetail }) {
  const { t, n } = useT();
  const say = useMsg();
  const logoTitle = useLogoTitle();
  // The line is as tall as one that carries a tag chip or a language logo (the usual case), with or without them:
  // a workflow with neither would otherwise sit 4px higher than the skeleton and shift everything below.
  if (!d) return <div aria-hidden="true" className={`mb-3 ${SK_REAL}`}><Meta d={META_PLACEHOLDER} /></div>;
  const zone = d.timezone && d.timezone !== "UTC" && d.timezone !== "Etc/UTC" ? ` (${d.timezone})` : "";
  return (
    <p data-workflow-meta style={META_MIN} className="mb-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-[12.5px] text-muted">
      {d.hasSchedule && (
        <>
          {d.active ? (
            <span className="inline-flex items-center gap-1.5 text-state-success"><span className="h-[7px] w-[7px] rounded-full bg-state-success" />{t("workflow.active")}</span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-muted"><span className="h-[7px] w-[7px] rounded-full bg-state-pending" />{t("workflow.paused")}</span>
          )}
          <span className="text-faint">·</span>
        </>
      )}
      {d.project && (
        <>
          <span>{t("workflows.chip.project")} <Link to={`/workflows?project=${encodeURIComponent(d.project)}`} className="font-mono text-ink-2 hover:underline">{d.project}</Link></span>
          <span className="text-faint">·</span>
        </>
      )}
      {d.cron === "" ? (
        <span>{t("workflows.manual_only")}</span>
      ) : (
        <>
          {d.schedule && <span className="text-ink">{say(d.schedule)}{zone}</span>}
          <span className="font-mono text-[12px] text-faint">{d.cron}</span>
        </>
      )}
      <span className="text-faint">·</span>
      <span>{n("workflows.steps", d.steps.length)} · {n("workflow.dependencies", d.edges)}</span>
      {d.langs.length > 0 && (
        <>
          <span className="text-faint">·</span>
          {d.langs.map((l) => (
            <span key={l.key} className="inline-flex items-center gap-1 text-ink-2"><Logo k={l.key} />{logoTitle(l.key)}</span>
          ))}
        </>
      )}
      {d.tags.map((tag) => (
        <Link key={tag} to={`/workflows?tag=${encodeURIComponent(tag)}`} className="inline-flex rounded-card border border-line bg-parchment px-2 py-0.5 text-[0.7rem] text-muted transition-colors hover:border-line-strong hover:text-ink">{tag}</Link>
      ))}
    </p>
  );
}

// ---- the five numbers: days without failure, days with, mean, in progress, next run ----

function WorkflowStats({ d }: { d?: WorkflowDetail }) {
  const until = useUntil();
  const { t, n } = useT();
  const { number } = useFormat();
  const { percent, elapsed } = useNumbers();
  const brief = useBrief();
  const last30 = t("workflow.kpi.last_days", { days: number(30) });
  const labels = {
    clean: { label: t("workflow.kpi.clean"), dot: "var(--st-success)" },
    failed: { label: t("workflow.kpi.failed_days"), dot: "var(--st-failed)" },
    mean: { label: t("workflow.kpi.mean") },
    open: { label: t("workflow.kpi.open") },
    next: { label: t("workflow.kpi.next") },
  };
  // The loading strip is the loaded one with ghosts for the numbers: same cells, same lines, the clean-days bar's row included.
  // The "last 30 days" notes are static words and stay as words.
  if (!d) {
    const ghost = (note?: string) => ({ value: <Ghost>000</Ghost>, note: note ?? <Ghost>00000000000</Ghost> });
    return (
      <Stats
        items={[
          { ...labels.clean, ...ghost(), bar: 0 },
          { ...labels.failed, ...ghost(last30) },
          { ...labels.mean, ...ghost(last30) },
          { ...labels.open, ...ghost() },
          { ...labels.next, ...ghost() },
        ]}
      />
    );
  }
  const { ranDays, failedDays, meanMs, running, queued } = d.stats;
  const clean: StatProps = { ...labels.clean, value: "—", note: last30 };
  if (ranDays > 0) {
    clean.value = percent(ranDays - failedDays, ranDays);
    clean.note = n("workflow.kpi.of_days", ranDays, { n: number(ranDays - failedDays) });
    clean.bar = (ranDays - failedDays) / ranDays;
  }
  const open: StatProps = { ...labels.open, value: number(running + queued), note: running + queued > 0 ? t("workflow.kpi.open_note", { running: number(running), queued: number(queued) }) : t("workflow.kpi.nothing_running") };
  const next: StatProps = { ...labels.next, value: "—" };
  if (!d.hasSchedule) next.value = t("workflows.manual_only");
  else if (!d.active) next.value = t("workflows.paused");
  else if (d.nextRun) {
    next.value = t("time.in", { span: until(new Date(d.nextRun).getTime() - new Date(d.now).getTime()) });
    next.note = `${brief(d.nextRun)} UTC`;
  }
  return (
    <Stats
      items={[
        clean,
        { ...labels.failed, value: number(failedDays), note: last30 },
        { ...labels.mean, value: meanMs > 0 ? elapsed(meanMs) : "—", note: last30 },
        open,
        next,
      ]}
    />
  );
}

// ---- the steps: the graph, or the list ----

function StepsList({ d, ghostRows = 4 }: { d?: WorkflowDetail; ghostRows?: number }) {
  const { t } = useT();
  const say = useMsg();
  const logoTitle = useLogoTitle();
  const { elapsed } = useNumbers();
  return (
    <div className="overflow-x-auto">
      <table className="ct w-full text-sm">
        <thead>
          <tr className="border-b border-line bg-parchment-soft/60 text-left">
            <th className={th("w-10 pl-4")}>#</th>
            <th className={th()}>{t("graph.list.step")}</th>
            <th className={th()}>{t("workflows.col.lang")}</th>
            <th className={th()}>{t("graph.list.uses")}</th>
            <th className={th()}>{t("graph.list.needs")}</th>
            <th className={th()}>{t("graph.list.command")}</th>
            <th className={th("pr-4 text-right")}>{t("graph.list.last")}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line-soft">
          {!d ? (
            <Rows cols={["1.2rem", "6rem", "5rem", "3rem", "5rem", "14rem", "3rem"]} rows={ghostRows} />
          ) : (
            d.steps.map((s, i) => (
              <tr key={s.id} className="align-top transition-colors hover:bg-hover">
                <td className={`${cell} pl-4 text-xs text-faint tabular-nums`}>{String(i + 1).padStart(2, "0")}</td>
                <td className={`${cell} font-mono text-[12.5px] text-ink`}>{s.id}</td>
                <td className={cell}>
                  <span className="inline-flex items-center gap-1.5 text-xs text-ink-2">
                    {s.langs.map((l) => <Logo key={l.key} k={l.key} />)}
                    {s.langs[0] && logoTitle(s.langs[0].key)}
                  </span>
                </td>
                <td className={cell}><span className="inline-flex items-center gap-1">{s.uses.map((l) => <Logo key={l.key} k={l.key} />)}</span></td>
                <td className={`${cell} font-mono text-xs text-muted`}>{s.needs.join(", ")}</td>
                <td className={`${cell} max-w-[22rem] truncate font-mono text-xs text-muted`} title={say(s.command)}>{say(s.command)}</td>
                <td className={`${cell} pr-4 text-right text-xs text-muted tabular-nums`}>{s.lastStatus ? elapsed(s.lastMs) : "—"}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

// How many steps a workflow has, from what the app already read BEFORE this page's
// own answer: the Workflows list rows carry it. Memory only (react-query's cache),
// never storage: it is API text. Null when the list was not read this visit (a
// page opened straight from its address), and then the card opens as a drawn graph.
function useStepsHint(slug: string): number | null {
  const qc = useQueryClient();
  for (const [, list] of qc.getQueriesData<WorkflowsList>({ queryKey: ["workflows"] })) {
    const row = list?.rows?.find((r) => r.slug === slug);
    if (row?.steps != null) return row.steps;
  }
  return null;
}

function StepsCard({ d, slug }: { d?: WorkflowDetail; slug: string }) {
  const { t, n } = useT();
  const hint = useStepsHint(slug);
  const [view, setView] = useState<"graph" | "list">("graph");
  const [picked, setPicked] = useState<string | null>(null);
  // A one-step workflow has nothing to draw: its list is its line. Before the answer
  // the list rows' step count says which card to hold the place of, so opening a
  // one-step workflow from the list shifts nothing (a graph box would collapse).
  const oneStep = d ? d.steps.length < 2 : hint !== null && hint < 2;
  const shown = oneStep ? "list" : view;
  return (
    <section className={card}>
      <header className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-line-soft px-4 py-2.5">
        <h2 className="text-[13px] font-semibold text-ink">
          {t("graph.title")}{" "}
          <span className="ml-1 font-normal text-faint tabular-nums">
            {d ? <>{n("workflows.steps", d.steps.length)} · {n("graph.stages", d.stages)}</> : <Ghost>00 steps · 00 stages</Ghost>}
          </span>
        </h2>
        <p className="text-[12.5px] text-muted max-[860px]:hidden">{t("graph.hint")}</p>
        {!oneStep && (
          <div className="ml-auto">
            {/* A VIEW toggle (how the same steps are drawn), not a filter: icon + word, same segmented family. */}
            <Segmented
              label={t("graph.view")}
              value={view}
              onChange={(v) => setView(v as "graph" | "list")}
              items={[
                { key: "graph", label: t("graph.view.graph"), icon: <Icon name="workflow" className="h-3.5 w-3.5" /> },
                { key: "list", label: t("graph.view.list"), icon: <Icon name="table" className="h-3.5 w-3.5" /> },
              ]}
            />
          </div>
        )}
      </header>
      {shown === "graph" ? (
        <div className="p-3">
          <Suspense fallback={<Skeleton h={dagHeight(`/api/workflows/${encodeURIComponent(slug)}/graph`)} />}>
            <Dag src={`/api/workflows/${encodeURIComponent(slug)}/graph`} onPick={setPicked} />
          </Suspense>
        </div>
      ) : (
        <StepsList d={d} ghostRows={hint ?? 4} />
      )}
      {shown === "graph" && (
        <div className="border-t border-line-soft px-4 py-3 text-[12.5px] text-muted">
          {picked ? <span className="font-mono text-ink">{picked}</span> : t("graph.panel.none")}
        </div>
      )}
    </section>
  );
}

// ---- the tabs ----

function RunsTab({ d }: { d?: WorkflowDetail }) {
  const { t } = useT();
  const ago = useAgo();
  const brief = useBrief();
  const { elapsed } = useNumbers();
  const { date } = useFormat();
  const slug = useParams().slug ?? "";
  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-2.5 text-[12.5px] text-muted">
        {d?.day ? (
          <span data-day-filter className="inline-flex items-center gap-1.5 rounded-card border border-line-strong bg-surface-2 px-2.5 py-0.5 text-ink">
            {date(d.day)}
            <Link to="#runs" aria-label={t("workflow.runs.clear_day")} className="text-muted hover:text-ink">×</Link>
          </span>
        ) : (
          <span>{t("workflow.runs.lead")}</span>
        )}
        <Link to={`/runs?workflow=${encodeURIComponent(slug)}`} className="text-ink hover:underline">{t("workflow.runs.all")} →</Link>
      </div>
      {d && d.runs.length === 0 ? (
        <p className="border-t border-line-soft px-4 py-6 text-center text-[12.5px] text-muted">{t("workflow.runs.none")}</p>
      ) : (
        <div className="overflow-x-auto border-t border-line-soft">
          <table className="ct w-full text-sm">
            <thead>
              <tr className="border-b border-line bg-parchment-soft/60 text-left">
                <th className={th("pl-4")}>{t("runs.col.state")}</th>
                <th className={th()}>{t("runs.col.trigger")}</th>
                <th className={th()}>{t("runs.col.slot")}</th>
                <th className={th()}>{t("workflow.runs.params")}</th>
                <th className={th()}>{t("runs.col.duration")}</th>
                <th className={th("pr-4 text-right")}>{t("workflow.runs.when")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {!d ? (
                <Rows cols={["4.5rem", "4rem", "5rem", "5rem", "3rem", "4rem"]} rows={10} />
              ) : (
                d.runs.map((r) => (
                  <tr key={r.id} className="transition-colors hover:bg-hover">
                    <td className={`${cell} pl-4`}>
                      <Link to={`/runs/${r.id}`} className="hover:underline"><Status kind={runKind(r.status)} /></Link>
                    </td>
                    <td className={`${cell} text-xs text-muted`}>{r.slot ? t("workflow.runs.manual_slot") : t(`trigger.${r.trigger}`)}</td>
                    <td className={`${cell} font-mono text-xs text-ink-2 tabular-nums`}>{r.logicalDate ? <span className="whitespace-nowrap">{brief(r.logicalDate)}</span> : <span className="text-faint">—</span>}</td>
                    <td className={`${cell} font-mono text-xs text-ink-2`}>{r.given.length > 0 ? r.given.join(" ") : <span className="font-sans text-muted">{t("workflow.runs.default")}</span>}</td>
                    <td className={`${cell} text-xs text-muted tabular-nums`}>{r.durationMs === null ? "—" : elapsed(r.durationMs)}</td>
                    <td className={`${cell} pr-4 text-right text-xs text-muted`}>{ago(r.createdAt, new Date(d.now).getTime())}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function ParamsTab({ d }: { d?: WorkflowDetail }) {
  const { t } = useT();
  if (d && d.params.length === 0)
    return <p className="px-4 py-6 text-center text-[12.5px] text-muted">{t("workflow.params.none")} <code className="font-mono text-ink-2">$BREVIS_AUTO_*</code>.</p>;
  return (
    <div className="overflow-x-auto">
      <table className="ct w-full text-sm">
        <thead>
          <tr className="border-b border-line bg-parchment-soft/60 text-left">
            <th className={th("pl-4")}>{t("workflow.params.name")}</th>
            <th className={th()}>{t("workflow.params.type")}</th>
            <th className={th()}>{t("workflow.params.default")}</th>
            <th className={th("pr-4")}>{t("workflow.params.what")}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line-soft">
          {!d ? (
            <Rows cols={["7rem", "4rem", "5rem", "16rem"]} rows={3} />
          ) : (
            d.params.map((p) => (
              <tr key={p.name}>
                <td className={`${cell} pl-4 align-top`}>
                  <span className="inline-flex items-center gap-1.5 font-mono text-[12.5px] text-ink">{p.name}{p.uses && <Logo k={p.uses} />}</span>
                  {p.group && <p className="mt-0.5 text-[11.5px] text-faint">{p.group}</p>}
                </td>
                <td className={`${cell} align-top font-mono text-xs text-muted`}>{p.type}</td>
                <td className={`${cell} align-top font-mono text-xs text-ink-2`}>{p.default || <span className="font-sans text-muted">{t("workflow.params.required")}</span>}</td>
                <td className={`${cell} pr-4 align-top text-[12.5px] text-ink-2`}>
                  {p.title && <strong className="font-medium text-ink">{p.title}. </strong>}
                  {p.description}
                  {p.warn && <p className="mt-0.5 text-[11.5px] text-warn">{p.warn}</p>}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function DefinitionTab({ d }: { d?: WorkflowDetail }) {
  const { t } = useT();
  const [copied, setCopied] = useState(false);
  return (
    <>
      <p className="px-4 py-2.5 text-[12.5px] text-muted">{t("workflow.definition.note")}</p>
      <div className="px-4 pb-4">
        {!d ? (
          <Skeleton h="12rem" />
        ) : d.yaml ? (
          <div className="relative">
            <pre tabIndex={0} className="overflow-x-auto rounded-card border border-line bg-canvas-subtle p-3 pr-11 font-mono text-[0.75rem] leading-relaxed text-ink"><code>{d.yaml}</code></pre>
            <span className="absolute top-1.5 right-1.5">
              <button
                type="button"
                aria-label={t("copy.label")}
                title={t("copy.label")}
                onClick={() => void navigator.clipboard?.writeText(d.yaml).then(() => { setCopied(true); setTimeout(() => setCopied(false), 1500); })}
                className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-card text-muted transition-colors duration-150 hover:bg-surface-2 hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong"
              >
                <Icon name={copied ? "check" : "copy"} className="h-4 w-4" />
              </button>
            </span>
          </div>
        ) : (
          <p className="text-[12.5px] text-muted">{t("workflow.definition.none")}</p>
        )}
      </div>
    </>
  );
}

function Tabs({ d }: { d?: WorkflowDetail }) {
  const { t } = useT();
  const { hash } = useLocation();
  const navigate = useNavigate();
  const current = ["params", "definition"].includes(hash.slice(1)) ? hash.slice(1) : "runs";
  const count = (n?: number) => (d ? n : <Ghost>00</Ghost>);
  return (
    <div className="mt-3">
      <div className="mb-3">
        <TabBar
          label={t("workflow.tabs")}
          current={current}
          onSelect={(key) => void navigate({ search: location.search, hash: `#${key}` }, { replace: true })}
          tabs={[
            { key: "runs", label: t("workflow.tab.runs"), count: count(d?.runs.length) },
            { key: "params", label: t("workflow.tab.params"), count: count(d?.params.length) },
            { key: "definition", label: t("workflow.tab.definition") },
          ]}
        />
      </div>
      <section id={`panel-${current}`} role="tabpanel" aria-labelledby={`tab-${current}`} className={card}>
        {current === "runs" ? <RunsTab d={d} /> : current === "params" ? <ParamsTab d={d} /> : <DefinitionTab d={d} />}
      </section>
    </div>
  );
}

// ---- the page's own header: the question mark, Pause or Resume, and Run ----

function About({ slug, text }: { slug: string; text: string }) {
  const { t } = useT();
  const dialog = useRef<HTMLDialogElement>(null);
  return (
    <>
      <button
        type="button"
        title={t("workflow.purpose.title")}
        aria-label={t("workflow.purpose.title")}
        onClick={() => dialog.current?.showModal()}
        className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-line text-[0.7rem] font-semibold text-muted transition-colors hover:border-line-strong hover:text-ink focus-visible:text-ink"
      >
        ?
      </button>
      <dialog ref={dialog} className="w-[min(680px,92vw)] rounded-card border border-line bg-surface p-0 text-ink shadow-warm backdrop:bg-ink/40 backdrop:backdrop-blur-[2px]">
        <div className="flex items-start justify-between gap-4 border-b border-line px-6 py-4">
          <div className="min-w-0">
            <p className="text-[11.5px] text-muted">{t("workflow.purpose.label")}</p>
            <p className="display mt-1 truncate text-2xl">{slug}</p>
          </div>
          <form method="dialog">
            <button autoFocus aria-label={t("dialog.close")} className="rounded-card border border-line px-3 py-1 text-xs text-muted transition-colors hover:border-line-strong hover:text-ink">{t("dialog.close_word")}</button>
          </form>
        </div>
        <p className="px-6 py-5 text-[13.5px] leading-relaxed whitespace-pre-wrap text-ink-2">{text}</p>
      </dialog>
    </>
  );
}

function Actions({ slug, d, toggler, onRun }: { slug: string; d?: WorkflowDetail; toggler: ReturnType<typeof useScheduleToggle>; onRun: (opener: HTMLElement) => void }) {
  const { t } = useT();
  return (
    <div className="flex items-center gap-2">
      {!d ? (
        <Skeleton w="5rem" h="2rem" />
      ) : (
        d.hasSchedule && (
          <button type="button" className={buttonClass("secondary", "regular")} disabled={toggler.pending === slug} onClick={() => toggler.toggle(slug)}>
            {t(d.active ? "workflow.pause" : "workflow.resume")}
          </button>
        )
      )}
      <button type="button" aria-haspopup="dialog" className={buttonClass("primary", "regular")} onClick={(e) => onRun(e.currentTarget)}>
        <Icon name="play" className="h-3.5 w-3.5" />
        {t("workflows.run_now.open")}
      </button>
    </div>
  );
}

// ---- what it loads: four lines over the last 90 days (web/components LoadTrend) ----

function WhatItLoads({ load }: { load: NonNullable<WorkflowDetail["load"]> }) {
  const { t } = useT();
  const trend = useTrend();
  const window = trend.window(load.from, load.to);
  return (
    <section className="mt-3">
      <Panel title={t("workflow.load.title")}>
        <div className="px-3.5 py-3">
          <p className="mb-3 text-[12.5px] text-muted">
            {window}. {t("workflow.load.lead")}
          </p>
          <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
            {load.series.map((s) => (
              <div key={s.key}>
                <div className="flex items-baseline justify-between gap-2">
                  <p className="text-[11.5px] text-muted">{trend.label(s.key)}</p>
                  {s.delta && <span className={`text-[0.7rem] font-medium ${s.hue}`}>{trend.delta(s.delta, s.pct)}</span>}
                </div>
                <p className="mt-1 font-serif text-2xl text-ink tabular-nums">{trend.value(s.unit, s.latest)}</p>
                {s.flat ? (
                  <p className="mt-1 text-xs text-muted">{t("workflow.trend.flat")}</p>
                ) : (
                  <svg viewBox="0 0 300 56" className="mt-2 h-[56px] w-full" preserveAspectRatio="none" role="img" aria-label={t("workflow.trend.aria", { label: trend.label(s.key), window })}>
                    <polyline points={s.area} className="fill-ink/10" stroke="none" />
                    <polyline points={s.path} className="stroke-ink-2" fill="none" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
                    {s.points.map((p) => (
                      <circle key={p.x} cx={p.x} cy={p.y} r="4" fill="transparent">
                        <title>{`${trend.day(p.date)} — ${trend.value(s.unit, p.value)}`}</title>
                      </circle>
                    ))}
                  </svg>
                )}
                <p className="mt-1 text-xs text-muted">{trend.note(s.key)}</p>
              </div>
            ))}
          </div>
        </div>
      </Panel>
    </section>
  );
}

export default function WorkflowRoute() {
  const { t } = useT();
  const { slug = "" } = useParams();
  useCrumb(slug);
  const [sp] = useSearchParams();
  const day = sp.get("day") ?? "";
  const toggler = useScheduleToggle();
  const runDrawer = useRunDrawer();
  useLive([`workflow:${slug}`], [["workflow", slug]]);
  const q = useQuery({
    queryKey: ["workflow", slug, day],
    queryFn: () => getJSON<WorkflowDetail>(`/api/workflows/${encodeURIComponent(slug)}${day ? `?day=${encodeURIComponent(day)}` : ""}`),
    placeholderData: keepPreviousData,
    refetchInterval: useFallbackInterval(),
    retry: (count, e) => !(e instanceof ApiError && e.status === 404) && count < 1,
  });
  const d = q.data;
  // The old page's `#run` opened the drawer; links that still say so (a load's "Run…") keep working.
  const wantsRun = useLocation().hash === "#run" && !!d;
  useEffect(() => {
    if (wantsRun) runDrawer.open({ slug }, document.body);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wantsRun]);
  const { note, error, dismiss, toggle } = toggler;
  const missing = q.error instanceof ApiError && q.error.status === 404;
  let body: ReactNode;
  if (q.isError && !d) {
    body = missing ? (
      <p className="rounded-card border border-dashed border-line px-6 py-12 text-center text-sm text-muted">
        {`No workflow “${slug}”.`} <Link to="/workflows" className={linkClass}>{t("nav.workflows")}</Link>
      </p>
    ) : (
      <LoadError onRetry={() => void q.refetch()} />
    );
  } else {
    body = (
      <div aria-busy={q.isPlaceholderData || !d} className={q.isPlaceholderData ? "opacity-60 transition-opacity" : undefined}>
        <Meta d={d} />
        <div role="status">
          {note && (
            <p className="mb-2 text-[12.5px] text-muted">
              {t(note.active ? "workflows.resumed.toast" : "workflows.paused.toast", { workflow: note.slug })}{" "}
              <button type="button" className={linkClass} onClick={() => (toggle(note.slug), dismiss())}>{t("workflows.undo")}</button>
            </p>
          )}
          {error && <p className="mb-2 text-[12.5px] text-state-failed"><ErrorText error={error} /></p>}
        </div>
        <div className="mb-3">
          <WorkflowStats d={d} />
        </div>
        <StepsCard d={d} slug={slug} />
        <div className="mt-3">
          <YearOfRuns d={d} />
        </div>
        <Tabs d={d} />
        {d?.load && <WhatItLoads load={d.load} />}
      </div>
    );
  }
  return (
    <PageFrame title={slug} mono actions={q.isError && !d ? undefined : <Actions slug={slug} d={d} toggler={toggler} onRun={(el) => runDrawer.open({ slug }, el)} />} aside={d?.description ? <About slug={slug} text={d.description} /> : undefined}>
      {body}
      {runDrawer.drawer}
    </PageFrame>
  );
}
