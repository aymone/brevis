import { lazy, Suspense } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router";
import { ApiError } from "../api/client";
import { useData } from "../api/live";
import { useFormat, useT } from "../i18n";
import { useSince } from "../ui/format";
import { Icon } from "../ui/icon";
import { Empty, Lead, PageHead, linkUnderline, Page } from "../ui/kit";
import { Logo, useLogoTitle } from "../ui/logo";
import { FilterBar, ResultCount, SearchField, ToggleFilter, useUrlSearch, type Chip, type FilterDef } from "../ui/filters";
import { useSeen } from "../ui/seen";
import { useStatusFilter } from "../ui/status-filter";
import { Freshness, StatusMark, freshnessKind } from "../ui/status";
import { Row, Table, Td, Th } from "../ui/table";
import { useFilter } from "../ui/use-filter";
import type { TargetDetail } from "./target-drawer";
import "./catalog-list.css";
import { ErrorText } from "../api/errors";

export type CatalogRow = {
  target: string;
  kind: string;
  legacy: boolean;
  status: string;
  where: string;
  what: string;
  autoTable: boolean;
  writer: string;
  more: number;
  queued: boolean;
  lastAt: string | null;
  lastRows: number | null;
  recent: (number | null)[];
  streamHref: string;
};
type Catalog = {
  now: string;
  kinds: string[];
  hasLegacy: boolean;
  total: number;
  shown: number;
  counts: Record<string, number>;
  rows: CatalogRow[];
};

const FIELDS = ["status", "kind", "legacy", "q"] as const;
const STATUSES = ["stale", "late", "on_time", "continuous", "paused", "unscheduled", "unidentified"];
type Filter = ReturnType<typeof useFilter>;

// The drawer is a separate chunk: most visits to the catalog never open one.
const TargetDrawer = lazy(() => import("./target-drawer"));

/** The last loads of a destination as small bars, the tallest the largest; a load that did not say its rows is an outline. */
function Recent({ counts }: { counts: (number | null)[] }) {
  if (counts.length === 0) return null;
  const peak = Math.max(0, ...counts.map((c) => c ?? 0));
  return (
    <span className="inline-flex h-5 items-end gap-px" aria-hidden="true">
      {counts.map((c, i) =>
        c === null ? (
          <span key={i} className="h-full w-1 rounded-[1px] border border-line-strong" />
        ) : (
          <span key={i} className="w-1 rounded-[1px] bg-bar" style={{ height: `${peak > 0 ? Math.max(12, Math.floor((c * 100) / peak)) : 12}%` }} />
        ),
      )}
    </span>
  );
}

function Head() {
  const { t } = useT();
  return (
    <>
      <Th edge="first" cls="whitespace-nowrap">{t("catalog.col.target")}</Th>
      <Th cls="whitespace-nowrap">{t("catalog.col.status")}</Th>
      <Th cls="whitespace-nowrap">{t("catalog.col.last")}</Th>
      <Th cls="whitespace-nowrap text-right">{t("catalog.col.rows")}</Th>
      <Th cls="whitespace-nowrap">{t("catalog.col.recent")}</Th>
      <Th edge="last" cls="w-8" />
    </>
  );
}

