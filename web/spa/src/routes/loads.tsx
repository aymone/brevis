import { useEffect, useRef } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import { ApiError } from "../api/client";
import { useData } from "../api/live";
import type { LoadRow, Loads, Source } from "../api/ingest";
import { cat, useFormat, useMsg, useT } from "../i18n";
import { localStamp, useSince } from "../ui/format";
import { Icon } from "../ui/icon";
import { ButtonLink, Empty, Lead, PageHead, linkUnderline, Page } from "../ui/kit";
import { Logo } from "../ui/logo";
import { useSourceLabel } from "../ui/place";
import { FilterBar, ResultCount, SearchField, ToggleFilter, useUrlSearch, type Chip, type FilterDef } from "../ui/filters";
import { useSeen } from "../ui/seen";
import { useStatusFilter } from "../ui/status-filter";
import { Sparkline } from "../ui/sparkline";
import { Freshness, FreshnessMark } from "../ui/status";
import { Row, Table, Td, Th } from "../ui/table";
import { useFilter } from "../ui/use-filter";
import { useCrumb } from "../shell/nav";
import { LoadDrawer, useLoad } from "./load";
import "./loads.css";
import { ErrorText } from "../api/errors";

// The statuses a load can have, in the band's order (a step is never
// continuous, nor its destination unidentified).
const STATUSES = ["stale", "late", "on_time", "paused", "unscheduled"] as const;
const FIELDS = ["status", "source", "legacy", "q"] as const;
type Filter = ReturnType<typeof useFilter>;

/** Where the load reads, as one quiet line under the destination: "HTTP API · api.example.com". A step the SDK did not describe says so. */
function From({ s }: { s: Source }) {
  const { t } = useT();
  const sourceLabel = useSourceLabel();
  if (s.kind === "")
    return (
      <p className="ldl-from ldl-none" title={t("loads.source.none.tip")}>
        <span className="sr-only">{t("loads.source")}: </span>
        {t("loads.source.none")}
      </p>
    );
  return (
    <p className="ldl-from">
      <span className="sr-only">{t("loads.source")}: </span>
      <span aria-hidden="true">&larr; </span>
      {sourceLabel(s)}
      {s.where && <span> &middot; {s.where}</span>}
    </p>
  );
}

function Head() {
  const { t } = useT();
  return (
    <>
      <Th edge="first">{t("loads.col.who")}</Th>
      <Th>{t("loads.col.flow")}</Th>
      <Th>{t("loads.col.status")}</Th>
      <Th>{t("loads.col.last")}</Th>
      <Th cls="text-right">{t("loads.col.rows")}</Th>
      <Th edge="last" cls="max-[1100px]:hidden">
        {t("loads.col.recent")}
      </Th>
    </>
  );
}

// A row opens the load's drawer at its address, with the list's filter after it so that closing comes back to
// the list as it was (the old console's keepFilter). `drawer` tells the drawer it was pushed, so closing steps back.
function useRowHref(filter: Filter) {
  return (href: string) => {
    if (href.startsWith("#")) return href;
    const [path = "", query = ""] = href.split("?");
    const q = new URLSearchParams(query);
    for (const f of FIELDS) if (filter.get(f)) q.set(f, filter.get(f));
    return q.size ? `${path}?${q}` : path;
  };
}
const PUSHED = { drawer: true };

// The tooltip keeps what the line gave up: the full destination URI, its note, and where it is read from.
function destTitle(r: LoadRow, sourceLabel: (s: Source) => string) {
  const to = r.target.uri === "" ? r.target.note : r.target.note ? `${r.target.uri} \u2014 ${r.target.note}` : r.target.uri;
  return r.source.kind === "" ? to : `${sourceLabel(r.source)}${r.source.where ? ` (${r.source.where})` : ""} \u2192 ${to}`;
}

