import { useMutation } from "@tanstack/react-query";
import { EndpointPanel } from "../ui/metrics/endpoints";
import { useState, type FormEvent, type ReactNode } from "react";
import { Link, useParams } from "react-router";
import { postJSON } from "../api/client";
import { useData } from "../api/live";
import { cat, useFormat, useMsg, useT, type Cat } from "../i18n";
import { useSince } from "../ui/format";
import { useCrumb } from "../shell/nav";
import { CodeBlock } from "../ui/copy";
import { Icon } from "../ui/icon";
import { btn, Card, Empty, MethodPill, PageHead, StatCells, linkUnderline, Page } from "../ui/kit";
import { Logo } from "../ui/logo";
import { Freshness, FreshnessMark } from "../ui/status";
import { EndpointFreshness, endpointHref, kindOf, splitTarget, useParamsSaid } from "./endpoints";
import { ErrorText } from "../api/errors";
import { useErrorText } from "../api/errors";

type Lineage = {
  target: string;
  where: string;
  what: string;
  kind: string;
  known: boolean;
  status: string;
  lastAt: string | null;
  origin: { main: Cat | null; sub: Cat | null } | null;
  writer: { name: string; href: string } | null;
  sql: boolean;
  cacheTtl: Cat | null;
};
type Field = { name: string; kind: string; value: string; options: string[]; required: boolean; maxLen: number };
type Endpoint = {
  api: string;
  name: string;
  path: string;
  about: string;
  keyed: boolean;
  freshness: "stale" | "fresh" | "";
  publishedAt: string;
  stale: { reason: Cat | null; target: string } | null;
  lineage: Lineage[];
  cache: { state: "set" | "none" | "unknown"; ttl: Cat | null; type: string; addrFrom: string; reads: string[]; bump: string };
  params: { name: string; type: string; required: boolean; note: string; accepts: Cat | null; default: string }[];
  paramCount: number;
  required: number;
  limits: { timeout: Cat | null; maxRows: number; maxBytes: number };
  canRun: boolean;
  form: Field[];
  curlHead: string;
  curlBase: string;
};
type Answer = {
  status: number;
  statusText: string;
  cache: string;
  millis: number;
  received: number;
  truncated: boolean;
  columns: string[];
  rows: string[][];
  error: string;
};

// Before the first answer the page draws the last endpoint it showed (or this typical one) through the same
// markup, as a ghost (.sk-ghost): the labels are the page's own, the values are bars, and the heights are the
// loaded heights. Only the data-bound parts are ghosted; titles, labels and buttons are never.
const GHOST: Endpoint = {
  api: "",
  name: "",
  path: "/v1/endpoint_path",
  about: "",
  keyed: true,
  freshness: "fresh",
  publishedAt: "2000-01-01T00:00:00Z",
  stale: null,
  lineage: [
    {
      target: "postgres://warehouse/schema/table_name",
      where: "postgres://warehouse/schema/",
      what: "table_name",
      kind: "postgres",
      known: true,
      status: "on_time",
      lastAt: "2000-01-01T00:00:00Z",
      origin: { main: cat("Source label"), sub: cat("api.example.com") },
      writer: { name: "workflow_name", href: "" },
      sql: false,
      cacheTtl: cat("6h"),
    },
  ],
  cache: { state: "set", ttl: cat("6h"), type: "redis", addrFrom: "BREVIS_CACHE_ADDR", reads: ["postgres://warehouse/schema/table_name"], bump: "" },
  params: [
    { name: "start_date", type: "date", required: true, note: "", accepts: cat("A date"), default: "" },
    { name: "end_date", type: "date", required: true, note: "", accepts: cat("A date"), default: "" },
  ],
  paramCount: 2,
  required: 2,
  limits: { timeout: cat("30s"), maxRows: 10000, maxBytes: 0 },
  canRun: true,
  form: [
    { name: "start_date", kind: "date", value: "", options: [], required: true, maxLen: 0 },
    { name: "end_date", kind: "date", value: "", options: [], required: true, maxLen: 0 },
  ],
  curlHead: "curl ",
  curlBase: "https://api.example.com/v1/endpoint_path",
};
const gh = (ghost: boolean) => (ghost ? "sk-ghost" : "");

