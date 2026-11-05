import { Link, useNavigate } from "react-router";
import { EndpointRowRate, EndpointsStrip } from "../ui/metrics/endpoints";
import { useData } from "../api/live";
import { useFormat, useT, type Vars } from "../i18n";
import { useSince } from "../ui/format";
import { useDrawer } from "../ui/drawer";
import { HowDrawer, HowSteps } from "../ui/how";
import { btn, Empty, Lead, MethodPill, PageHead, linkUnderline, Page } from "../ui/kit";
import { Icon } from "../ui/icon";
import { Logo } from "../ui/logo";
import { FilterBar, ResultCount, SearchField, Segmented, useUrlSearch, type Chip, type FilterDef } from "../ui/filters";
import { useSeen } from "../ui/seen";
import { FreshnessMark } from "../ui/status";
import { Row, Table, Td, Th } from "../ui/table";
import { useFilter } from "../ui/use-filter";
import { ErrorText } from "../api/errors";

export type EndpointRow = {
  api: string;
  name: string;
  path: string;
  keyed: boolean;
  about: string;
  freshness: "stale" | "fresh" | "";
  reads: string[];
  rebuilt: { at: string; by: string } | null;
  ttl: string;
  params: number;
  required: number;
  publishedAt: string;
};
type Endpoints = {
  now: string;
  total: number;
  apis: number;
  fresh: number;
  stale: number;
  caches: string[];
  connections: string[];
  apiNames: string[];
  rows: EndpointRow[];
};

const FIELDS = ["api", "state", "q"] as const;
type Filter = ReturnType<typeof useFilter>;

// The example the publish drawer shows: an API's file with one endpoint, and how it reaches the console.
const PUBLISH_YAML = `name: metrics-api
connection: warehouse

cache:
  type: redis
  addr_from: BREVIS_CACHE_ADDR
  ttl: 6h

endpoints:
  #: Clicks per day, for the customer's dashboard.
  - name: clicks
    path: /v1/clicks
    tenant: key
    params:
      start_date: {type: date, required: true}
      end_date: {type: date, required: true}
    sql_file: endpoints/clicks.sql`;
const PUBLISH_COMMANDS = `brevis-sql-api describe --config api.yaml --out manifest.json
brevis endpoints publish manifest.json`;

export const endpointHref = (r: { api: string; name: string }) => `/data/endpoints/${encodeURIComponent(r.api)}/${encodeURIComponent(r.name)}`;

/** How an endpoint's data stands, by the tables it reads: fresh, stale, or not known to the catalog. */
export function EndpointFreshness({ value }: { value: string }) {
  const { t } = useT();
  if (value === "stale")
    return (
      <span className="inline-flex items-center gap-1.5 text-[12.5px] text-state-queued">
        <FreshnessMark status="stale" />
        {t("endpoints.state.stale")}
      </span>
    );
  if (value === "fresh")
    return (
      <span className="inline-flex items-center gap-1.5 text-[12.5px] text-state-success">
        <FreshnessMark status="on_time" />
        {t("endpoints.state.fresh")}
      </span>
    );
  return (
    <span className="text-[12.5px] text-faint" title={t("endpoints.state.unknown.tip")}>
      {t("endpoints.state.unknown")}
    </span>
  );
}

/** Splits `postgres://db/schema/` from `table`: the part every row shares is faint. */
export const splitTarget = (t: string): [string, string] => {
  if (!t.includes("://")) {
    const i = t.lastIndexOf(".");
    return i >= 0 ? [t.slice(0, i + 1), t.slice(i + 1)] : ["", t];
  }
  if (t.endsWith("/")) return [t, ""];
  const i = t.lastIndexOf("/");
  return [t.slice(0, i + 1), t.slice(i + 1)];
};

/** The scheme of a destination URI, which is the kind whose mark draws it. */
export const kindOf = (uri: string) => (uri.includes("://") ? uri.slice(0, uri.indexOf("://")) : "");

/** "8 · 2 required", or how many when none is required. */
export function useParamsSaid() {
  const { t } = useT();
  return (params: number, required: number) =>
    params === 0 ? t("endpoints.params.none") : required > 0 ? t("endpoints.params.required", { n: params, required } satisfies Vars) : String(params);
}

