import { useEffect, useState, type MouseEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { getJSON } from "../api/client";
import { useFallbackInterval, useLive } from "../live/hooks";
import { useScheduleToggle, type Logo as LogoRef, type WorkflowRow, type WorkflowsList } from "../api/workflows";
import { useFormat, useMsg, useT } from "../i18n";
import { useAgo } from "../ui/format";
import { buttonClass } from "../ui/button";
import { FilterBar, SearchField, Segmented, ResultCount, useUrlSearch, type Chip, type FilterDef } from "../ui/filters";
import { linkClass } from "../ui/card";
import { CodeBlock } from "../ui/copy";
import { Drawer, useDrawer } from "../ui/drawer";
import { Icon } from "../ui/icon";
import { Logo, useLogoTitle } from "../ui/logo";
import { PageFrame } from "../ui/page-frame";
import { Pagination } from "../ui/pagination";
import { recalled, remember } from "../ui/remembered";
import { LoadError } from "../ui/query-boundary";
import { useRunDrawer } from "./run-now";
import { ToggleSwitch } from "../ui/toggle-switch";
import { ErrorText } from "../api/errors";

const STATES = ["", "failing", "running", "paused", "manual"] as const;
const SORTABLE = [
  ["workflow", "workflows.col.workflow", "max-[640px]:pl-4"],
  ["schedule", "workflows.col.schedule", ""],
  ["next", "workflows.col.next", "max-[640px]:hidden"],
  ["last", "workflows.col.last", "max-[860px]:hidden"],
] as const;
type Patch = Record<string, string | string[] | null>;

// The filter, the sort and the page live in the query string, with the old
// page's names, so a link is the same link. Every control is a real link.
function useListLinks() {
  const [sp] = useSearchParams();
  const hrefWith = (patch: Patch, keepPage = false): string => {
    const q = new URLSearchParams(sp);
    if (!keepPage) q.delete("page");
    for (const [k, v] of Object.entries(patch)) {
      q.delete(k);
      for (const one of Array.isArray(v) ? v : v ? [v] : []) q.append(k, one);
    }
    const s = q.toString();
    return s ? `?${s}` : "?";
  };
  return { sp, hrefWith };
}
type Links = ReturnType<typeof useListLinks>;

function toggled(tags: string[], tag: string): string[] {
  return tags.includes(tag) ? tags.filter((x) => x !== tag) : [...tags, tag];
}

const zoneSuffix = (tz: string) => (tz === "" || tz === "UTC" || tz === "Etc/UTC" ? "" : ` (${tz})`);

// The column header, drawn from the address alone: its sort arrow needs no data.
const th = (extra: string) => `px-2.5 text-[11.5px] font-medium whitespace-nowrap text-faint ${extra}`;

function SortTh({ label, field, extra, links }: { label: string; field: string; extra: string; links: Links }) {
  const sort = links.sp.get("sort") ?? "";
  const dir = links.sp.get("dir") === "desc" ? "desc" : "asc";
  // Asc, then desc, then the natural order: the old page's three clicks.
  const patch: Patch = sort !== field ? { sort: field, dir: null } : dir === "asc" ? { dir: "desc" } : { sort: null, dir: null };
  return (
    <th className={`px-2.5 text-[11.5px] font-medium whitespace-nowrap ${extra} ${sort === field ? "text-ink" : "text-muted"}`} aria-sort={sort === field ? (dir === "desc" ? "descending" : "ascending") : undefined}>
      <Link to={links.hrefWith(patch)} className="inline-flex items-center gap-1 hover:underline">
        {label}
        {sort === field ? <span className="text-ink">{dir === "desc" ? "↓" : "↑"}</span> : <span className="text-line">↕</span>}
      </Link>
    </th>
  );
}

// A workflow's last runs as squares, oldest to newest, each a link to its run.
function RecentStrip({ runs }: { runs: WorkflowRow["recent"] }) {
  const { t } = useT();
  const { dateTime } = useFormat();
  if (runs.length === 0) return null;
  return (
    <div data-recent-runs className="flex items-center gap-0.5">
      {runs.map((r) => {
        const label = `${t(`state.${r.status}`)} · ${dateTime(r.at)}`;
        return <Link key={r.id} to={`/runs/${r.id}`} title={label} aria-label={label} className={`h-3 w-2 rounded-[1px] transition-opacity hover:opacity-70 ${FILL[r.status] ?? "bg-state-pending"}`} />;
      })}
    </div>
  );
}
const FILL: Record<string, string> = {
  success: "bg-state-success", failed: "bg-state-failed", running: "bg-state-running", queued: "bg-state-queued",
  retrying: "bg-state-retrying", canceled: "bg-state-canceled", skipped: "bg-state-skipped",
};

const playGlyph = (
  <svg className="h-3.5 w-3.5" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true">
    <path d="M2.5 1.6v8.8L10 6z" />
  </svg>
);
const runBtn = "inline-flex h-8 w-8 items-center justify-center rounded-card border border-line text-muted transition-colors hover:border-line-strong hover:bg-hover hover:text-ink";
const Tag = ({ tag, to }: { tag: string; to: string }) => (
  <Link to={to} className="inline-flex rounded-card border border-line bg-parchment px-2 py-0.5 text-[0.7rem] text-muted transition-colors hover:border-line-strong hover:text-ink">{tag}</Link>
);

function Row({ w, links, busy, onToggle, onRun }: { w: WorkflowRow; links: Links; busy: boolean; onToggle: () => void; onRun: (slug: string, opener: HTMLElement) => void }) {
  const { t, n } = useT();
  const { dateTime } = useFormat();
  const since = useAgo();
  const say = useMsg();
  const logoTitle = useLogoTitle();
  const schedule = say(w.schedule);
  const navigate = useNavigate();
  const paused = w.hasSchedule && !w.active;
  const href = `/workflows/${encodeURIComponent(w.slug)}`;
  const langs = w.runsIn.map((l) => l.key);
  const langNames = w.runsIn.slice(0, 2).map((l) => logoTitle(l.key)).join(" + ") + (w.runsIn.length > 2 ? ` +${w.runsIn.length - 2}` : "");
  // The whole row opens the workflow; the controls inside it keep their own click.
  const open = (e: MouseEvent) => {
    if (!(e.target as HTMLElement).closest("a, button, input, label, form")) void navigate(href);
  };
  return (
    <tr onClick={open} data-paused={paused || undefined} className="cursor-pointer align-top transition-colors hover:bg-hover">
      <td className="py-3 pl-5 max-[640px]:hidden">
        {w.hasSchedule ? (
          <ToggleSwitch on={w.active} busy={busy} label={t(w.active ? "workflows.pause" : "workflows.resume", { workflow: w.slug })} onToggle={onToggle} />
        ) : (
          <span className="block w-[30px] text-center text-faint" title={t("workflows.no_schedule.tip")}>—</span>
        )}
      </td>
      <td className="py-3 pr-3 max-[640px]:pl-4" data-fade>
        <div className="flex flex-wrap items-center gap-1.5">
          <Link to={href} className="font-mono text-[13px] font-medium text-ink hover:underline">{w.slug}</Link>
          {w.failing ? (
            <span data-pill-failing className="rounded-full bg-state-failed/15 px-2 py-px text-[0.7rem] text-state-failed">{t("workflows.pill.failing")}</span>
          ) : w.inFlight ? (
            <span data-pill-running className="rounded-full bg-state-running/15 px-2 py-px text-[0.7rem] text-state-running">{t("workflows.pill.running")}</span>
          ) : null}
        </div>
        <p className="mt-0.5 text-xs text-muted">
          <Link to={links.hrefWith({ project: w.project })} className="hover:text-ink hover:underline">{w.project}</Link>
          {w.steps !== null && <> · {n("workflows.steps", w.steps)}</>}
        </p>
      </td>
      <td className="py-3 pr-3 max-[640px]:hidden" data-fade>
        {langs.length > 0 ? (
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center gap-1">
              {langs.slice(0, 4).map((k) => <Logo key={k} k={k} />)}
            </span>
            <span className="text-xs whitespace-nowrap text-ink-2">{langNames}</span>
          </div>
        ) : (
          <span className="text-muted">—</span>
        )}
      </td>
      <td className="py-3 pr-3 max-[1100px]:hidden" data-fade>
        <span className="inline-flex flex-wrap items-center gap-1">
          {w.connects.map((l) => <Logo key={l.key} k={l.key} />)}
        </span>
      </td>
      <td className="py-3 pr-3" data-fade>
        {w.cron === "" ? (
          <span className="text-xs text-muted">{t("workflows.manual_only")}</span>
        ) : (
          <>
            {paused ? (
              <p className="text-xs text-muted">
                {t("workflows.paused")}
                {schedule && <> · {t("workflows.was", { schedule })}</>}
              </p>
            ) : (
              schedule && <p className="text-xs text-ink">{schedule}{zoneSuffix(w.timezone)}</p>
            )}
            <p className="mt-0.5 font-mono text-[0.7rem] text-faint">
              {w.cron}
              {w.catchup && <span className="ml-1.5 font-sans">catchup</span>}
            </p>
          </>
        )}
      </td>
      <td className="py-3 pr-3 text-xs tabular-nums max-[640px]:hidden" data-fade>
        {w.inFlight ? (
          <span className="text-state-running">{t("workflows.in_flight")}</span>
        ) : w.nextRun && !paused ? (
          <span className="text-ink" title={dateTime(w.nextRun)}>{since(w.nextRun)}</span>
        ) : (
          <span className="text-muted">—</span>
        )}
      </td>
      <td className="py-3 pr-3 max-[860px]:hidden" data-fade>
        {w.totalRuns > 0 || w.recent.length > 0 ? (
          <>
            <RecentStrip runs={w.recent} />
            <Link to={`/runs?workflow=${encodeURIComponent(w.slug)}`} className="mt-1 block text-[0.7rem] tabular-nums text-muted hover:underline">
              {n("workflows.runs", w.totalRuns)}
            </Link>
          </>
        ) : (
          <span className="text-xs text-muted">{t("workflows.never_ran")}</span>
        )}
      </td>
      <td className="py-3 pr-3 max-[1100px]:hidden" data-fade>
        <div className="flex flex-wrap gap-1">
          {w.tags.map((tag) => <Tag key={tag} tag={tag} to={links.hrefWith({ tag: toggled(links.sp.getAll("tag"), tag) })} />)}
        </div>
      </td>
      <td className="py-3 pr-5 text-right">
        <button type="button" aria-haspopup="dialog" title={t("workflows.run.tip")} aria-label={t("workflows.run", { workflow: w.slug })} className={runBtn} onClick={(e) => onRun(w.slug, e.currentTarget)}>{playGlyph}</button>
      </td>
    </tr>
  );
}

// The row a list of workflows draws while it waits: the REAL Row with placeholder values
// inside a .sk-ghost body, so its height is the loaded row's by construction.
const GHOST: WorkflowRow = {
  slug: "workflow_name", project: "default", steps: 3, failing: false, inFlight: false, hasParams: false,
  runsIn: [{ key: "python" }], connects: [{ key: "postgres" }], hasSchedule: true, active: true,
  cron: "0 * * * *", schedule: { k: "cron.hourly", a: { m: "00" } }, timezone: "UTC", catchup: false, nextRun: "2026-01-01T00:00:00Z", lastRunAt: null, totalRuns: 10,
  recent: Array.from({ length: 5 }, (_, i) => ({ id: `ghost-${i}`, status: "success", at: "2026-01-01T00:00:00Z" })), tags: ["tag"],
};
// 25 is the server's page size: what a first visit draws before it has seen the list.
const PAGE = 25;

function RowsSkeleton({ links }: { links: Links }) {
  const [rows] = useState(() => recalled("workflows.rows", PAGE));
  return (
    <>
      {Array.from({ length: rows }, (_, i) => (
        <Row key={i} w={{ ...GHOST, slug: `${GHOST.slug}_${i}` }} links={links} busy={false} onToggle={() => {}} onRun={() => {}} />
      ))}
    </>
  );
}

// The bar (see ui/filters): the state question is drawn at once (its counts wait); the language and
// tag lists come from what the workflows are, so they come with the data. Every change is ONE navigation.
function WorkflowFilters({ data, links }: { data?: WorkflowsList; links: Links }) {
  const { t, n } = useT();
  const navigate = useNavigate();
  const [text, setText] = useUrlSearch("q");
  const go = (patch: Patch) => void navigate(links.hrefWith(patch));
  const f = data?.filter;
  const state = f ? f.state : (links.sp.get("state") ?? "");
  const active = f ? f.active : (links.sp.get("active") ?? "");
  const project = f ? f.project : (links.sp.get("project") ?? "");
  const lang = f ? f.lang : (links.sp.get("lang") ?? "");
  const tags = f ? f.tags : links.sp.getAll("tag");
  const q = f ? f.q : (links.sp.get("q") ?? "");
  const legacyState = state !== "" && !(STATES as readonly string[]).includes(state);
  const logoTitle = useLogoTitle();
  const langName = (key: string) => logoTitle(key);
  const filters: FilterDef[] = [
    {
      id: "lang", label: t("workflows.filter.lang"), allLabel: t("workflows.lang.all"), pending: !data,
      options: (data?.langs ?? []).map((l: LogoRef & { count: number }) => ({ key: l.key, label: logoTitle(l.key), icon: <Logo k={l.key} />, count: l.count })),
      value: lang ? [lang] : [], onChange: (v) => go({ lang: v[0] ?? null }),
    },
    {
      id: "tags", label: t("workflows.col.tags"), icon: "tag", multiple: true, pending: !data, searchLabel: t("workflows.tags.filter"), noneLabel: t("workflows.tags.none"),
      options: (data?.tags ?? []).map((tag) => ({ key: tag, label: tag })),
      value: tags, onChange: (v) => go({ tag: v }),
    },
  ];
  const chips: Chip[] = [
    ...(legacyState ? [{ id: "state", group: t("workflows.chip.last_run"), value: t(`state.${state}`), onRemove: () => go({ state: null }) }] : []),
    ...(active ? [{ id: "active", group: t("workflows.chip.schedule"), value: t(active === "active" ? "workflows.active" : "workflows.paused"), onRemove: () => go({ active: null }) }] : []),
    ...(lang ? [{ id: "lang", group: t("workflows.filter.lang"), value: langName(lang), onRemove: () => go({ lang: null }) }] : []),
    ...tags.map((tag) => ({ id: `tag:${tag}`, group: t("workflows.col.tags"), value: tag, onRemove: () => go({ tag: tags.filter((x) => x !== tag) }) })),
    ...(project ? [{ id: "project", group: t("workflows.chip.project"), value: project, mono: true, onRemove: () => go({ project: null }) }] : []),
  ];
  const any = chips.length > 0 || q !== "" || (state !== "" && !legacyState);
  return (
    <FilterBar
      search={<SearchField value={text} onChange={setText} label={t("workflows.search")} />}
      primary={
        <Segmented
          label={t("workflows.filter.state")}
          pending={!data}
          value={state}
          onChange={(s) => go({ state: s })}
          items={STATES.map((s) => ({ key: s, label: t(`workflows.state.${s || "all"}`), count: data ? (data.counts[s] ?? 0) : 0 }))}
        />
      }
      filters={filters}
      chips={chips}
      clearable={any}
      onClearAll={() => go({ q: null, state: null, active: null, lang: null, tag: null, project: null })}
      count={
        <ResultCount
          pending={!data}
          shown={data?.filtered ?? 0}
          tail={<>{n("workflows.count", data?.total ?? 2)}{q && <> {t("workflows.count.search", { q })}</>}</>}
        />
      }
    />
  );
}

// The table: its header is the screen's; its rows are data. With no rows to show the
// box says so instead.
function Table({ data, links, toggler, onRun }: { data?: WorkflowsList; links: Links; toggler: ReturnType<typeof useScheduleToggle>; onRun: (slug: string, opener: HTMLElement) => void }) {
  const { t } = useT();
  // Before the early return below: a hook after it is skipped on the render where
  // a search finds nothing, and React throws (error #300) and blanks the page.
  useEffect(() => {
    if (data) {
      remember("workflows.rows", data.rows.length);
      remember("workflows.pages", Math.max(1, Math.ceil(data.filtered / data.filter.perPage)));
    }
  }, [data]);
  if (data && data.rows.length === 0) {
    return (
      <div className="rounded-card border border-dashed border-line px-6 py-12 text-center">
        <p className="text-sm text-muted">{t("workflows.empty")}</p>
        <Link to="?" className={`mt-2 inline-block text-xs ${linkClass}`}>{t("workflows.clear")}</Link>
      </div>
    );
  }
  const f = data?.filter;
  return (
    <div className="overflow-x-auto rounded-card border border-line bg-surface shadow-warm-sm">
      <table className="ct w-full text-sm min-[860px]:min-w-[60rem]" aria-busy={!data}>
        <thead>
          <tr className="border-b border-line bg-parchment-soft/60 text-left">
            <th className={th("w-16 pl-5 max-[640px]:hidden")}>{t("workflows.col.active")}</th>
            <SortTh label={t(SORTABLE[0][1])} field={SORTABLE[0][0]} extra={SORTABLE[0][2]} links={links} />
            <th className={th("max-[640px]:hidden")}>{t("workflows.col.lang")}</th>
            <th className={th("max-[1100px]:hidden")}>{t("workflows.col.connects")}</th>
            <SortTh label={t(SORTABLE[1][1])} field={SORTABLE[1][0]} extra={SORTABLE[1][2]} links={links} />
            <SortTh label={t(SORTABLE[2][1])} field={SORTABLE[2][0]} extra={SORTABLE[2][2]} links={links} />
            <SortTh label={t(SORTABLE[3][1])} field={SORTABLE[3][0]} extra={SORTABLE[3][2]} links={links} />
            <th className={th("max-[1100px]:hidden")}>{t("workflows.col.tags")}</th>
            <th className={th("w-24 pr-5 text-right")}><span className="sr-only">{t("workflows.run.tip")}</span></th>
          </tr>
        </thead>
        <tbody className={`divide-y divide-line-soft ${data ? "" : "sk-ghost"}`} aria-hidden={data ? undefined : true}>
          {data ? data.rows.map((w) => <Row key={w.slug} w={w} links={links} busy={toggler.pending === w.slug} onToggle={() => toggler.toggle(w.slug)} onRun={onRun} />) : <RowsSkeleton links={links} />}
        </tbody>
      </table>
      {data && f ? (
        <Pagination page={f.page} perPage={f.perPage} total={data.filtered} pages={Math.ceil(data.filtered / f.perPage)} to={(p) => links.hrefWith({ page: p > 1 ? String(p) : null }, true)} />
      ) : (
        <div className="sk-ghost" aria-hidden="true">
          <Pagination page={1} perPage={PAGE} total={PAGE} pages={recalled("workflows.pages", 1)} to={() => ""} />
        </div>
      )}
    </div>
  );
}

const CREATE_EXAMPLE = `name: daily_orders
schedule: "0 4 * * *"
steps:
  - id: load
    run: python load.py
  - id: build
    run: dbt build --select orders+`;

function useDocs() {
  const { lang } = useT();
  const prefix = lang === "pt-BR" ? "pt/" : lang === "zh-CN" ? "zh/" : lang === "en" || !lang ? "" : `${lang}/`;
  return `https://brevis.sh/${prefix}docs/workflows/`;
}

// A workflow is born in the repository and published by the CLI: both header
// buttons open the drawer that says how. They stay links to the docs, which
// is what the server's did with no script.
function Actions() {
  const { t } = useT();
  const docs = useDocs();
  const { show } = useDrawer("how-to-create");
  return (
    <div className="flex items-center gap-2">
      {(["secondary", "primary"] as const).map((kind) => (
        <a
          key={kind}
          href={docs}
          onClick={(e) => {
            e.preventDefault();
            show();
          }}
          className={buttonClass(kind, "regular")}
        >
          <Icon name={kind === "primary" ? "plus" : "book-open"} className="h-3.5 w-3.5" />
          {t(kind === "primary" ? "workflows.create.publish" : "workflows.create.how")}
        </a>
      ))}
    </div>
  );
}

function HowToCreate() {
  const { t } = useT();
  const docs = useDocs();
  return (
    <Drawer id="how-to-create" plain title={t("workflows.create.title")}>
      <div className="space-y-4 px-5 py-4 text-[13px] text-ink-2">
        <p>{t("workflows.create.lead")}</p>
        <div>
          <p className="mb-1.5 font-medium text-ink">1. {t("workflows.create.file")}</p>
          <CodeBlock scroll text={CREATE_EXAMPLE} />
        </div>
        <div>
          <p className="mb-1.5 font-medium text-ink">2. {t("workflows.create.validate")}</p>
          <CodeBlock scroll text="brevis validate workflows/" />
        </div>
        <div>
          <p className="mb-1.5 font-medium text-ink">3. {t("workflows.create.publish_step")}</p>
          <CodeBlock scroll text="brevis publish --project <project> workflows/" />
          <p className="mt-1.5 text-xs text-muted">{t("workflows.create.publish_note")}</p>
        </div>
        <a href={docs} className="inline-block text-ink underline decoration-line-strong underline-offset-2 hover:decoration-ink">
          {t("workflows.create.docs")}
        </a>
      </div>
    </Drawer>
  );
}

export default function WorkflowsRoute() {
  const { t } = useT();
  const links = useListLinks();
  const toggler = useScheduleToggle();
  const runDrawer = useRunDrawer();
  const qs = links.sp.toString();
  // The list keeps the previous page on screen while a filter loads, and renews itself
  // when the server reports `workflows` (the fallback polls only while the stream is down).
  useLive(["workflows"], [["workflows"]]);
  const q = useQuery({
    queryKey: ["workflows", qs],
    queryFn: () => getJSON<WorkflowsList>(`/api/workflows${qs ? `?${qs}` : ""}`),
    placeholderData: keepPreviousData,
    refetchInterval: useFallbackInterval(),
  });
  const data = q.data;
  const { note, error, dismiss, toggle } = toggler;
  return (
    <PageFrame title={t("nav.workflows")} actions={<Actions />}>
      <HowToCreate />
      <p className="mb-4 max-w-3xl text-[13.5px] text-muted">
        {t("workflows.lead.before")} <strong className="font-medium text-ink">{t("workflows.lead.strong")}</strong>
        {t("workflows.lead.after")} <Link to="/runs" className={linkClass}>{t("workflows.lead.run")}</Link>.
      </p>
      <WorkflowFilters data={data} links={links} />
      <div aria-busy={q.isPlaceholderData || !data} className={q.isPlaceholderData ? "opacity-60 transition-opacity" : undefined}>
        {q.isError && !data ? (
          <LoadError onRetry={() => void q.refetch()} />
        ) : (
          <>
            <div role="status">
              {note && (
                <p className="mb-2 text-[12.5px] text-muted">
                  {t(note.active ? "workflows.resumed.toast" : "workflows.paused.toast", { workflow: note.slug })}{" "}
                  <button type="button" className={linkClass} onClick={() => (toggle(note.slug), dismiss())}>{t("workflows.undo")}</button>
                </p>
              )}
              {error && <p className="mb-2 text-[12.5px] text-state-failed"><ErrorText error={error} /></p>}
            </div>
            <Table data={data} links={links} toggler={toggler} onRun={(slug, el) => runDrawer.open({ slug }, el)} />
          </>
        )}
      </div>
      {runDrawer.drawer}
    </PageFrame>
  );
}