/** A byte count somebody can judge at a glance (the server's pages say it the same way). */
function bytesText(n: number): string {
  const units: [number, string][] = [[2 ** 40, "TB"], [2 ** 30, "GB"], [2 ** 20, "MB"], [2 ** 10, "kB"]];
  for (const [size, name] of units) if (n >= size) return `${(n / size).toFixed(1)} ${name}`;
  return `${n} B`;
}

const field = "h-8 w-full min-w-0 rounded-card border border-line bg-surface px-2.5 text-[0.82rem] text-ink focus:border-accent-strong focus:outline-none";
const required = "rounded-card bg-state-queued/13 px-1.5 py-px text-[0.68rem] font-medium tracking-[0.04em] text-state-queued uppercase";

/** The line above the path: back to every endpoint, then what it is. The method and the API are known from the address; the rest arrives. */
function Eyebrow({ api, d, ghost }: { api: string; d: Endpoint; ghost: boolean }) {
  const { t } = useT();
  const since = useSince();
  return (
    <>
      <Link to="/data/endpoints" className="text-[0.75rem] text-muted hover:text-ink">
        &larr; {t("endpoint.back")}
      </Link>
      <span className="text-faint">·</span>
      <MethodPill method="GET" />
      <span className="font-mono text-[0.75rem] text-ink-2">{api}</span>
      <span aria-hidden={ghost || undefined} className={`text-[0.72rem] text-muted ${gh(ghost)}`}>
        {t("endpoint.published", { when: since(d.publishedAt) })}
      </span>
      {d.freshness && (
        <span aria-hidden={ghost || undefined} className={`inline-flex ${gh(ghost)}`}>
          <EndpointFreshness value={d.freshness} />
        </span>
      )}
    </>
  );
}

/** Why the table is behind, in its writer's words, that the endpoint answers with the last good data meanwhile, and the table in the Catalog. */
function Stale({ s }: { s: NonNullable<Endpoint["stale"]> }) {
  const { t } = useT();
  const msg = useMsg();
  return (
    <section role="note" className="max-w-3xl rounded-card border border-state-queued/40 bg-state-queued/8 px-4 py-3 text-[13px]">
      <p className="flex items-center gap-2 font-semibold text-state-queued">
        <FreshnessMark status="stale" />
        {t("endpoint.stale.title")}
      </p>
      {s.reason && <p className="mt-1.5 text-ink-2">{msg(s.reason)}</p>}
      <p className="mt-1 text-muted">{t("endpoint.stale.serves")}</p>
      <Link to={`/data/target?u=${encodeURIComponent(s.target)}`} className={`mt-2 inline-block text-[12.5px] ${linkUnderline}`}>
        {t("endpoint.stale.catalog")} &rarr;
      </Link>
    </section>
  );
}

function Step({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="mb-0.5 text-[10.5px] font-medium tracking-[0.08em] text-faint uppercase">{label}</p>
      {children}
    </div>
  );
}
const Arrow = () => (
  <span aria-hidden="true" className="hidden pt-4 text-center text-faint min-[1100px]:block">
    &rarr;
  </span>
);

const lineageRow =
  "grid grid-cols-1 gap-3 px-4 py-3 text-[12.5px] min-[1100px]:grid-cols-[minmax(0,1fr)_1rem_minmax(0,1fr)_1rem_minmax(0,1.3fr)_1rem_minmax(0,1fr)] min-[1100px]:gap-2";