function Head() {
  const { t } = useT();
  return (
    <>
      <Th edge="first">{t("endpoints.col.endpoint")}</Th>
      <Th cls="max-[1280px]:hidden">{t("endpoints.col.api")}</Th>
      <Th cls="max-[960px]:hidden">{t("endpoints.col.reads")}</Th>
      <Th cls="max-[1180px]:hidden">{t("endpoints.col.cache")}</Th>
      <Th cls="max-[1180px]:hidden">{t("endpoints.col.params")}</Th>
      <Th>{t("endpoints.col.status")}</Th>
      <Th cls="max-[1280px]:hidden">{t("endpoints.col.published")}</Th>
      <Th edge="last" cls="w-8" />
    </>
  );
}

function Rows({ rows, now }: { rows: EndpointRow[]; now: string }) {
  const { t } = useT();
  const since = useSince();
  const navigate = useNavigate();
  const paramsSaid = useParamsSaid();
  return (
    <>
      {rows.map((r) => {
        const first = r.reads[0] ?? "";
        const [where, what] = splitTarget(first);
        const href = endpointHref(r);
        return (
          <Row key={r.api + "/" + r.name} onOpen={() => navigate(href)}>
            <Td edge="first">
              <div className="mx-line">
                <Link to={href} className="inline-flex min-w-0 items-center gap-2 font-mono text-[13px]">
                  <MethodPill method="GET" />
                  <span className="truncate font-medium text-ink">{r.path}</span>
                </Link>
                <EndpointRowRate api={r.api} name={r.name} />
              </div>
              {r.about && (
                <p className="mt-1 max-w-[22rem] truncate text-[12px] text-muted" title={r.about}>
                  {r.about}
                </p>
              )}
            </Td>
            <Td cls="max-[1280px]:hidden">
              <span className="rounded-card border border-line px-1.5 py-px font-mono text-[11.5px] text-ink-2">{r.api}</span>
            </Td>
            <Td cls="max-[960px]:hidden">
              {r.reads.length > 0 && (
                <div className="max-w-[18rem] min-w-0">
                  <p className="flex min-w-0 items-center gap-1.5 font-mono text-[12px]" title={first}>
                    <Logo k={kindOf(first)} />
                    <span className="truncate text-faint">{where}</span>
                    <span className="shrink-0 text-ink">{what}</span>
                    {r.reads.length > 1 && (
                      <span className="shrink-0 font-sans text-[11.5px] text-muted" title={r.reads.slice(1).join("\n")}>
                        +{r.reads.length - 1}
                      </span>
                    )}
                  </p>
                  {r.rebuilt && <p className="mt-0.5 truncate text-[11.5px] text-muted">{t("endpoints.rebuilt", { when: since(r.rebuilt.at, now), by: r.rebuilt.by })}</p>}
                </div>
              )}
            </Td>
            <Td cls="text-[12.5px] whitespace-nowrap text-ink-2 max-[1180px]:hidden">
              {r.ttl ? <span>{t("endpoints.ttl", { ttl: r.ttl })}</span> : <span className="text-faint">—</span>}
            </Td>
            <Td cls="text-[12.5px] whitespace-nowrap text-ink-2 max-[1180px]:hidden"><span>{paramsSaid(r.params, r.required)}</span></Td>
            <Td cls="whitespace-nowrap">
              <EndpointFreshness value={r.freshness} />
            </Td>
            <Td cls="text-xs whitespace-nowrap text-muted max-[1280px]:hidden"><span>{since(r.publishedAt, now)}</span></Td>
            <Td edge="last" cls="text-right text-faint">
              <Icon name="chevron-right" className="ml-auto h-4 w-4" />
            </Td>
          </Row>
        );
      })}
    </>
  );
}

