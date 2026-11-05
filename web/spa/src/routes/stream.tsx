import { useState, type ReactNode } from "react";
import { StreamPanel } from "../ui/metrics/streams";
import { Link, useParams } from "react-router";
import { ApiError } from "../api/client";
import { useData } from "../api/live";
import type { Stream } from "../api/ingest";
import { useT } from "../i18n";
import { useBrief } from "../ui/format";
import { useCrumb } from "../shell/nav";
import { ConfigNote, DeadLetterPanel } from "../ui/cfg";
import { CopyButton } from "../ui/copy";
import { Drawer, useDrawer } from "../ui/drawer";
import { checkEvent } from "../ui/test-event";
import { FlowDiagram } from "../ui/flow";
import { HashTabs } from "../ui/hash-tabs";
import { Icon } from "../ui/icon";
import { btn, ButtonLink, Card, Empty, PageHead, linkUnderline, Page } from "../ui/kit";
import { Logo, useLogoTitle } from "../ui/logo";
import { HealthStatus, useHealthReason, useHealthSince } from "../ui/health";
import { TOPIC_STREAMS } from "../ui/metrics/hooks";
import "./streams.css";

const pill = "inline-flex items-center gap-1.5 rounded-card border border-line px-2 py-0.5 text-ink-2";
// The tab names the panel; the server hides this heading when the tabs are there.
const h2 = "mb-3 hidden text-[13px] font-semibold text-ink";

// Before the first answer the page draws the last stream it showed (or this typical one) through the same
// markup, as a ghost (.sk-ghost): the words are the page's own, the values are bars, and the heights are the
// loaded heights. Only the data-bound parts are ghosted; titles, labels, tabs and buttons are never.
const GHOST_PLACE = { uri: "postgres://warehouse/schema/table_name", kind: "postgres", kindTitle: "PostgreSQL", where: "postgres://warehouse/schema/", what: "table_name", note: "" };
const GHOST: Stream = {
  gateway: "",
  path: "",
  sink: GHOST_PLACE,
  deadLetter: { ...GHOST_PLACE, kind: "file", uri: "file:///var/dead-letter/stream/" },
  publishedAt: "2000-01-01T00:00:00Z",
  format: "json",
  lede: "Takes json events over HTTP, batches them and writes them to PostgreSQL. What the destination refuses goes to the dead letter.",
  summary: "JSON events posted to the path land in a Postgres table, merged by ingestion id so a redelivery is ignored, in batches of 100 events; batches the destination refuses go to files.",
  flow: {
    nodes: [
      { kind: "source", main: "POST /v1/stream", detail: "json" },
      { kind: "buffer", main: "", detail: "every 1 second or 100 events" },
      { kind: "destination", main: "postgres://warehouse/schema/table_name", detail: "postgres · merge" },
    ],
    deadLetter: { kind: "dead_letter", main: "file:///var/dead-letter/stream/", detail: "refused batches" },
  },
  tables: [],
  catalogHref: "",
  query: "",
  readBy: [],
  hasConfig: true,
  cards: [
    { key: "intake", title: "Intake", rows: [{ path: "name", label: "Name", value: "stream", default: false, note: "" }, { path: "path", label: "Path", value: "/v1/stream", default: false, note: "" }] },
    { key: "buffer", title: "Buffer", rows: [{ path: "buffer.flush.every", label: "Flush every", value: "1s", default: true, note: "" }] },
  ],
  yaml: "",
  changes: [],
  deadLetterPanel: null,
  buriedMetric: "brevis_gateway_buried_total",
  testEvent: { format: "json", body: "{}", curlHead: "", lands: "", table: "", allow: [], dataset: "" },
};
const gh = (ghost: boolean) => (ghost ? "sk-ghost" : "");

