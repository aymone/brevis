import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useData } from "../api/live";
import type { Place, Streams, StreamRow } from "../api/ingest";
import { cat, useFormat, useMsg, useT } from "../i18n";
import { useDrawer } from "../ui/drawer";
import { HowDrawer, HowSteps } from "../ui/how";
import { Icon } from "../ui/icon";
import { btn, Empty, Lead, MethodPill, PageHead, linkUnderline, Page } from "../ui/kit";
import { Logo, useLogoTitle } from "../ui/logo";
import { FilterBar, ResultCount, SearchField, useUrlSearch, type Chip, type FilterDef } from "../ui/filters";
import { TOPIC_STREAMS } from "../ui/metrics/hooks";
import { StreamRowRate, StreamsStrip } from "../ui/metrics/streams";
import { recall, remember, useSeen } from "../ui/seen";
import { HEALTH_ORDER, HealthStatus, severity } from "../ui/health";
import { Status, useStatusWord } from "../ui/status";
import { Row, Table, Td, Th } from "../ui/table";
import { useFilter } from "../ui/use-filter";
import "./streams.css";
import { ErrorText } from "../api/errors";

const FIELDS = ["kind", "q"] as const;
type Filter = ReturnType<typeof useFilter>;

// The example the new-stream drawer shows: a stream as the gateway's file declares one, and how it reaches the console.
const NEW_YAML = `streams:
  #: What this stream is for, shown under its path.
  - name: orders
    path: /v1/orders
    format: json
    identity:
      provider: acme
      entity: order
      source_key: order_id
      record_ts: created_at
    buffer:
      flush:
        every: 1m
        records: 2000
    sink:
      type: bigquery
      project: acme-prod
      dataset: bronze
      table: orders
    dead_letter:
      type: files
      path: gs://acme-dead-letter/orders/`;
const NEW_COMMANDS = `gateway describe -out gateway.json gateway.yaml
brevis gateway publish gateway.json`;

function Head() {
  const { t } = useT();
  return (
    <>
      <Th edge="first">{t("streams.col.stream")}</Th>
      <Th>{t("streams.kind")}</Th>
      <Th>{t("streams.col.batch")}</Th>
      <Th>{t("streams.col.status")}</Th>
      <Th edge="last" cls="w-8" />
    </>
  );
}

// The description is the manifest's comment, a line of markdown that can stop mid-sentence: its first sentence
// is the short line (the whole of it stays in the tooltip), and its `code` spans are code, never backticks.
const firstSentence = (text: string) => /^.*?[.!?](?=\s|$)/.exec(text)?.[0] ?? text;
const inlineCode = (text: string) => firstSentence(text).split("`").map((part, i) => (i % 2 ? <code key={i}>{part}</code> : part));

/** A destination: its mark and name, and under it the URI, which gives way in the middle (the object at the end stays). */
function Destination({ p }: { p: Place }) {
  const { t } = useT();
  const kindName = useLogoTitle();
  const title = p.uri === "" ? p.note || t("streams.unidentified") : p.note ? `${p.uri} — ${p.note}` : p.uri;
  return (
    <div title={title}>
      <span className="stl-kind">
        {p.kind !== "" && <Logo k={p.kind} />}
        <span>{p.kind !== "" ? kindName(p.kind) : t("streams.unidentified")}</span>
      </span>
      {p.kind === "" ? null : p.uri === "" ? (
        <p className="stl-uri">{t("streams.unidentified")}</p>
      ) : (
        <p className="stl-uri">
          <span>{p.where}</span>
          <span>{p.what}</span>
        </p>
      )}
    </div>
  );
}

// "2m · 2,000": the declared values read the same in every language; the sentence stays as the tooltip and the spoken label.
function useCompactBatch() {
  const { number } = useFormat();
  const msg = useMsg();
  return (s: StreamRow) => {
    const count = s.batchRecords == null ? "" : number(s.batchRecords);
    if (s.batchEvery === undefined) return msg(s.batch);
    return s.batchEvery && count ? `${s.batchEvery} · ${count}` : s.batchEvery || (count ? `≤ ${count}` : "");
  };
}