// What the screen shows before its first answer: the last shape it saw, or this one on a first visit,
// drawn from the real row markup with placeholder values so it takes the height the loaded rows take.
type Shape = { n: number; total: number; apis: number; fresh: number; stale: number; caches: number; connections: number; apiNames: number };
// A first visit draws the empty state (a fresh install has no endpoints); the box is the one the real empty state fills.
const FIRST: Shape = { n: 0, total: 0, apis: 0, fresh: 0, stale: 0, caches: 0, connections: 0, apiNames: 0 };
const shapeOf = (d: Endpoints): Shape => ({ n: d.rows.length, total: d.total, apis: d.apis, fresh: d.fresh, stale: d.stale, caches: d.caches.length, connections: d.connections.length, apiNames: d.apiNames.length });
const GHOST_ROW: EndpointRow = {
  api: "api-name",
  name: "endpoint",
  path: "/v1/endpoint_path",
  keyed: false,
  about: "",
  freshness: "fresh",
  reads: ["postgres://warehouse/schema/table_name"],
  rebuilt: null,
  ttl: "6h",
  params: 2,
  required: 1,
  publishedAt: "2000-01-01T00:00:00Z",
};
const ghostOf = (s: Shape): Endpoints => ({ ...s, caches: Array.from({ length: s.caches }, () => "redis"), connections: Array.from({ length: s.connections }, () => "connection"), apiNames: Array.from({ length: s.apiNames }, (_, i) => `api${i}`), now: "2000-01-02T00:00:00Z", rows: Array.from({ length: s.n }, (_, i) => ({ ...GHOST_ROW, name: `endpoint_${i}` })) });

function Summary({ d, ghost }: { d: Endpoints; ghost: boolean }) {
  const { n, t } = useT();
  if (d.total === 0) return null;
  return (
    <div aria-hidden={ghost || undefined} className={`mb-4 flex flex-wrap items-baseline gap-x-5 gap-y-1 text-[13px] text-muted ${ghost ? "sk-ghost" : ""}`}>
      <span>{n("endpoints.summary.endpoints", d.total)}</span>
      <span>{n("endpoints.summary.apis", d.apis)}</span>
      {d.fresh > 0 && (
        <span className="inline-flex items-baseline gap-1.5">
          <FreshnessMark status="on_time" />
          {n("endpoints.summary.fresh", d.fresh)}
        </span>
      )}
      {d.stale > 0 && (
        <span className="inline-flex items-baseline gap-1.5">
          <FreshnessMark status="stale" />
          {n("endpoints.summary.stale", d.stale)}
        </span>
      )}
      {d.caches.map((c) => (
        <span key={c} className="text-faint">
          {t("endpoints.summary.cache", { type: c })}
        </span>
      ))}
      {d.connections.map((c) => (
        <span key={c} className="text-faint">
          {t("endpoints.summary.connection", { name: c })}
        </span>
      ))}
    </div>
  );
}

// The bar (see ui/filters): drawn at once from the last shape seen; only the counts wait. The three states of
// freshness are the primary choice; the API (a list that grows with the manifests) is the dropdown.
function EndpointsBar({ filter, d, ghost }: { filter: Filter; d: Endpoints; ghost: boolean }) {
  const { t, n } = useT();
  const { number } = useFormat();
  const navigate = useNavigate();
  const [text, setText] = useUrlSearch("q");
  const go = (patch: Record<string, string>) => void navigate(filter.hrefWith(patch));
  const api = filter.get("api");
  const state = filter.get("state");
  const q = filter.get("q");
  // The mark's own label would be read before the word that already names it.
  const dot = (s: string) => <span aria-hidden="true" className="inline-flex"><FreshnessMark status={s} /></span>;
  const filters: FilterDef[] = d.apiNames.length > 1
    ? [{
        id: "api", label: t("endpoints.col.api"), allLabel: t("endpoints.filter.api.all"), mono: true, pending: ghost, searchLabel: t("endpoints.col.api"),
        options: d.apiNames.map((a) => ({ key: a, label: a, mono: true })),
        value: api ? [api] : [], onChange: (v) => go({ api: v[0] ?? "" }),
      }]
    : [];
  const chips: Chip[] = api ? [{ id: "api", group: t("endpoints.col.api"), value: api, mono: true, onRemove: () => go({ api: "" }) }] : [];
  return (
    <FilterBar
      search={<SearchField value={text} onChange={setText} label={t("endpoints.search")} />}
      primary={
        <Segmented
          label={t("endpoints.col.status")}
          pending={ghost}
          value={state}
          onChange={(s) => go({ state: s })}
          items={[
            { key: "", label: t("endpoints.filter.state.all"), count: d.total },
            { key: "fresh", label: t("endpoints.filter.state.fresh"), icon: dot("on_time"), count: d.fresh },
            { key: "stale", label: t("endpoints.filter.state.stale"), icon: dot("stale"), count: d.stale },
          ]}
        />
      }
      filters={filters}
      chips={chips}
      clearable={filter.any}
      onClearAll={() => go({ api: "", state: "", q: "" })}
      count={<ResultCount pending={ghost} shown={d.rows.length} tail={<>{n("endpoints.count", d.total, { count: number(d.total) })}{q && <> {t("workflows.count.search", { q })}</>}</>} />}
    />
  );
}