/** Where the data comes from: per table read, origin, writer, table and this endpoint; stacked on a narrow screen. */
function LineagePanel({ d, ghost }: { d: Endpoint; ghost: boolean }) {
  const { t } = useT();
  const msg = useMsg();
  const since = useSince();
  return (
    <Card title={t("endpoint.lineage")} count={ghost ? undefined : String(d.lineage.length)} id="lineage">
      <ul aria-hidden={ghost || undefined} className={`divide-y divide-line-soft ${gh(ghost)}`}>
        {d.lineage.map((l) => {
          const [where, what] = splitTarget(l.target);
          return (
            <li key={l.target} className={lineageRow}>
              <Step label={t("endpoint.lineage.origin")}>
                {l.origin ? (
                  <>
                    <p className="truncate text-ink-2">{msg(l.origin.main)}</p>
                    {msg(l.origin.sub) && (
                      <p className="truncate font-mono text-[11.5px] text-muted" title={msg(l.origin.sub)}>
                        {msg(l.origin.sub)}
                      </p>
                    )}
                  </>
                ) : (
                  <p className="text-faint">—</p>
                )}
              </Step>
              <Arrow />
              <Step label={t("endpoint.lineage.writer")}>
                {l.writer ? (
                  <Link to={l.writer.href} className="block truncate font-mono text-ink hover:underline" title={l.writer.name}>
                    {l.writer.name}
                  </Link>
                ) : (
                  <p className="text-faint">—</p>
                )}
              </Step>
              <Arrow />
              <Step label={t("endpoint.lineage.table")}>
                <Link to={`/data/target?u=${encodeURIComponent(l.target)}`} className="flex min-w-0 items-center gap-1.5 font-mono hover:underline" title={l.target}>
                  <Logo k={kindOf(l.target)} />
                  <span className="truncate text-faint">{where}</span>
                  <span className="shrink-0 text-ink">{what}</span>
                </Link>
                {l.known ? (
                  <p className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[11.5px] text-muted">
                    <Freshness status={l.status} />
                    {l.lastAt && <span>· {t("endpoint.lineage.rebuilt", { when: since(l.lastAt) })}</span>}
                  </p>
                ) : (
                  <p className="mt-0.5 text-[11.5px] text-faint">{t("endpoint.lineage.unknown")}</p>
                )}
                {l.sql && (
                  <Link to={`/sql?target=${encodeURIComponent(l.target)}`} className="mt-0.5 inline-block text-[11.5px] text-muted underline decoration-line-strong underline-offset-2 hover:text-ink">
                    {t("endpoint.lineage.sql")}
                  </Link>
                )}
              </Step>
              <Arrow />
              <Step label={t("endpoint.lineage.endpoint")}>
                <p className="truncate font-mono text-ink">GET {d.path}</p>
                {msg(l.cacheTtl) && <p className="text-[11.5px] text-muted">{t("endpoint.lineage.cache", { ttl: msg(l.cacheTtl) })}</p>}
              </Step>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

/** How long an answer is kept, what clears it, what holds it, and the command to clear it by hand. */
function CachePanel({ c, ghost }: { c: Endpoint["cache"]; ghost: boolean }) {
  const { t } = useT();
  const msg = useMsg();
  const cell = "bg-surface px-4 py-3";
  return (
    <Card title={t("endpoints.col.cache")} id="cache">
      {c.state === "set" ? (
        <div aria-hidden={ghost || undefined} className={gh(ghost)}>
          <dl className="grid grid-cols-1 gap-px bg-line-soft min-[720px]:grid-cols-3">
            <div className={cell}>
              <dt className="text-[11.5px] text-muted">{t("endpoint.cache.ttl")}</dt>
              <dd className="mt-0.5 text-[15px] font-semibold text-ink">{msg(c.ttl)}</dd>
              <dd className="text-[11.5px] text-faint">{t("endpoint.cache.ttl.hint")}</dd>
            </div>
            <div className={cell}>
              <dt className="text-[11.5px] text-muted">{t("endpoint.cache.cleared")}</dt>
              <dd className="mt-0.5 text-[15px] font-semibold text-ink">{t("endpoint.cache.cleared.value")}</dd>
              <dd className="truncate font-mono text-[11.5px] text-faint" title={c.reads.join("\n")}>
                {c.reads.join(" · ")}
              </dd>
            </div>
            <div className={cell}>
              <dt className="text-[11.5px] text-muted">{t("endpoint.cache.type")}</dt>
              <dd className="mt-0.5 text-[15px] font-semibold text-ink">{c.type === "redis" ? "Redis" : c.type}</dd>
              <dd className="font-mono text-[11.5px] text-faint">{c.addrFrom}</dd>
            </div>
          </dl>
          <div className="space-y-3 border-t border-line-soft px-4 py-3 text-[12.5px]">
            <p className="text-muted">{t("endpoint.cache.explain")}</p>
            {c.bump && (
              <div>
                <p className="mb-1.5 font-medium text-ink">{t("endpoint.cache.clear")}</p>
                <CodeBlock text={c.bump} />
                <p className="mt-1.5 text-muted">{t("endpoint.cache.clear.when")}</p>
              </div>
            )}
          </div>
        </div>
      ) : (
        <p aria-hidden={ghost || undefined} className={`px-4 py-3 text-[12.5px] text-muted ${gh(ghost)}`}>{t(c.state === "unknown" ? "endpoint.cache.unknown" : "endpoint.cache.none")}</p>
      )}
    </Card>
  );
}

const ph = "px-3 py-2 font-medium whitespace-nowrap";
const pd = "px-3 py-2.5";

function ParamsPanel({ d, ghost }: { d: Endpoint; ghost: boolean }) {
  const { t } = useT();
  const msg = useMsg();
  const paramsSaid = useParamsSaid();
  return (
    <Card title={t("endpoints.col.params")} count={ghost ? undefined : paramsSaid(d.paramCount, d.required)}>
      {d.params.length === 0 ? (
        <p aria-hidden={ghost || undefined} className={`px-4 py-3 text-sm text-muted ${gh(ghost)}`}>{t("endpoint.params.none")}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="ct w-full text-[0.82rem]">
            <thead>
              <tr className="border-b border-line bg-canvas-subtle text-left text-[11.5px] text-faint">
                <th className={`${ph} pl-4`}>{t("endpoint.params.name")}</th>
                <th className={ph}>{t("endpoint.params.type")}</th>
                <th className={ph}>{t("endpoint.params.accepts")}</th>
                <th className={`${ph} pr-4`}>{t("endpoint.params.default")}</th>
              </tr>
            </thead>
            <tbody aria-hidden={ghost || undefined} className={`divide-y divide-line-soft ${gh(ghost)}`}>
              {d.params.map((p) => (
                <tr key={p.name} className="align-top">
                  <td className={`${pd} pl-4`}>
                    <span className="font-mono text-[0.8rem] text-ink">{p.name}</span>{" "}
                    {p.required && <span className={`ml-1.5 ${required}`}>{t("endpoint.params.required")}</span>}
                    {p.note && <p className="mt-1 max-w-md text-[0.75rem] whitespace-pre-line text-muted">{p.note}</p>}
                  </td>
                  <td className={`${pd} font-mono text-[0.75rem] text-muted`}>{p.type}</td>
                  <td className={`${pd} text-[0.78rem] text-ink-2`}>{msg(p.accepts)}</td>
                  <td className={`${pd} pr-4 font-mono text-[0.75rem] text-ink`}>{p.default === "" ? <span className="text-faint">—</span> : p.default}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

function Limits({ l, ghost }: { l: Endpoint["limits"]; ghost: boolean }) {
  const { t } = useT();
  const msg = useMsg();
  const f = useFormat();
  const v = (x: string) => (
    <span aria-hidden={ghost || undefined} className={gh(ghost)}>
      {x}
    </span>
  );
  return (
    <section>
      <h2 className="mb-2 text-[0.875rem] font-semibold text-ink">{t("endpoint.limits")}</h2>
      <StatCells
        items={[
          { label: t("endpoint.limits.timeout"), icon: "clock", value: v(msg(l.timeout)) },
          { label: t("endpoint.limits.rows"), icon: "rows-3", value: v(f.number(l.maxRows)) },
          { label: t("endpoint.limits.bytes"), icon: "hard-drive", value: v(l.maxBytes > 0 ? bytesText(l.maxBytes) : t("endpoint.limits.no_ceiling")) },
        ]}
      />
    </section>
  );
}

/** The call from outside, with the key a placeholder; it follows Try it's form. */
function Curl({ d, values, ghost }: { d: Endpoint; values: Record<string, string>; ghost: boolean }) {
  const { t } = useT();
  const q = d.form
    .filter((f) => values[f.name])
    // The server's pages escape with a "+" for a space: the same curl in both.
    .map((f) => `${encodeURIComponent(f.name)}=${encodeURIComponent(values[f.name] ?? "").replace(/%20/g, "+")}`);
  const text = `${d.curlHead}'${d.curlBase}${q.length ? `?${q.join("&")}` : ""}'`;
  return (
    <Card title={t("endpoint.call")}>
      <div aria-hidden={ghost || undefined} className={`p-4 ${gh(ghost)}`}>
        <CodeBlock text={text} />
      </div>
    </Card>
  );
}

const answerBox = "rounded-card border border-line bg-canvas-subtle px-3 py-2.5 text-[0.82rem] text-ink";

function AnswerView({ a }: { a: Answer }) {
  const { t, n } = useT();
  const f = useFormat();
  const cache = a.cache === "hit" || a.cache === "miss" || a.cache === "bypass" ? t(`endpoint.try.cache.${a.cache}`) : "";
  const total = a.truncated ? `${a.received}+` : String(a.received);
  return (
    <>
      <div className="mb-2 flex flex-wrap items-center gap-2 text-[0.75rem] text-muted">
        <span className={`rounded-card px-1.5 py-px font-mono font-semibold ${a.status < 300 ? "bg-state-success/13 text-state-success" : "bg-state-failed/13 text-state-failed"}`}>
          {a.status} {a.statusText}
        </span>
        {a.cache && <span className="rounded-card border border-line px-1.5 py-px font-mono text-[0.7rem] font-semibold text-ink">{a.cache.toUpperCase()}</span>}
        <span className="tabular-nums">{f.duration(a.millis)}</span>
        {a.status === 200 && <span className="tabular-nums">{n("endpoint.try.rows", a.received, { n: total })}</span>}
      </div>
      {cache && <p className="mb-2 text-[0.72rem] text-muted">{cache}</p>}
      {a.received > a.rows.length && <p className="mb-2 text-[0.72rem] text-muted">{t("endpoint.try.part", { shown: a.rows.length, n: a.received })}</p>}
      {a.error ? (
        <p className={answerBox}>{a.error}</p>
      ) : a.rows.length === 0 ? (
        <p className="text-[0.82rem] text-muted">{t("endpoint.try.no_rows")}</p>
      ) : (
        <div className="max-h-[28rem] overflow-auto rounded-card border border-line-soft" tabIndex={0} role="region" aria-label={n("endpoint.try.rows", a.rows.length)}>
          <table className="ct w-full text-sm">
            <thead className="border-b border-line-soft text-left text-xs text-muted">
              <tr>
                {a.columns.map((c) => (
                  <th key={c} scope="col" className="px-3 py-2 font-medium whitespace-nowrap">
                    {c}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {a.rows.map((row, i) => (
                <tr key={i}>
                  {row.map((cell, j) => (
                    <td key={j} className="px-3 py-2 text-xs whitespace-nowrap">
                      {cell}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

/** Try it: a POST of the parameters; nothing is asked of the API until Run is pressed. */
function TryIt({ d, values, setValues, ghost }: { d: Endpoint; values: Record<string, string>; setValues: (v: Record<string, string>) => void; ghost: boolean }) {
  const { t } = useT();
  const run = useMutation({
    mutationFn: () => postJSON<Answer>(`/api/endpoints/${encodeURIComponent(d.api)}/${encodeURIComponent(d.name)}/call`, { params: values }),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    run.mutate();
  };
  return (
    <Card title={t("endpoint.try")}>
      <div className="p-4">
        <p className="mb-3 text-[0.75rem] text-muted">{t("endpoint.try.lead")}</p>
        <form className="flex flex-col gap-3" onSubmit={submit} onKeyDown={(e) => (e.metaKey || e.ctrlKey) && e.key === "Enter" && run.mutate()}>
          <div aria-hidden={ghost || undefined} className={`grid grid-cols-1 gap-3 sm:grid-cols-2 min-[1180px]:grid-cols-1 ${gh(ghost)}`}>
            {d.form.map((f) => (
              <label key={f.name} className="flex min-w-0 flex-col gap-1">
                <span className="flex items-center gap-1.5 font-mono text-[0.75rem] text-ink-2">
                  {f.name}
                  {f.required && <span className={`${required} font-sans`}>{t("endpoint.params.required")}</span>}
                </span>
                {f.kind === "select" ? (
                  <select className={field} value={values[f.name] ?? ""} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}>
                    <option value="">{t("endpoint.try.default")}</option>
                    {f.options.map((o) => (
                      <option key={o} value={o}>
                        {o}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    className={field}
                    type={f.kind}
                    value={values[f.name] ?? ""}
                    maxLength={f.maxLen > 0 ? f.maxLen : undefined}
                    onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}
                  />
                )}
              </label>
            ))}
          </div>
          <button type="submit" className={`${btn(true)} self-start`} disabled={run.isPending || ghost}>
            <Icon name="play" className="h-3.5 w-3.5" />
            {t("endpoint.try.run")}
            <span className="ml-1 font-mono text-[0.7rem] opacity-60">⌘↵</span>
          </button>
        </form>
        <div aria-live="polite" aria-busy={run.isPending} className="mt-4 transition-opacity aria-busy:opacity-50">
          {run.isError ? (
            <p className={answerBox}><ErrorText error={run.error} /></p>
          ) : run.data ? (
            <AnswerView a={run.data} />
          ) : (
            <p className="text-[0.78rem] text-faint">{t("endpoint.try.empty")}</p>
          )}
        </div>
      </div>
    </Card>
  );
}

function EndpointPage({ api, name, d, ghost }: { api: string; name: string; d: Endpoint; ghost: boolean }) {
  const { t } = useT();
  // Try it starts from each parameter's default; the curl follows what is typed.
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(d.form.map((f) => [f.name, f.value])));
  const side = d.canRun;
  return (
    <Page>
      <PageHead mono eyebrow={<Eyebrow api={api} d={d} ghost={ghost} />} title={<span className={gh(ghost)}>{d.path}</span>} />
      <div className={`grid grid-cols-1 gap-3 ${side ? "min-[1180px]:grid-cols-[minmax(0,1fr)_360px]" : ""}`}>
        <div className="flex min-w-0 flex-col gap-3">
          {d.about && (
            <p aria-hidden={ghost || undefined} className={`max-w-3xl text-[13.5px] whitespace-pre-line text-ink-2 ${gh(ghost)}`}>
              {d.about}
            </p>
          )}
          {d.stale && (
            <div aria-hidden={ghost || undefined} className={gh(ghost)}>
              <Stale s={d.stale} />
            </div>
          )}
          <div aria-hidden={ghost || undefined} className={`flex max-w-3xl items-start gap-2.5 rounded-card border border-line bg-surface px-3.5 py-3 text-[0.82rem] text-ink-2 ${gh(ghost)}`}>
            {d.keyed ? (
              <>
                <span className="mt-0.5 text-ink">
                  <Icon name="key-round" className="h-4 w-4" />
                </span>
                <span>{t("endpoint.tenant.key")}</span>
              </>
            ) : (
              <>
                <span className="mt-0.5 text-state-running">
                  <Icon name="info" className="h-4 w-4" />
                </span>
                <span>{t("endpoint.tenant.none")}</span>
              </>
            )}
          </div>
          <EndpointPanel api={api} name={name} />
          <LineagePanel d={d} ghost={ghost} />
          <CachePanel c={d.cache} ghost={ghost} />
          <ParamsPanel d={d} ghost={ghost} />
          <Limits l={d.limits} ghost={ghost} />
          {!side && <Curl d={d} values={values} ghost={ghost} />}
        </div>
        {side && (
          <div className="flex min-w-0 flex-col gap-3 min-[1180px]:sticky min-[1180px]:top-4 min-[1180px]:max-h-[calc(100vh-2rem)] min-[1180px]:self-start min-[1180px]:overflow-y-auto">
            <TryIt d={d} values={values} setValues={setValues} ghost={ghost} />
            <Curl d={d} values={values} ghost={ghost} />
          </div>
        )}
      </div>
    </Page>
  );
}

export default function EndpointRoute() {
  const { t } = useT();
  const errText = useErrorText();
  const { api = "", name = "" } = useParams();
  const q = useData<Endpoint>(["endpoint", api, name], `/api/endpoints/${encodeURIComponent(api)}/${encodeURIComponent(name)}`, { keep: false });
  useCrumb(q.data?.path ?? "", { mono: true });
  if (q.isError)
    return (
      <Page>
        <div className="pt-3">
          <Empty msg={errText(q.error)}>
            <Link to="/data/endpoints" className={`mt-2 inline-block text-xs ${linkUnderline}`}>
              &larr; {t("endpoint.back")}
            </Link>
          </Empty>
        </div>
      </Page>
    );
  // The key resets Try it when another endpoint opens; a poll keeps what was typed.
  return <EndpointPage key={q.data ? endpointHref(q.data) : "loading"} api={api} name={name} d={q.data ?? GHOST} ghost={!q.data} />;
}