function Strip({ gateway, v, ghost }: { gateway: string; v: Stream; ghost: boolean }) {
  const { t } = useT();
  const brief = useBrief();
  const kind = useLogoTitle();
  const reason = useHealthReason();
  const since = useHealthSince()(v.health);
  return (
    <div className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-card border border-line bg-surface px-4 py-3 text-[12.5px] text-muted">
      {/* Health has a row of its own, so the width of its word (healthy, degraded) never changes how the rest wraps. */}
      <div className="stp-health flex flex-wrap items-baseline gap-x-3">
        <HealthStatus health={v.health} />
        <p data-testid="health-reason" aria-hidden={ghost || undefined} className={`text-[12.5px] text-muted ${gh(ghost)}`}>
          {reason(v.health)}
          {since && <span className="ml-1.5 whitespace-nowrap text-faint">· {since}</span>}
          {v.health?.recovering && <span className="ml-1.5 text-faint">· {t("streams.health.recovering")}</span>}
        </p>
      </div>
      <span>{t("streams.page.from")}</span>
      <span className={pill}>
        <Logo k="gateway" />
        {t("streams.gateway")}
        <span className="font-mono text-ink">{gateway}</span>
      </span>
      <span>{t("streams.page.to")}</span>
      {
        <>
          <span aria-hidden={ghost || undefined} className={`${pill} min-w-0 ${gh(ghost)}`}>
            {v.sink.kind !== "" && (
              <>
                <Logo k={v.sink.kind} />
                {kind(v.sink.kind)}
              </>
            )}
            <span className="truncate font-mono text-ink" title={v.sink.uri}>
              {v.sink.uri || t("streams.unidentified")}
            </span>
          </span>
          {v.format !== "" && <span className={gh(ghost)}>{t("streams.page.format", { format: v.format })}</span>}
          {v.publishedAt && <span className={gh(ghost)}>{t("streams.page.published", { at: `${brief(v.publishedAt)} UTC` })}</span>}
        </>
      }

    </div>
  );
}