function Rows({ rows, filter }: { rows: LoadRow[]; filter: Filter }) {
  const href = useRowHref(filter);
  const { t, n } = useT();
  const { number } = useFormat();
  const msg = useMsg();
  const sourceLabel = useSourceLabel();
  const since = useSince();
  const navigate = useNavigate();
  return (
    <>
      {rows.map((r) => (
        <Row key={r.href} onOpen={() => navigate(href(r.href), { state: PUSHED })}>
          <Td edge="first" cls="ldl-fill">
            <Link to={href(r.href)} state={PUSHED} data-load={`${r.workflow}/${r.step}`} data-target={r.target.uri} className="ldl-link" title={`${r.workflow} · ${r.step}`}>
              <span className="ldl-wf">{r.workflow}</span>
              <span className="ldl-step">{r.step}</span>
            </Link>
            <p className="ldl-sub" title={msg(r.schedule)}>
              {msg(r.schedule)}
            </p>
          </Td>
          <Td cls="ldl-fill-dest">
            <div title={destTitle(r, sourceLabel)}>
              <p className="ldl-to">
                {r.target.kind !== "" && <Logo k={r.target.kind} />}
                {r.target.uri === "" ? (
                  <span className="ldl-what ldl-none">{t("streams.unidentified")}</span>
                ) : (
                  <>
                    <span className="ldl-where">{r.target.where}</span>
                    <span className="ldl-what">{r.target.what}</span>
                  </>
                )}
                {r.legacy && <span className="ldl-chip">{t("catalog.chip.legacy")}</span>}
              </p>
              <From s={r.source} />
            </div>
          </Td>
          <Td>
            <Freshness status={r.status} />
            {r.fixing && (
              <Link to={`/runs/${r.fixing}`} className="ldl-fixing">
                {t("loads.fixing")}
              </Link>
            )}
          </Td>
          <Td cls="ldl-last" title={r.lastLoaded ? localStamp(r.lastLoaded) : undefined}>
            <span>{r.lastLoaded ? since(r.lastLoaded) : "—"}</span>
          </Td>
          <Td cls="ldl-rows">
            {r.rows === null ? (
              <span className="text-faint" title={t("catalog.no_rows")}>
                —
              </span>
            ) : (
              <span title={n("loads.drawer.rows", r.rows)}>{number(r.rows)}</span>
            )}
          </Td>
          <Td edge="last" cls="ldl-recent">
            <Sparkline values={r.recent} />
          </Td>
        </Row>
      ))}
    </>
  );
}

// What the screen shows before its first answer: the last shape it saw (rows, counts, filters), or
// this one on a first visit. The rows are the real ones drawn from placeholder values, so they take
// the height the loaded ones take.
type Shape = { n: number; total: number; workflows: number; summary: Record<string, number>; counts: Record<string, number>; sources: number; hasLegacy: boolean };
// A first visit draws the empty state (a fresh install has no loads); the box is the one the real empty state fills.
const FIRST: Shape = { n: 0, total: 0, workflows: 0, summary: {}, counts: {}, sources: 0, hasLegacy: false };
// Only numbers are remembered, and only under the fixed status words: nothing the API says about the data.
const numbers = (r: Record<string, number>) => Object.fromEntries(Object.entries(r).filter(([k, n]) => (k === "" || STATUSES.includes(k as never) || k === "attention") && typeof n === "number"));
const shapeOf = (v: Loads): Shape => ({ n: v.rows.length, total: v.total, workflows: v.workflows, summary: numbers(v.summary), counts: numbers(v.counts), sources: v.sources.length, hasLegacy: v.hasLegacy });
const GHOST_ROW: LoadRow = {
  workflow: "workflow_name",
  step: "load_step_name",
  href: "",
  target: { uri: "", kind: "postgres", where: "postgres://warehouse/", what: "table_name", note: "" },
  legacy: false,
  status: "on_time",
  lastLoaded: "2000-01-01T00:00:00Z",
  rows: 1000,
  schedule: cat("every hour"),
  source: { kind: "http", driver: "", label: "Source label", labelMsg: null, where: "api.example.com" },
  recent: [1, 3, 2, 4],
  fixing: null,
};
const ghostOf = (s: Shape): Loads => ({
  rows: Array.from({ length: s.n }, (_, i) => ({ ...GHOST_ROW, href: `#${i}` })),
  total: s.total,
  workflows: s.workflows,
  summary: s.summary,
  counts: s.counts,
  sources: Array.from({ length: s.sources }, (_, i) => ({ key: `source${i}`, label: { k: "loads.source.other" } })),
  hasLegacy: s.hasLegacy,
  filter: { status: "", source: "", q: "", legacy: false },
});