function Endpoints({ filter }: { filter: Filter }) {
  const { t } = useT();
  const qs = new URLSearchParams();
  for (const f of FIELDS) if (filter.get(f)) qs.set(f, filter.get(f));
  const q = useData<Endpoints>(["endpoints", qs.toString()], `/api/endpoints${qs.size ? `?${qs}` : ""}`);
  const ghost = !q.data;
  // Only the unfiltered answer is remembered: a filter's counts are not the list's shape.
  const seen = useSeen("endpoints", qs.size === 0 && q.data ? shapeOf(q.data) : undefined, FIRST);
  const d = q.data ?? ghostOf(seen);
  const none = d.rows.length === 0;
  return (
    <>
      {/* Only where there is something listed — and, while the first answer is on its way, where the
          remembered shape says there was (ghostOf): drawing the band for everyone and removing it for an
          empty list moved the whole page up by its height. */}
      {d.total > 0 && <EndpointsStrip />}
      <Summary d={d} ghost={ghost} />
      {d.total > 0 && (
        <EndpointsBar filter={filter} d={d} ghost={ghost} />
      )}
      <div aria-busy={ghost}>
        {q.isError ? (
          <p role="alert" className="rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">
            <ErrorText error={q.error} />
          </p>
        ) : none ? (
          <div aria-hidden={ghost || undefined} className={ghost ? "sk-ghost" : undefined}>
            <Empty msg={d.total > 0 ? t("endpoints.empty.filtered") : t("endpoints.empty")} hint={d.total > 0 ? undefined : "brevis endpoints publish manifest.json"}>
              {d.total > 0 && (
                <Link to="/data/endpoints" className={`mt-2 inline-block text-xs ${linkUnderline}`}>
                  {t("workflows.clear")}
                </Link>
              )}
            </Empty>
          </div>
        ) : (
          <Table id="endpoint-rows" head={<Head />} ghost={ghost} memo="endpoints">
            <Rows rows={d.rows} now={d.now} />
          </Table>
        )}
      </div>
    </>
  );
}

function HowItWorks() {
  const { t } = useT();
  return (
    <HowSteps
      steps={[
        { kicker: t("endpoints.how.1"), title: t("endpoints.how.declare"), text: t("endpoints.how.declare.text") },
        {
          kicker: t("endpoints.how.2"),
          title: t("endpoints.how.publish"),
          text: (
            <>
              <code className="font-mono">brevis endpoints publish</code> {t("endpoints.how.publish.text")}
            </>
          ),
        },
        { kicker: t("endpoints.how.3"), title: t("endpoints.how.serve"), text: t("endpoints.how.serve.text") },
      ]}
    />
  );
}

export default function EndpointsRoute() {
  const { t } = useT();
  const filter = useFilter(FIELDS, "/data/endpoints");
  const { show } = useDrawer("publicar");
  return (
    <Page>
      <PageHead
        title={t("endpoints.title")}
        actions={
          <button type="button" className={btn(true)} aria-haspopup="dialog" onClick={show}>
            <Icon name="plus" className="h-3.5 w-3.5" />
            {t("endpoints.publish")}
          </button>
        }
      />
      <Lead before={t("endpoints.lead.before")} strong={t("endpoints.lead.strong")} after={t("endpoints.lead.after")} />
      <Endpoints filter={filter} />
      <HowItWorks />
      <HowDrawer
        id="publicar"
        title={t("endpoints.publish")}
        lead={t("endpoints.publish.lead")}
        blocks={[
          { title: t("endpoints.publish.declare"), text: PUBLISH_YAML },
          { title: t("endpoints.publish.commands"), text: PUBLISH_COMMANDS },
        ]}
        note={t("endpoints.publish.note")}
      />
    </Page>
  );
}