function Overview({ path, v, ghost }: { path: string; v: Stream; ghost: boolean }) {
  const { t } = useT();
  return (
    <section className="mb-3">
      <h2 className={h2}>{t("streams.tab.overview")}</h2>
      <div aria-hidden={ghost || undefined} className={`overflow-hidden rounded-card border border-line bg-surface ${gh(ghost)}`}>{v.flow ? <FlowDiagram flow={v.flow} /> : null}</div>
      <div className="mt-3 grid grid-cols-1 gap-3 min-[1180px]:grid-cols-[minmax(0,1fr)_360px]">
        <Card title={t("streams.page.sentence")}>
          <div className="px-4 py-3">
            <p aria-hidden={ghost || undefined} className={`text-[13.5px] text-ink ${gh(ghost)}`}>{v.summary}</p>
            <p className="mt-3 border-t border-line-soft pt-3 text-[12px] text-muted">{t("streams.page.metrics")}</p>
          </div>
        </Card>
        <Card title={t("streams.page.next")}>
          <ol className="space-y-2 px-4 py-3 text-[12.5px]">
            <li>
              <span className="inline-flex items-center gap-1.5 rounded-card border border-line px-2 py-0.5 font-mono text-ink">
                <span className="font-sans text-[10.5px] tracking-[0.08em] text-muted uppercase">{t("config.section.stream")}</span>
                {path}
              </span>
            </li>
            <li className="pl-2 text-muted">&darr; {t("streams.page.writes")}</li>
            <li>
              {
                <div aria-hidden={ghost || undefined} className={gh(ghost)}>
                  <span className="inline-flex max-w-full flex-wrap items-center gap-1.5 rounded-card border border-line px-2 py-0.5 font-mono text-ink">
                    {v.sink.kind !== "" && <Logo k={v.sink.kind} />}
                    {v.tables.length > 0 ? v.tables.join(" · ") : <span className="break-all">{v.sink.uri || t("streams.unidentified")}</span>}
                  </span>
                  {v.catalogHref !== "" && (
                    <Link to={v.catalogHref} className="mt-1 block text-[12px] text-ink-2 hover:text-ink hover:underline">
                      {t("loads.drawer.catalog")} &rarr;
                    </Link>
                  )}
                </div>
              }
            </li>
            <li className="pl-2 text-muted">&darr; {t("streams.page.read_by")}</li>
            <li className={gh(ghost)} aria-hidden={ghost || undefined}>
              {v.readBy.length === 0 ? (
                <span className="text-faint">{t("streams.page.read_by.none")}</span>
              ) : (
                <ul className="space-y-1">
                  {v.readBy.map((e) => (
                    <li key={e.href}>
                      <Link to={e.href} className="font-mono text-ink hover:underline">
                        GET {e.path}
                      </Link>{" "}
                      <span className="ml-1.5 text-muted">{e.api}</span>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          </ol>
        </Card>
      </div>
    </section>
  );
}

function Config({ v, ghost }: { v: Stream; ghost: boolean }) {
  const { t } = useT();
  const brief = useBrief();
  const [view, setView] = useState<"explained" | "yaml">("explained");
  const toggle = (k: "explained" | "yaml", label: string) => (
    <button
      type="button"
      aria-pressed={view === k}
      onClick={() => setView(k)}
      className="rounded-[5px] px-2.5 py-1 text-[12.5px] text-muted aria-pressed:bg-surface-2 aria-pressed:text-ink"
    >
      {label}
    </button>
  );
  return (
    <section className="mb-3">
      <h2 className={h2}>{t("streams.tab.config")}</h2>
      {!v.hasConfig ? (
        <p className="rounded-card border border-line bg-surface px-4 py-3 text-xs text-muted">{t("streamcfg.v1")}</p>
      ) : (
        <div aria-hidden={ghost || undefined} className={gh(ghost)}>
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <p className="text-[12.5px] text-muted">{t("streamcfg.lead")}</p>
            <div role="group" aria-label={t("streams.tab.config")} className="inline-flex h-8 items-center gap-0.5 rounded-card border border-line bg-surface p-0.5">
              {toggle("explained", t("streamcfg.explained"))}
              {toggle("yaml", "YAML")}
            </div>
          </div>
          {view === "explained" ? (
            <div className="grid grid-cols-1 gap-3 min-[900px]:grid-cols-2">
              {v.cards.map((c) => (
                <Card key={c.key} title={c.title}>
                  <dl className="divide-y divide-line-soft">
                    {c.rows.map((r) => (
                      <div key={r.path} data-explained={r.path} className="grid grid-cols-[minmax(0,11rem)_minmax(0,1fr)] gap-x-4 px-4 py-2.5 text-[12.5px]">
                        <dt>
                          <span className="text-ink-2">{r.label}</span>
                          <span className="mt-0.5 block font-mono text-[11px] text-faint">{r.path}</span>
                        </dt>
                        <dd className="min-w-0">
                          <span className="font-mono break-all text-ink">{r.value}</span>{" "}
                          {r.default && (
                            <span className="ml-2 rounded-card border border-line px-1 py-px text-[0.65rem] text-muted uppercase" title={t("config.default.tip")}>
                              {t("config.default")}
                            </span>
                          )}
                          {r.note !== "" && (
                            <div className="mt-1">
                              <ConfigNote text={r.note} />
                            </div>
                          )}
                        </dd>
                      </div>
                    ))}
                  </dl>
                </Card>
              ))}
            </div>
          ) : (
            <section>
              <div className="mb-1.5 flex items-center justify-between">
                <h3 className="text-[12.5px] text-muted">{t("streamcfg.yaml")}</h3>
                <CopyButton text={v.yaml} label={t("loads.drawer.copy")} />
              </div>
              <pre className="overflow-x-auto rounded-card border border-line bg-canvas px-4 py-3 font-mono text-[12px] text-ink-2">
                <code>{v.yaml}</code>
              </pre>
              <p className="mt-1.5 text-[11.5px] text-faint">{t("catalog.drawer.credentials")}</p>
            </section>
          )}
          {v.changes.length > 0 && (
            <p className="mt-3 text-[12px] text-muted">
              {t("streamcfg.history")}{" "}
              {v.changes.map((c, i) => (
                <span key={c.at}>
                  {i > 0 && <span className="text-faint"> · </span>}
                  {brief(c.at)} UTC — {c.text}
                </span>
              ))}
            </p>
          )}
        </div>
      )}
    </section>
  );
}

function DeadLetter({ v, ghost }: { v: Stream; ghost: boolean }) {
  const { t } = useT();
  const dl = v.deadLetter;
  const uri = dl ? dl.uri || t("streams.unidentified") : "";
  let body: ReactNode;
  if (!dl) body = <p className="rounded-card border border-line bg-surface px-4 py-3 text-sm text-muted">{t("deadletter.none")}</p>;
  else
    body = (
      <div aria-hidden={ghost || undefined} className={`flex flex-col gap-4 ${gh(ghost)}`}>
        <Card title={t("deadletter.where")} count={dl.kind}>
          <div className="flex items-start gap-3 px-4 py-3">
            <pre className="min-w-0 flex-1 overflow-x-auto font-mono text-[0.8rem] break-all whitespace-pre-wrap text-ink">{uri}</pre>
            <CopyButton text={uri} label={t("loads.drawer.copy")} />
          </div>
          {dl.note !== "" && <p className="px-4 pb-3 text-[12px] text-muted">{dl.note}</p>}
          <p className="border-t border-line-soft px-4 py-3 text-[0.75rem] text-muted">
            {t("deadletter.metric")} <code className="font-mono text-ink-2">{v.buriedMetric}</code>
          </p>
        </Card>
        {v.deadLetterPanel ? (
          <div className="rounded-card border border-line bg-surface">
            <DeadLetterPanel dl={v.deadLetterPanel} />
          </div>
        ) : (
          <p className="rounded-card border border-line bg-surface px-4 py-3 text-xs text-muted">{t("deadletter.no_config")}</p>
        )}
        <p className="text-[12px] text-faint">{t("deadletter.count_later")}</p>
      </div>
    );
  return (
    <section className="mb-3">
      <h2 className={h2}>{t("streams.tab.dead_letter")}</h2>
      {body}
    </section>
  );
}

const STATUS_HUE = { failed: "text-state-failed", queued: "text-state-queued", success: "text-state-success" };

// The example event in the stream's format and the curl that would send it.
// It sends nothing: the console holds no token for the gateway. The body is
// editable and the curl follows it. (The old drawer also validated the body
// against the format; that check is not carried over.)

function TestEvent({ path, v, ghost }: { path: string; v: Stream; ghost: boolean }) {
  const { t } = useT();
  const { show } = useDrawer("test-event");
  const [edited, setEdited] = useState<string | null>(null);
  const body = edited ?? v.testEvent.body;
  const curl = ghost ? "" : `${v.testEvent.curlHead}\n${body.replace(/\s+$/, "")}\nEOF`;
  const status = ghost ? null : checkEvent(body, v.testEvent, t);
  return (
    <>
      <button type="button" className={btn(true)} aria-haspopup="dialog" disabled={ghost} onClick={show}>
        <Icon name="send" className="h-3.5 w-3.5" />
        {t("streams.page.test")}
      </button>
      <Drawer
        id="test-event"
        width={520}
        bodyClass=""
        title={t("testevent.title")}
        lead={t("testevent.lead")}
        kicker={
          <p className="truncate font-mono text-[11px] tracking-[0.08em] text-muted uppercase">
            POST {path} · {v.testEvent.format}
          </p>
        }
      >
        <label className="flex flex-col gap-1.5">
          <span className="text-[12.5px] font-medium text-ink">{t("testevent.body")}</span>
          <textarea
            rows={8}
            spellCheck={false}
            value={body}
            onChange={(e) => setEdited(e.target.value)}
            className="w-full rounded-card border border-line bg-canvas px-3 py-2 font-mono text-[0.78rem] text-ink focus:border-accent-strong focus:outline-none"
          />
        </label>
        <p className={`text-[12px] ${status ? STATUS_HUE[status.kind] : "text-muted"}`} aria-live="polite">{status?.text}</p>
        <div>
          <div className="mb-1.5 flex items-center justify-between">
            <span className="text-[12.5px] text-muted">{t("testevent.send_with")}</span>
            <CopyButton text={curl} label={t("testevent.copy")} />
          </div>
          <pre className="overflow-x-auto rounded-card border border-line-soft bg-canvas px-3 py-2 font-mono text-[0.72rem] text-ink-2">
            <code>{curl}</code>
          </pre>
        </div>
        <p className="text-[12px] text-muted">{t("testevent.note", { sink: v.sink.uri || t("streams.unidentified") })}</p>
      </Drawer>
    </>
  );
}

export default function StreamRoute() {
  const { t } = useT();
  // The stream's path is the tail of the address: /ingest/streams/<gateway>/<path...>.
  const { gateway = "", "*": rest = "" } = useParams();
  const path = `/${rest}`;
  useCrumb(`POST ${path}`, { mono: true });
  const q = useData<Stream>(["ingest", "stream", gateway, rest], `/api/ingest/streams/${encodeURIComponent(gateway)}/${rest.split("/").map(encodeURIComponent).join("/")}`, {
    topic: ["ingest", TOPIC_STREAMS],
    keep: false,
  });
  const ghost = !q.data;
  const v = q.data ?? GHOST;
  if (q.error instanceof ApiError && q.error.status === 404)
    return (
      <Page>
        <div className="pt-3">
          <Empty msg={t("streams.empty.filtered")}>
            <Link to="/ingest/streams" className={`mt-2 inline-block text-xs ${linkUnderline}`}>
              {t("nav.streams")}
            </Link>
          </Empty>
        </div>
      </Page>
    );
  return (
    <Page>
      <PageHead
        mono
        title={`POST ${path}`}
        eyebrow={
          <>
            <span className="text-[0.7rem] font-medium tracking-[0.08em] text-muted uppercase">{t("streams.page.kicker")}</span>
            <span className="font-mono text-[0.75rem] text-ink-2">{gateway}</span>
          </>
        }
        actions={
          <>
            {v.query !== "" && (
              <span aria-hidden={ghost || undefined} className={`inline-flex ${gh(ghost)}`}>
                <ButtonLink to={v.query} icon="terminal">
                  {t("loads.drawer.query")}
                </ButtonLink>
              </span>
            )}
            <TestEvent path={path} v={v} ghost={ghost} />
          </>
        }
      />
      {q.isError && (
        <p role="alert" className="mb-3 rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">
          {(q.error as Error).message}
        </p>
      )}
      <div aria-hidden={ghost || undefined} className={`mb-4 max-w-3xl text-[13.5px] text-muted ${gh(ghost)}`}>{v.lede}</div>
      <Strip gateway={gateway} v={v} ghost={ghost} />
      <div className="mb-4">
        <StreamPanel gateway={gateway} path={path} />
      </div>
      <HashTabs
        id="stream-tabs"
        label={t("streams.page.tabs")}
        tabs={[
          { key: "overview", label: t("streams.tab.overview"), panel: <Overview path={path} v={v} ghost={ghost} /> },
          { key: "config", label: t("streams.tab.config"), panel: <Config v={v} ghost={ghost} /> },
          { key: "deadletter", label: t("streams.tab.dead_letter"), panel: <DeadLetter v={v} ghost={ghost} /> },
        ]}
      />
    </Page>
  );
}