function Summary({ v, ghost }: { v: Loads; ghost: boolean }) {
  const { t, n } = useT();
  if (v.total === 0) return null;
  return (
    <div aria-hidden={ghost || undefined} className={`mb-4 flex ${ghost ? "sk-ghost" : ""} flex-wrap items-baseline gap-x-5 gap-y-1 text-[13px]`}>
      <span className="text-muted">{n("loads.summary.destinations", v.total)}</span>
      <span className="text-muted">{n("loads.summary.workflows", v.workflows)}</span>
      {STATUSES.filter((s) => (v.summary[s] ?? 0) > 0).map((s) => (
        <span key={s} className="inline-flex items-baseline gap-1.5">
          <span className="font-medium text-ink tabular-nums">{v.summary[s]}</span>
          <Freshness status={s} />
        </span>
      ))}
      <Link to={`/data${v.filter.status ? `?status=${encodeURIComponent(v.filter.status)}` : ""}`} className="text-ink-2 hover:text-ink hover:underline">
        {t("loads.catalog")} &rarr;
      </Link>
    </div>
  );
}

// The bar (see ui/filters): drawn at once from the last shape seen; only the counts wait.
function LoadsBar({ filter, v, ghost }: { filter: Filter; v: Loads; ghost: boolean }) {
  const { t, n } = useT();
  const msg = useMsg();
  const { number } = useFormat();
  const navigate = useNavigate();
  const [text, setText] = useUrlSearch("q");
  const go = (patch: Record<string, string>) => void navigate(filter.hrefWith(patch));
  const source = filter.get("source");
  const legacy = filter.get("legacy") === "1";
  const q = filter.get("q");
  const st = useStatusFilter({
    statuses: STATUSES,
    count: (k) => v.counts[k] ?? 0,
    pending: ghost,
    mark: (s) => <FreshnessMark status={s} />,
    status: filter.get("status"),
    onChange: (status) => go({ status }),
  });
  const sourceMark = (k: string) => (k === "http" || k === "file" ? <Logo k={k} /> : k === "database" ? <Icon name="database" className="h-3.5 w-3.5 text-muted" /> : undefined);
  const filters: FilterDef[] = [
    st.def,
    ...(v.sources.length > 1
      ? [{
          id: "source", label: t("loads.source"), allLabel: t("loads.source.all"), pending: ghost,
          options: v.sources.map((s) => ({ key: s.key, label: msg(s.label), icon: sourceMark(s.key) })),
          value: source ? [source] : [], onChange: (x: string[]) => go({ source: x[0] ?? "" }),
        } satisfies FilterDef]
      : []),
  ];
  const chips: Chip[] = [
    ...st.chip,
    ...(source ? [{ id: "source", group: t("loads.source"), value: msg(v.sources.find((s) => s.key === source)?.label) || source, onRemove: () => go({ source: "" }) }] : []),
  ];
  return (
    <FilterBar
      search={<SearchField value={text} onChange={setText} label={t("loads.search")} />}
      primary={<>{st.primary}{v.hasLegacy && <ToggleFilter label={t("catalog.legacy")} on={legacy} onChange={(on) => go({ legacy: on ? "1" : "" })} />}</>}
      filters={filters}
      chips={chips}
      clearable={filter.any}
      onClearAll={() => go({ status: "", source: "", legacy: "", q: "" })}
      count={<ResultCount pending={ghost} shown={v.rows.length} tail={<>{n("loads.count", v.total, { count: number(v.total) })}{q && <> {t("workflows.count.search", { q })}</>}</>} />}
    />
  );
}

// The phone draws cards, whose heights are not the table rows': each keeps its own memory of row heights.
const phone = () => typeof matchMedia === "function" && matchMedia("(max-width: 768px)").matches;