function Rows({ rows, gateway }: { rows: StreamRow[]; gateway: boolean }) {
  const { t } = useT();
  const msg = useMsg();
  const navigate = useNavigate();
  const kindName = useLogoTitle();
  const compact = useCompactBatch();
  return (
    <>
      {rows.map((s) => {
        const batch = compact(s);
        return (
          <Row key={s.href} onOpen={() => navigate(s.href)}>
            <Td edge="first" cls="stl-fill">
              <div className="mx-line">
                <Link to={s.href} className="stl-link inline-flex items-center gap-2 font-mono text-[13px]">
                  <MethodPill method="POST" />
                  <span className="stl-path">{s.path}</span>
                </Link>
                <StreamRowRate gateway={s.gateway} path={s.path} />
              </div>
              <p className="stl-sub" title={s.note ? `${s.gateway} — ${s.note.replaceAll("`", "")}` : s.gateway}>
                {gateway && (
                  <>
                    <span className="stl-gw">{s.gateway}</span>
                    {s.note && " · "}
                  </>
                )}
                {inlineCode(s.note)}
              </p>
            </Td>
            <Td cls="stl-fill-dest">
              <Destination p={s.sink} />
            </Td>
            <Td>
              {batch ? (
                <span className="stl-batch" title={msg(s.batch)}>
                  <span aria-hidden="true">{batch}</span>
                  <span className="sr-only">{msg(s.batch)}</span>
                </span>
              ) : (
                <span className="text-faint">—</span>
              )}
            </Td>
            <Td>
              <div className="stl-state">
                <HealthStatus health={s.health} />
                {s.deadLetter && (
                  <span className="stl-dl" title={`${s.deadLetter.kind ? kindName(s.deadLetter.kind) : ""} ${s.deadLetter.uri || t("streams.unidentified")}`.trim()}>
                    {t("streams.col.dead_letter")}
                    <span className="sr-only">: {s.deadLetter.kind ? kindName(s.deadLetter.kind) : t("streams.unidentified")}</span>
                  </span>
                )}
              </div>
            </Td>
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
type Shape = { n: number; total: number; gateways: number; withDeadLetter: number; kinds: number };
const FIRST: Shape = { n: 2, total: 2, gateways: 1, withDeadLetter: 1, kinds: 2 };
const shapeOf = (v: Streams): Shape => ({ n: v.rows.length, total: v.total, gateways: v.gateways, withDeadLetter: v.withDeadLetter, kinds: v.kinds.length });
const PLACE = { uri: "", kind: "postgres", kindTitle: "", where: "postgres://warehouse/", what: "table_name", note: "" };
const GHOST_ROW: StreamRow = { gateway: "events_gateway", path: "/v1/stream_path", href: "", note: "What this stream is for", sink: PLACE, batch: cat("1 second or 500"), batchEvery: "1s", batchRecords: 500, deadLetter: PLACE };
const ghostOf = (s: Shape): Streams => ({
  rows: Array.from({ length: s.n }, (_, i) => ({ ...GHOST_ROW, href: `#${i}` })),
  total: s.total,
  gateways: s.gateways,
  withDeadLetter: s.withDeadLetter,
  kinds: Array.from({ length: s.kinds }, (_, i) => `kind${i}`),
  filter: { kind: "", q: "" },
});

// Every stream's health, counted by state and said with the state's own mark: the summary never claims a state
// nobody measured, so a console that holds no traffic says "no data" and a failing stream would say "N failing".
function HealthSummary({ v }: { v: Streams }) {
  const word = useStatusWord();
  const counts = v.healthCounts ?? { unknown: v.total };
  // What needs a person first; a state this app does not know yet still counts, after the others.
  const states = [...HEALTH_ORDER, ...Object.keys(counts).filter((s) => !HEALTH_ORDER.includes(s))].filter((s) => counts[s]);
  return (
    <>
      {states.map((state) => (
        <Status key={state} kind={state} label={`${counts[state]} ${word(state)}`} />
      ))}
    </>
  );
}

// Failing streams first, then degraded, then the rest, each group in the order the server sent. The order is
// decided when data arrives, and not while a hand is on the table: the rows the reader is pointing at or has
// focused keep their places (their numbers still update) until the pointer and the focus leave.
function useOrdered(rows: StreamRow[]) {
  const [held, setHeld] = useState<string[] | null>(null);
  const sorted = useMemo(() => [...rows].sort((a, b) => severity(a.health) - severity(b.health)), [rows]);
  const ordered = useMemo(() => {
    if (!held) return sorted;
    const at = (r: StreamRow) => held.indexOf(r.href);
    return [...sorted].sort((a, b) => (at(a) < 0 ? held.length : at(a)) - (at(b) < 0 ? held.length : at(b)));
  }, [sorted, held]);
  const hold = () => setHeld((h) => h ?? ordered.map((r) => r.href));
  const release = () => setHeld(null);
  return { ordered, hold, release };
}

// The bar (see ui/filters): a search and the destination kinds, drawn at once from the last shape seen.
const kindLogo = (kind: string) => (kind === "gs" ? "gcs" : kind); // the logo a destination's kind is drawn with

function StreamsBar({ filter, v, ghost }: { filter: Filter; v: Streams; ghost: boolean }) {
  const { t, n } = useT();
  const kindName = useLogoTitle();
  const { number } = useFormat();
  const navigate = useNavigate();
  const [text, setText] = useUrlSearch("q");
  const go = (patch: Record<string, string>) => void navigate(filter.hrefWith(patch));
  const kind = filter.get("kind");
  const q = filter.get("q");
  const filters: FilterDef[] = v.kinds.length > 1
    ? [{
        id: "kind", label: t("streams.kind"), allLabel: t("streams.kind.all"), pending: ghost,
        options: v.kinds.map((k) => ({ key: k, label: kindName(kindLogo(k)), icon: <Logo k={k} /> })),
        value: kind ? [kind] : [], onChange: (x) => go({ kind: x[0] ?? "" }),
      }]
    : [];
  const chips: Chip[] = kind ? [{ id: "kind", group: t("streams.kind"), value: v.kinds.includes(kind) ? kindName(kindLogo(kind)) : kind, onRemove: () => go({ kind: "" }) }] : [];
  return (
    <FilterBar
      search={<SearchField value={text} onChange={setText} label={t("streams.search")} />}
      filters={filters}
      chips={chips}
      clearable={filter.any}
      onClearAll={() => go({ kind: "", q: "" })}
      count={<ResultCount pending={ghost} shown={v.rows.length} tail={<>{n("streams.count", v.total, { count: number(v.total) })}{q && <> {t("workflows.count.search", { q })}</>}</>} />}
    />
  );
}

// The phone draws cards, whose heights are not the table rows': each keeps its own memory of row heights.
const phone = () => typeof matchMedia === "function" && matchMedia("(max-width: 768px)").matches;

function StreamsData({ filter }: { filter: Filter }) {
  const { t, n } = useT();
  const qs = new URLSearchParams();
  for (const f of FIELDS) if (filter.get(f)) qs.set(f, filter.get(f));
  const q = useData<Streams>(["ingest", "streams", qs.toString()], `/api/ingest/streams${qs.size ? `?${qs}` : ""}`, { topic: ["ingest", TOPIC_STREAMS] });
  const ghost = !q.data;
  const { ordered, hold, release } = useOrdered(q.data?.rows ?? []);
  // Only the unfiltered answer is remembered: a filter's counts are not the list's shape.
  const seen = useSeen("streams", qs.size === 0 && q.data ? shapeOf(q.data) : undefined, FIRST);
  const v = q.data ?? ghostOf(seen);
  // Every stream of a gateway shares its origin: said once up here, and per row only when the list mixes gateways.
  const oneGateway = v.gateways === 1 ? v.rows[0]?.gateway : "";
  return (
    <>
      {(v.total > 0 || ghost) && <StreamsStrip />}
      {v.total > 0 && (
        <div aria-hidden={ghost || undefined} className={`stl-sum ${ghost ? "sk-ghost" : ""}`}>
          <span>{n("streams.summary.streams", v.total)}</span>
          <span title={t("streams.gateway")}>
            {n("streams.summary.gateways", v.gateways)}
            {oneGateway && <> <span className="stl-gw">{oneGateway}</span></>}
          </span>
          <HealthSummary v={v} />
          {v.withDeadLetter > 0 && <span>{n("streams.summary.dead_letter", v.withDeadLetter)}</span>}
        </div>
      )}
      {v.total > 0 && (
        <StreamsBar filter={filter} v={v} ghost={ghost} />
      )}
      <div
        aria-busy={ghost}
        onPointerEnter={hold}
        onPointerLeave={release}
        onFocus={hold}
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) release();
        }}
      >
        {q.isError ? (
          <p role="alert" className="rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">
            <ErrorText error={q.error} />
          </p>
        ) : v.rows.length === 0 ? (
          <div aria-hidden={ghost || undefined} className={ghost ? "sk-ghost" : undefined}>
          <Empty msg={v.total > 0 ? t("streams.empty.filtered") : t("streams.empty")} hint={v.total > 0 ? undefined : "brevis gateway publish gateway.yaml"}>
            {v.total > 0 && (
              <Link to="/ingest/streams" className={`mt-2 inline-block text-xs ${linkUnderline}`}>
                {t("workflows.clear")}
              </Link>
            )}
          </Empty>
          </div>
        ) : (
          <Table id="stream-rows" cls="stl" head={<Head />} ghost={ghost} memo={phone() ? "streams.phone" : "streams"}>
            <Rows rows={q.data ? ordered : v.rows} gateway={v.gateways !== 1} />
          </Table>
        )}
      </div>
    </>
  );
}

// Open the first time, then as the operator left it (a boolean, never text).
function HowItWorks() {
  const { t } = useT();
  const [open, setOpen] = useState(() => recall<{ open: boolean }>("streams.how", { open: true }).open);
  return (
    <details
      className="stl-how"
      open={open}
      onToggle={(e) => {
        const now = e.currentTarget.open;
        if (now === open) return;
        setOpen(now);
        remember("streams.how", { open: now });
      }}
    >
      <summary>
        <Icon name="chevron-right" className="h-3.5 w-3.5" />
        {t("streams.how.title")}
      </summary>
      <HowSteps
        steps={(["post", "batch", "write"] as const).map((k, i) => ({
          kicker: t(`streams.how.${i + 1}`),
          title: t(`streams.how.${k}`),
          text: t(`streams.how.${k}.text`),
        }))}
      />
    </details>
  );
}

export default function StreamsRoute() {
  const { t } = useT();
  const filter = useFilter(FIELDS, "/ingest/streams");
  const { show } = useDrawer("new");
  return (
    <Page>
      <PageHead
        title={t("streams.title")}
        actions={
          <button type="button" className={btn(true)} aria-haspopup="dialog" onClick={show}>
            <Icon name="plus" className="h-3.5 w-3.5" />
            {t("streams.new")}
          </button>
        }
      />
      <Lead before={t("streams.lead.before")} strong={t("streams.lead.strong")} after={t("streams.lead.after")} />
      <StreamsData filter={filter} />
      <HowItWorks />
      <HowDrawer
        id="new"
        title={t("streams.new")}
        lead={t("streams.new.lead")}
        blocks={[
          { title: t("streams.new.declare"), text: NEW_YAML },
          { title: t("streams.new.publish"), text: NEW_COMMANDS },
        ]}
        note={t("streams.new.note")}
      />
    </Page>
  );
}