function CatalogRows({ rows, filter, now }: { rows: CatalogRow[]; filter: Filter; now: string }) {
  const { t } = useT();
  const f = useFormat();
  const since = useSince();
  const navigate = useNavigate();
  return (
    <>
      {rows.map((r) => {
        const href = r.streamHref || filter.hrefWith({}, "/data/target", { u: r.target });
        // A gateway-only row already names its writer as the destination; any other row says it once, muted, under it.
        const sub = r.target === "" ? "" : r.writer;
        return (
          <Row key={r.target || r.streamHref} onOpen={() => navigate(href)}>
            <Td edge="first" cls="ctl-fill">
              <div className="ctl-to">
                <Link to={href} className="ctl-link" title={r.target || r.writer}>
                  {r.target === "" ? (
                    <>
                      <Logo k="gateway" />
                      <span className="ctl-what">{r.writer}</span>
                    </>
                  ) : (
                    <>
                      <Logo k={r.kind} />
                      <span className="ctl-where">{r.where}</span>
                      <span className="ctl-what">{r.what}</span>
                    </>
                  )}
                </Link>
                {r.legacy && <span className="ctl-chip">{t("catalog.chip.legacy")}</span>}
                {r.autoTable && <span className="ctl-chip">auto_table</span>}
              </div>
              {sub !== "" && (
                <p className="ctl-sub" title={`${t("catalog.col.writer")}: ${sub}${r.more > 0 ? ` +${r.more}` : ""}`}>
                  <span className="sr-only">{t("catalog.col.writer")}: </span>
                  {sub}
                  {r.more > 0 && <span className="ctl-more">+{r.more}</span>}
                </p>
              )}
            </Td>
            <Td>
              <Freshness status={r.status} />
              {r.queued && <p className="mt-0.5 text-[11.5px] text-muted">{t("catalog.queued")}</p>}
            </Td>
            <Td cls="ctl-last"><span>{r.lastAt ? since(r.lastAt, now) : "—"}</span></Td>
            <Td cls="ctl-rows">
              {r.lastRows !== null ? f.number(r.lastRows) : <span title={t("catalog.no_rows")} className="text-faint">—</span>}
            </Td>
            <Td cls="ctl-recent">
              <Recent counts={r.recent} />
            </Td>
            <Td edge="last" cls="ctl-go text-right text-faint">
              <Icon name="chevron-right" className="ml-auto h-4 w-4" />
            </Td>
          </Row>
        );
      })}
    </>
  );
}

// What the screen shows before its first answer: the last shape it saw (rows, counts, filters), or
// this one on a first visit, drawn from the real row markup with placeholder values so it takes the
// height the loaded rows take.
type Shape = Omit<Catalog, "rows" | "now" | "kinds"> & { n: number; kinds: number };
const FIRST: Shape = { n: 4, kinds: 2, hasLegacy: false, total: 4, shown: 4, counts: { all: 4, attention: 0, continuous: 4 } };
const shapeOf = (d: Catalog): Shape => ({ n: d.rows.length, kinds: d.kinds.length, hasLegacy: d.hasLegacy, total: d.total, shown: d.shown, counts: Object.fromEntries(Object.entries(d.counts).filter(([k, n]) => (k === "all" || k === "attention" || STATUSES.includes(k)) && typeof n === "number")) });
const GHOST_ROW: CatalogRow = {
  target: "postgres://warehouse/schema/table_name",
  kind: "postgres",
  legacy: false,
  status: "on_time",
  where: "postgres://warehouse/schema/",
  what: "table_name",
  autoTable: false,
  writer: "workflow_name",
  more: 0,
  queued: false,
  lastAt: "2000-01-01T00:00:00Z",
  lastRows: 1000,
  recent: [1, 3, 2, 4],
  streamHref: "",
};
const ghostOf = (s: Shape): Catalog => ({ ...s, kinds: Array.from({ length: s.kinds }, (_, i) => ["postgres", "file", "pubsub"][i % 3] ?? "file"), now: "2000-01-02T00:00:00Z", rows: Array.from({ length: s.n }, (_, i) => ({ ...GHOST_ROW, target: `${GHOST_ROW.target}_${i}` })) });

// The bar (see ui/filters). Static: the controls are drawn at once from the last shape seen; only the counts wait.
function CatalogBar({ filter, data, ghost }: { filter: Filter; data: Catalog; ghost: boolean }) {
  const { t, n } = useT();
  const { number } = useFormat();
  const logoTitle = useLogoTitle();
  const navigate = useNavigate();
  const [text, setText] = useUrlSearch("q");
  const go = (patch: Record<string, string>) => void navigate(filter.hrefWith(patch));
  const kind = filter.get("kind");
  const legacy = filter.get("legacy") === "1";
  const q = filter.get("q");
  const st = useStatusFilter({
    statuses: STATUSES,
    count: (k) => data.counts[k === "" ? "all" : k] ?? 0,
    pending: ghost,
    mark: (s) => <StatusMark kind={freshnessKind(s)} />,
    status: filter.get("status"),
    onChange: (status) => go({ status }),
  });
  const filters: FilterDef[] = [
    st.def,
    ...(data.kinds.length > 1
      ? [{
          id: "kind", label: t("catalog.kind"), allLabel: t("catalog.kind.all"), pending: ghost,
          options: data.kinds.map((k) => ({ key: k, label: logoTitle(k), icon: <Logo k={k} /> })),
          value: kind ? [kind] : [], onChange: (v: string[]) => go({ kind: v[0] ?? "" }),
        } satisfies FilterDef]
      : []),
  ];
  const chips: Chip[] = [
    ...st.chip,
    ...(kind ? [{ id: "kind", group: t("catalog.kind"), value: logoTitle(kind), onRemove: () => go({ kind: "" }) }] : []),
  ];
  return (
    <FilterBar
      search={<SearchField value={text} onChange={setText} label={t("catalog.search")} />}
      primary={<>{st.primary}{data.hasLegacy && <ToggleFilter label={t("catalog.legacy")} on={legacy} onChange={(on) => go({ legacy: on ? "1" : "" })} />}</>}
      filters={filters}
      chips={chips}
      clearable={filter.any}
      onClearAll={() => go({ status: "", kind: "", legacy: "", q: "" })}
      count={<ResultCount pending={ghost} shown={data.shown} tail={<>{n("catalog.count", data.total, { count: number(data.total) })}{q && <> {t("workflows.count.search", { q })}</>}</>} />}
    />
  );
}