function LoadsData({ filter }: { filter: Filter }) {
  const { t } = useT();
  const qs = new URLSearchParams();
  for (const f of FIELDS) if (filter.get(f)) qs.set(f, filter.get(f));
  const q = useData<Loads>(["ingest", "loads", qs.toString()], `/api/ingest/loads${qs.size ? `?${qs}` : ""}`, { topic: "ingest" });
  const ghost = !q.data;
  // Only the unfiltered answer is remembered: a filter's counts are not the list's shape.
  const seen = useSeen("loads", qs.size === 0 && q.data ? shapeOf(q.data) : undefined, FIRST);
  const v = q.data ?? ghostOf(seen);
  const shown = v.total > 0 || v.hasLegacy;
  const none = v.rows.length === 0;
  return (
    <>
      <Summary v={v} ghost={ghost} />
      {shown && (
        <LoadsBar filter={filter} v={v} ghost={ghost} />
      )}
      <div aria-busy={ghost}>
        {q.isError ? (
          <p role="alert" className="rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">
            <ErrorText error={q.error} />
          </p>
        ) : none ? (
          <div aria-hidden={ghost || undefined} className={ghost ? "sk-ghost" : undefined}>
          {filter.any && shown ? (
            <Empty msg={t("loads.empty.filtered")}>
              <Link to="/ingest/loads" className={`mt-2 inline-block text-xs ${linkUnderline}`}>
                {t("workflows.clear")}
              </Link>
            </Empty>
          ) : (
            <div className="rounded-card border border-dashed border-line px-6 py-12 text-center">
              <p className="text-sm text-muted">{t("loads.empty")}</p>
              <p className="mt-1 text-xs text-faint">{t("loads.empty.how")}</p>
            </div>
          )}
          </div>
        ) : (
          <Table id="load-rows" cls="ldl" head={<Head />} foot={t("loads.honest")} ghost={ghost} memo={phone() ? "loads.phone" : "loads"}>
            <Rows rows={v.rows} filter={filter} />
          </Table>
        )}
      </div>
    </>
  );
}

/** The list; a load's address (/ingest/loads/:workflow/:step) draws the same list with that load's drawer open over it. */
export default function LoadsRoute() {
  const { t } = useT();
  const filter = useFilter(FIELDS, "/ingest/loads");
  const { workflow = "", step = "" } = useParams();
  const { state, search } = useLocation();
  const navigate = useNavigate();
  const open = workflow !== "" && step !== "";
  const target = new URLSearchParams(search).get("target") ?? "";
  useCrumb(open ? `${workflow} · ${step}` : "");
  const load = useLoad(workflow, step, target, open);
  // A step that writes no destination is the list with a note, as the server drew it.
  const missing = open && load.error instanceof ApiError && load.error.status === 404 ? `${workflow} · ${step}` : "";

  // Pushed by a row: Back is the way out, so the history does not grow. Entered by address: replace it.
  const close = () => (state?.drawer ? void navigate(-1) : void navigate(filter.hrefWith({}), { replace: true }));
  // The focus goes back to the row that opened it, whichever way the drawer was left (Esc, backdrop, ×, Back).
  const was = useRef<{ key: string; target: string } | null>(null);
  const shown = open && missing === "";
  useEffect(() => {
    if (shown) {
      was.current = { key: `${workflow}/${step}`, target };
      return;
    }
    const w = was.current;
    was.current = null;
    if (w) [...document.querySelectorAll<HTMLElement>("#load-rows [data-load]")].find((a) => a.dataset.load === w.key && (!w.target || a.dataset.target === w.target))?.focus();
  }, [shown, workflow, step, target]);

  return (
    <Page>
      <PageHead
        title={t("loads.title")}
        actions={
          <ButtonLink to="/workflows" icon="workflow">
            {t("loads.workflows")}
          </ButtonLink>
        }
      />
      <Lead before={t("loads.lead.before")} strong={t("loads.lead.strong")} after={t("loads.lead.after")} />
      {missing !== "" && (
        <p className="mb-3 rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">{t("loads.missing", { step: missing })}</p>
      )}
      <LoadsData filter={filter} />
      {shown && <LoadDrawer workflow={workflow} step={step} target={target} q={load} close={close} />}
    </Page>
  );
}
