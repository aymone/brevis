import { useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { useData } from "../api/live";
import type { Change, Load, LoadDetail, Place } from "../api/ingest";
import { cat, useFormat, useMsg, useT } from "../i18n";
import { useSince } from "../ui/format";
import { Drawer } from "../ui/drawer";
import { Icon } from "../ui/icon";
import { btn, ButtonLink, Card, ShortStamp, linkUnderline } from "../ui/kit";
import { Logo, useLogoTitle } from "../ui/logo";
import { useSourceLabel } from "../ui/place";
import { LoadBars } from "../ui/sparkline";
import { Freshness } from "../ui/status";
import { StepConfigPanel } from "./load-config";
import { useRefreshRuns } from "./run-now";
import { fieldErrors, fieldText, triggerRun } from "../api/trigger";
import { toast } from "../shell/toast";
import { ErrorText, useErrorText } from "../api/errors";

const short = (id: string) => id.slice(0, 8);
const th = "px-2.5 py-2 text-[11.5px] font-medium whitespace-nowrap text-faint";
const dt = "text-muted";

// Before the first answer the page draws the last load it showed (or this typical one) through the same
// markup, as a ghost (.sk-ghost): the labels are the page's own, the values are bars, and the heights are the
// loaded heights. Only the data-bound parts are ghosted; titles, labels, tabs and buttons are never.
const GHOST_PLACE: Place = { uri: "postgres://warehouse/schema/table_name", kind: "postgres", where: "postgres://warehouse/schema/", what: "table_name", note: "" };
const GHOST: Load = {
  workflow: "",
  step: "",
  choices: [],
  choosing: false,
  detail: {
    status: "on_time",
    reason: null,
    source: { kind: "http", driver: "", label: "Source label", labelMsg: null, where: "https://api.example.com/path" },
    target: GHOST_PLACE,
    hasSchedule: true,
    when: cat("every hour"),
    cron: "0 * * * *",
    timezone: "UTC",
    manual: false,
    legacy: false,
    lastLoaded: "2000-01-01T00:00:00Z",
    lastRows: 1000,
    read: null,
    readNothing: false,
    fixing: null,
    bars: Array.from({ length: 12 }, (_, i) => ({ runId: `${i}`, rows: 1, title: "" })),
    missed: 0,
    loads: Array.from({ length: 5 }, (_, i) => ({ loadedAt: "2000-01-01T00:00:00Z", rows: 1000, bytes: 1000, runId: `run${i}` })),
    loadsCount: { k: "loads.drawer.recent.count", n: 5 },
    described: false,
    config: null,
    changes: [],
    readBy: [],
    query: "",
    run: "",
  },
};
const gh = (ghost: boolean) => (ghost ? "sk-ghost" : "");

function Changes({ changes, ghost }: { changes: Change[]; ghost: boolean }) {
  const { t } = useT();
  const msg = useMsg();
  return (
    <Card title={t("config.changes.title")} count={ghost ? undefined : String(changes.length)}>
      <ol aria-hidden={ghost || undefined} className={`divide-y divide-line-soft ${gh(ghost)}`}>
        {changes.map((c) => (
          <li key={c.at + c.runId} className="px-3.5 py-2 text-[12px]">
            <span className="font-mono text-muted tabular-nums">
              <ShortStamp at={c.at} />
            </span>{" "}
            {c.runId !== "" && (
              <Link to={`/runs/${c.runId}`} className={`ml-2 font-mono ${linkUnderline}`}>
                {short(c.runId)}
              </Link>
            )}{" "}
            {c.first ? <span className="ml-2 text-muted">{t("config.first_recorded")}</span> : c.earlier ? <span className="ml-2 text-muted">{t("config.earlier")}</span> : null}{" "}
            {c.lines.length > 0 && (
              <ul className="mt-1 space-y-0.5">
                {c.lines.map((l) => (
                  <li key={l.path}>
                    <span className="font-mono text-muted">{l.path}</span>{" "}
                    <span className="ml-2 font-mono break-all text-ink">{msg(l.text)}</span>
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ol>
    </Card>
  );
}

/** The facts of the load: from where to where, when it runs, its last load. The labels are static; the values arrive. */
function Facts({ step, d, ghost }: { step: string; d: LoadDetail; ghost: boolean }) {
  const { t, n } = useT();
  const { number } = useFormat();
  const msg = useMsg();
  const sourceLabel = useSourceLabel();
  const since = useSince();
  const kindName = useLogoTitle();
  return (
    <dl className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-4 gap-y-3 rounded-card border border-line bg-surface px-4 py-3.5 text-[12.5px]">
      <dt className={dt}>{t("loads.drawer.from")}</dt>
      <dd aria-hidden={ghost || undefined} className={`min-w-0 ${gh(ghost)}`}>
        {d.source.kind === "" ? (
          <span className="text-faint" title={t("loads.source.none.tip")}>
            {t("loads.source.none")}
          </span>
        ) : (
          <>
            <span className="flex items-center gap-1.5 text-ink-2">
              {(d.source.kind === "http" || d.source.kind === "file") && <Logo k={d.source.kind} />}
              {sourceLabel(d.source)}
            </span>
            {d.source.where !== "" && <p className="mt-0.5 font-mono text-[12px] break-all text-ink">{d.source.where}</p>}
          </>
        )}
      </dd>
      <dt className={dt}>{t("loads.drawer.to")}</dt>
      <dd aria-hidden={ghost || undefined} className={`min-w-0 ${gh(ghost)}`}>
        {
          <>
            {d.target.kind !== "" && (
              <span className="flex items-center gap-1.5 text-ink-2">
                <Logo k={d.target.kind} />
                {kindName(d.target.kind)}
              </span>
            )}
            <p className="mt-0.5 flex items-start gap-1.5 font-mono text-[12px] break-all text-ink">{d.target.uri}</p>
            {d.target.uri !== "" && !d.legacy && (
              <Link to={`/data/target?u=${encodeURIComponent(d.target.uri)}`} className="mt-1 inline-block text-[12px] text-ink-2 hover:text-ink hover:underline">
                {t("loads.drawer.catalog")} &rarr;
              </Link>
            )}
          </>
        }
      </dd>
      <dt className={dt}>{t("loads.drawer.step")}</dt>
      <dd className="font-mono text-ink">{step}</dd>
      <dt className={dt}>{t("loads.drawer.when")}</dt>
      <dd aria-hidden={ghost || undefined} className={`text-ink ${gh(ghost)}`}>
        {d.hasSchedule ? (
          <>
            {msg(d.when)} <span className="ml-1 font-mono text-[11.5px] text-muted">{d.cron} {d.timezone}</span>
          </>
        ) : (
          t("loads.manual")
        )}
      </dd>
      <dt className={dt}>{t("loads.drawer.last")}</dt>
      <dd aria-hidden={ghost || undefined} className={`text-ink ${gh(ghost)}`}>
        {
          <>
            {d.lastLoaded ? since(d.lastLoaded) : "—"}
            {d.lastRows !== null && <span className={`ml-1 ${d.readNothing ? "text-state-failed" : ""}`}>· {n("loads.drawer.rows", d.lastRows)}</span>}
            {d.read !== null && <span className="ml-1 text-muted">{t("loads.drawer.read", { n: number(d.read) })}</span>}
          </>
        }
      </dd>
    </dl>
  );
}

function Body({ workflow, step, d, ghost }: { workflow: string; step: string; d: LoadDetail; ghost: boolean }) {
  const { t } = useT();
  const { number } = useFormat();
  const msg = useMsg();
  return (
    <div className="space-y-3">
      {d.reason && (
        <p aria-hidden={ghost || undefined} className={`rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-ink-2 ${gh(ghost)}`}>
          {msg(d.reason)}
        </p>
      )}
      <Facts step={step} d={d} ghost={ghost} />
      {d.fixing && (
        <Link to={`/runs/${d.fixing}`} className={`inline-block text-[12px] text-state-running hover:underline ${gh(ghost)}`}>
          {t("loads.fixing")} &rarr;
        </Link>
      )}
      <section>
        <h3 className="mb-2 text-[11px] font-medium tracking-[0.08em] text-muted uppercase">{t("loads.drawer.bars", { n: ghost ? "…" : d.bars.length })}</h3>
        <div aria-hidden={ghost || undefined} className={gh(ghost)}>
          <LoadBars bars={d.bars.map((b) => ({ href: `/runs/${b.runId}`, rows: b.rows, title: b.title }))} missed={d.missed} />
        </div>
        <p className="mt-1.5 text-[11.5px] text-faint">{t("catalog.drawer.legend")}</p>
        <p className="mt-1 text-[12px] text-muted">{t("loads.drawer.drop")}</p>
      </section>

      <Card title={t("config.title")}>
        {d.described && d.config ? (
          <div aria-hidden={ghost || undefined} className={gh(ghost)}>
            <StepConfigPanel c={d.config} />
          </div>
        ) : (
          <p className="px-3.5 py-3 text-[12.5px] text-muted">
            {t("config.undescribed")}{" "}
            <Link to={`/workflows/${encodeURIComponent(workflow)}`} className={linkUnderline}>
              {t("config.definition")}
            </Link>
            .
          </p>
        )}
      </Card>

      {d.changes.length > 0 && <Changes changes={d.changes} ghost={ghost} />}

      <Card title={t("loads.drawer.recent")} count={ghost ? undefined : msg(d.loadsCount)}>
        <div className="max-h-[28rem] overflow-auto">
          <table className="ct w-full text-[12.5px]">
            <thead className="sticky top-0 bg-surface">
              <tr className="text-left">
                <th className={`${th} pl-3.5`}>{t("loads.drawer.recent.at")}</th>
                <th className={`${th} text-right`}>{t("loads.col.rows")}</th>
                <th className={`${th} text-right`}>{t("loads.drawer.recent.bytes")}</th>
                <th className={`${th} pr-3.5`}>{t("loads.drawer.recent.run")}</th>
              </tr>
            </thead>
            <tbody aria-hidden={ghost || undefined} className={`divide-y divide-line-soft ${gh(ghost)}`}>
              {d.loads.map((l) => (
                    <tr key={l.runId}>
                      <td className="px-2.5 pl-3.5 font-mono text-[12px] whitespace-nowrap text-muted tabular-nums">
                        <ShortStamp at={l.loadedAt} />
                      </td>
                      <td className="px-2.5 text-right tabular-nums">{l.rows === null ? "—" : number(l.rows)}</td>
                      <td className="px-2.5 text-right tabular-nums">{l.bytes === null ? "—" : number(l.bytes)}</td>
                      <td className="px-2.5 pr-3.5">
                        <Link to={`/runs/${l.runId}`} className={`font-mono text-[12px] ${linkUnderline}`}>
                          {short(l.runId)}
                        </Link>
                      </td>
                    </tr>
                  ))}
            </tbody>
          </table>
        </div>
      </Card>

      {d.readBy.length > 0 && (
        <section aria-hidden={ghost || undefined} className={gh(ghost)}>
          <h3 className="mb-2 text-[12.5px] font-semibold text-ink">{t("catalog.drawer.read_by")}</h3>
          <ul className="space-y-1 text-[12.5px]">
            {d.readBy.map((e) => (
              <li key={e.href}>
                <Link to={e.href} className="font-mono text-ink hover:underline">
                  GET {e.path}
                </Link>{" "}
                <span className="ml-1.5 text-muted">{e.api}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

/** The page's verbs: a query of the destination, the workflow, and running it. */
function Actions({ workflow, d, ghost }: { workflow: string; d: LoadDetail | null; ghost: boolean }) {
  const { t } = useT();
  const navigate = useNavigate();
  const errText = useErrorText();
  const refresh = useRefreshRuns();
  const [pending, setPending] = useState(false);
  // One click, no questions (the workflow has no parameters). The drawer is modal, so a toast beside it could be
  // neither seen nor clicked: the reader goes to the new run, and the toast says it was made.
  const runNow = async () => {
    setPending(true);
    try {
      const r = await triggerRun(workflow, { params: {} });
      void refresh(workflow);
      toast(t("workflows.run_created", { workflow }), { kind: "run" });
      navigate(r.href);
    } catch (e) {
      const [first] = Object.entries(fieldErrors(e));
      toast(first ? `${first[0]}: ${fieldText(t, first[1])}` : errText(e), { kind: "fail" });
    } finally {
      setPending(false);
    }
  };
  return (
    <>
      {d && d.query !== "" && (
        <span aria-hidden={ghost || undefined} className={`inline-flex ${gh(ghost)}`}>
          <ButtonLink to={d.query} icon="terminal">
            {t("loads.drawer.query")}
          </ButtonLink>
        </span>
      )}
      <ButtonLink to={`/workflows/${encodeURIComponent(workflow)}`}>{t("loads.drawer.workflow")}</ButtonLink>
      {d?.run === "now" && (
        <button type="button" aria-hidden={ghost || undefined} disabled={pending} className={`${btn(true)} ${gh(ghost)}`} onClick={() => void runNow()}>
          <Icon name="play" className="h-3.5 w-3.5" />
          {t("loads.drawer.run")}
        </button>
      )}
      {d?.run === "params" && (
        <ButtonLink to={`/workflows/${encodeURIComponent(workflow)}#run`} icon="play" primary>
          {t("loads.drawer.run.params")}
        </ButtonLink>
      )}
    </>
  );
}

/** The load behind the address; the list reads it too (a 404 is its note), and one key keeps that one request. */
export function useLoad(workflow: string, step: string, target: string, enabled: boolean) {
  return useData<Load>(["ingest", "load", workflow, step, target], `/api/ingest/loads/${encodeURIComponent(workflow)}/${encodeURIComponent(step)}${target ? `?target=${encodeURIComponent(target)}` : ""}`, {
    topic: "ingest",
    keep: false,
    enabled,
  });
}

const choiceCls = "block rounded-card border px-3 py-2 font-mono text-[12px] break-all transition-colors";

/**
 * One step landing on one destination, as a drawer over the loads list (it is mounted by the list, so the list
 * stays as it was). Opening another load swaps the content of this same dialog. The title and the labels are
 * static; only the values wait.
 */
export function LoadDrawer({ workflow, step, target, q, close }: { workflow: string; step: string; target: string; q: ReturnType<typeof useLoad>; close: () => void }) {
  const { t, n } = useT();
  // Kept across the swap so that closing still knows whether the drawer was pushed.
  const { state } = useLocation();
  const ghost = !q.data;
  const v = q.data ?? GHOST;
  // The choices of this step stay while another destination loads: the link that was pressed must not vanish
  // under the keyboard.
  const seen = useRef<{ key: string; uris: string[] }>({ key: "", uris: [] });
  const key = `${workflow}/${step}`;
  if (v.choices.length > 0) seen.current = { key, uris: v.choices.map((c) => c.uri) };
  const uris = v.choices.length > 0 ? v.choices.map((c) => c.uri) : seen.current.key === key ? seen.current.uris : [];
  const choosing = !ghost && v.choosing;
  return (
    <Drawer
      plain
      id="load"
      title={step}
      open
      onHide={close}
      head={
        <div className="min-w-0">
          <p className="text-[11px] font-medium tracking-[0.08em] text-muted uppercase">{t("loads.drawer.kicker", { workflow })}</p>
          <h2 id="load-title" className="font-mono text-[15px] font-semibold break-all text-ink">{step}</h2>
          {v.detail && !choosing && (
            <p aria-hidden={ghost || undefined} className={`mt-1 ${gh(ghost)}`}>
              <Freshness status={v.detail.status} />
            </p>
          )}
        </div>
      }
      foot={<Actions workflow={workflow} d={v.detail} ghost={ghost} />}
    >
      <div className="space-y-4">
        {q.isError && (
          <p role="alert" className="rounded-card border border-line bg-surface-2 px-3 py-2 text-[12.5px] text-muted">
            <ErrorText error={q.error} />
          </p>
        )}
        {uris.length > 0 && (
          <section>
            <p aria-hidden={ghost || undefined} className={`mb-2 text-[12.5px] text-muted ${gh(ghost)}`}>{n("loads.drawer.choices", uris.length)}</p>
            <ul className="space-y-1">
              {uris.map((uri) => {
                const current = uri === target;
                return (
                  <li key={uri}>
                    {/* A swap, not a push: Back leaves the drawer, it does not walk through the destinations. */}
                    <Link
                      to={{ search: `?target=${encodeURIComponent(uri)}` }}
                      replace
                      state={state}
                      aria-current={current ? "true" : undefined}
                      className={`${choiceCls} ${current ? "border-ink text-ink" : "border-line text-ink-2 hover:border-line-strong"}`}
                    >
                      {uri}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </section>
        )}
        {!q.isError && v.detail && !choosing && <Body workflow={workflow} step={step} d={v.detail} ghost={ghost} />}
      </div>
    </Drawer>
  );
}