// The phone draws cards, whose heights are not the table rows': each keeps its own memory of row heights.
const phone = () => typeof matchMedia === "function" && matchMedia("(max-width: 768px)").matches;

function Catalog({ filter }: { filter: Filter }) {
  const { t } = useT();
  const qs = new URLSearchParams();
  for (const f of FIELDS) if (filter.get(f)) qs.set(f, filter.get(f));
  const q = useData<Catalog>(["catalog", qs.toString()], `/api/catalog${qs.size ? `?${qs}` : ""}`, { topic: "catalog" });
  const ghost = !q.data;
  // Only the unfiltered answer is remembered: a filter's counts are not the list's shape.
  const seen = useSeen("catalog", qs.size === 0 && q.data ? shapeOf(q.data) : undefined, FIRST);
  const data = q.data ?? ghostOf(seen);
  const empty = data.rows.length === 0;
  return (
    <div aria-busy={ghost}>
      <CatalogBar filter={filter} data={data} ghost={ghost} />
      {q.isError ? (
        <p role="alert" className="rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">
          <ErrorText error={q.error} />
        </p>
      ) : empty ? (
        <div aria-hidden={ghost || undefined} className={ghost ? "sk-ghost" : undefined}>
        <Empty msg={filter.any ? t("catalog.empty.filtered") : t("catalog.empty")}>
          {filter.any && (
            <Link to="/data" className={`mt-2 inline-block text-xs ${linkUnderline}`}>
              {t("workflows.clear")}
            </Link>
          )}
        </Empty>
        </div>
      ) : (
        <Table catalog cls="ctl" head={<Head />} foot={`${t("catalog.order")} ${t("catalog.honest")}`} ghost={ghost} memo={phone() ? "catalog.phone" : "catalog"}>
          <CatalogRows rows={data.rows} filter={filter} now={data.now} />
        </Table>
      )}
    </div>
  );
}

export default function CatalogRoute() {
  const { t } = useT();
  const { pathname } = useLocation();
  const [params] = useSearchParams();
  const filter = useFilter(FIELDS, "/data");

  // /data/target?u= is the catalog with one destination's drawer open over it.
  const u = pathname.endsWith("/data/target") ? (params.get("u") ?? "") : null;
  const target = useData<TargetDetail>(["catalog-target", u], `/api/catalog/target?u=${encodeURIComponent(u ?? "")}`, { topic: "catalog", enabled: !!u, keep: false });
  const missing = u !== null && target.error instanceof ApiError && target.error.status === 404;

  return (
    <Page>
      <PageHead title={t("catalog.title")} />
      <Lead before={t("catalog.lead.before")} strong={t("catalog.lead.strong")} after={t("catalog.lead.after")} />
      {missing && (
        <p className="mb-3 rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">{t("catalog.missing", { target: u ?? "" })}</p>
      )}
      <Catalog filter={filter} />
      {u !== null && !missing && (
        <Suspense fallback={null}>
          <TargetDrawer u={u} query={target} closeTo={filter.hrefWith({}, "/data")} />
        </Suspense>
      )}
    </Page>
  );
}
