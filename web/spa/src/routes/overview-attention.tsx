import { useQuery } from "@tanstack/react-query";
import { useFallbackInterval, useLive } from "../live/hooks";
import { Link } from "react-router";
import { getJSON } from "../api/client";
import { useFormat, useT } from "../i18n";
import { useAgo } from "../ui/format";
import { buttonClass } from "../ui/button";
import { Icon } from "../ui/icon";
import { LoadError } from "../ui/query-boundary";
import { StatusMark, runKind, useStatusWord } from "../ui/status";
import { TOPIC_STREAMS } from "../ui/metrics/hooks";
import { useQuickRun } from "./run-now";

type Cause = { step: string; mapIndex: number | null; exitCode: number | null; line: string };
type Item = {
  workflow: string;
  runId: string;
  failedAt: string;
  streak: number;
  cause: Cause | null;
  rerunnable: boolean;
  params: Record<string, string>;
  rerunning: string;
};
type Target = { target: string; status: string; workflow: string; missed: string };
type StreamItem = { gateway: string; path: string; href: string; since: string | null };
export type Attention = { total: number; items: Item[]; more: number; targets: Target[]; targetsMore: number; streams: StreamItem[]; streamsMore: number };

/** "Needs your attention"; the overview screen refetches it on the `overview` topic, and the fallback polls only while the stream is down. */
export const attentionQuery = {
  queryKey: ["overview", "attention"],
  queryFn: () => getJSON<Attention>("/api/overview/attention"),
};
export const useAttention = () => useQuery({ ...attentionQuery, refetchInterval: useFallbackInterval() });

/** The card that stands where the section will be: the "all is well" card (the usual answer) drawn as a ghost, so its box is the real one. */
export function AttentionSkeleton() {
  const { t } = useT();
  return <NoneCard ghost label={t("overview.attention.none")} hint={t("overview.attention.none_hint")} />;
}

function NoneCard({ ghost, label, hint }: { ghost?: boolean; label: string; hint: string }) {
  return (
    <section data-attention={ghost ? undefined : ""} aria-busy={ghost || undefined} aria-hidden={ghost || undefined} className="mb-3">
      <div data-attention-empty={ghost ? undefined : ""} className={`flex items-start gap-3 rounded-card border px-4 py-3 ${ghost ? "border-line bg-surface" : "border-state-success/30 bg-state-success/5"}`}>
        <Icon name="check" className={`mt-0.5 h-4 w-4 text-state-success ${ghost ? "sk-ghost" : ""}`} />
        <div className={ghost ? "sk-ghost" : undefined}>
          <p className="text-[13.5px] font-medium text-ink">{label}</p>
          <p className="mt-0.5 text-[12.5px] text-muted">{hint}</p>
        </div>
      </div>
    </section>
  );
}

// What stopped the run: its step, and how it exited; "The run failed" when it died before any step.
function useWhy() {
  const { t } = useT();
  return (c: Cause) => {
    if (!c.step) return t("run.why.run");
    const step = c.mapIndex === null ? c.step : `${c.step}[${c.mapIndex}]`;
    return c.exitCode === null ? t("run.why.step", { step }) : t("run.why.step_exit", { step, code: c.exitCode });
  };
}

function ItemRow({ it }: { it: Item }) {
  const { t, n } = useT();
  const ago = useAgo();
  const quick = useQuickRun(true);
  const why = useWhy();
  const streak = it.streak < 2 ? "" : it.streak === 2 ? t("overview.second_time") : n("run.why.streak", it.streak);
  return (
    <li data-attention-item={it.workflow} className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-state-failed/15 px-4 py-3 last:border-b-0">
      <StatusMark kind="failed" />
      <div className="min-w-0 flex-1 max-[640px]:basis-[calc(100%-2rem)]">
        <p className="text-[13px]">
          <span className="font-mono text-ink">{it.workflow}</span>
          <span className="ml-1 text-[12.5px] text-muted">
            {t("overview.failed")} {ago(it.failedAt)}
            {streak && ` · ${streak}`}
          </span>
        </p>
        {it.cause && (
          <p className="mt-0.5 truncate text-[12.5px] text-ink-2">
            {why(it.cause)}
            {it.cause.line && (
              <>
                {" — "}
                <code className="font-mono text-[12px] text-state-failed">{it.cause.line}</code>
              </>
            )}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Link to={`/runs/${it.runId}#logs`} className={buttonClass("secondary", "small")}>{t("overview.view_log")}</Link>
        {it.rerunning ? (
          <Link to={`/runs/${it.rerunning}`} className="inline-flex items-center gap-1.5 text-[12.5px] text-state-running hover:underline">
            <StatusMark kind="running" />
            {t("overview.rerunning")}
          </Link>
        ) : (
          it.rerunnable && (
            // The values are the failed run's: no questions, a toast that follows the new run.
            <button type="button" disabled={quick.pending === it.workflow} className={buttonClass("secondary", "small")} onClick={() => void quick.run(it.workflow, it.params, it.runId)}>
              <Icon name="refresh-cw" className="h-3 w-3" />
              {t("overview.rerun")}
            </button>
          )
        )}
      </div>
    </li>
  );
}

function TargetRow({ tg }: { tg: Target }) {
  const { t } = useT();
  const ago = useAgo();
  const word = useStatusWord();
  return (
    <li data-attention-target={tg.target} className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-state-failed/15 px-4 py-3 last:border-b-0">
      <StatusMark kind={runKind(tg.status)} />
      <div className="min-w-0 flex-1 max-[640px]:basis-[calc(100%-2rem)]">
        <p className="text-[13px]">
          <span className="font-mono break-all text-ink">{tg.target}</span>
          <span className="ml-1 text-[12.5px] text-muted">{word(runKind(tg.status))}</span>
        </p>
        <p className="mt-0.5 text-[12.5px] text-ink-2">{t("overview.target.missed", { workflow: tg.workflow, when: ago(tg.missed) })}</p>
      </div>
      <Link to={`/data/target?u=${encodeURIComponent(tg.target)}`} className={buttonClass("secondary", "small")}>{t("overview.open_catalog")}</Link>
    </li>
  );
}

function StreamRow({ st }: { st: StreamItem }) {
  const { t } = useT();
  const { clock } = useFormat();
  // The path is code and the rest is a sentence in the reader's language, with the path wherever that
  // language puts it: the catalog's {stream} is filled with a marker and the text is cut around it.
  const [before = "", after = ""] = t("overview.stream.failing", { stream: "\u0001" }).split("\u0001");
  return (
    <li data-attention-stream={`${st.gateway}${st.path}`} className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-state-failed/15 px-4 py-3 last:border-b-0">
      <StatusMark kind="failed" />
      <div className="min-w-0 flex-1 max-[640px]:basis-[calc(100%-2rem)]">
        <p className="text-[13px] text-ink">
          {before}
          <span className="font-mono break-all">{st.path}</span>
          {after}
          {st.since && <span className="ml-1 text-[12.5px] text-muted">{t("overview.stream.since", { when: clock(st.since) })}</span>}
        </p>
        <p className="mt-0.5 font-mono text-[12px] text-muted">{st.gateway}</p>
      </div>
      <Link to={st.href} className={buttonClass("secondary", "small")}>{t("overview.stream.open")}</Link>
    </li>
  );
}

const more = "block border-t border-state-failed/20 px-4 py-2 text-[12.5px] text-ink hover:underline";

export function AttentionSection() {
  const { t, n } = useT();
  const q = useAttention();
  // A stream's health follows the gateway's metrics, which move faster than the `overview` topic.
  useLive([TOPIC_STREAMS], [attentionQuery.queryKey]);
  const a = q.data;
  if (q.isError && !a) return <section className="mb-3"><LoadError onRetry={() => void q.refetch()} /></section>;
  if (!a) return <AttentionSkeleton />;

  if (a.total === 0) return <NoneCard label={t("overview.attention.none")} hint={t("overview.attention.none_hint")} />;
  return (
    <section data-attention className="mb-3">
      <div className="overflow-hidden rounded-card border border-state-failed/40 bg-state-failed/5">
        <header className="flex flex-wrap items-baseline gap-x-2 gap-y-1 border-b border-state-failed/20 px-4 py-2.5">
          <Icon name="triangle-alert" className="h-4 w-4 self-center text-state-failed" />
          <h2 className="text-[14px] font-medium text-ink">{t("overview.attention")}</h2>
          <span className="font-mono text-[12px] text-muted tabular-nums">{a.total}</span>
          <span className="text-[12.5px] text-muted">{t("overview.attention.hint")}</span>
        </header>
        <ul>
          {a.items.map((it) => (
            <ItemRow key={it.workflow} it={it} />
          ))}
        </ul>
        {a.more > 0 && <Link className={more} to="/runs?state=failed">{n("overview.attention.more", a.more)} →</Link>}
        {a.targets.length > 0 && (
          <ul className="border-t border-state-failed/20">
            {a.targets.map((tg) => (
              <TargetRow key={tg.target} tg={tg} />
            ))}
          </ul>
        )}
        {a.streams.length > 0 && (
          <ul className="border-t border-state-failed/20">
            {a.streams.map((st) => (
              <StreamRow key={`${st.gateway}${st.path}`} st={st} />
            ))}
          </ul>
        )}
        {a.streamsMore > 0 && <Link className={more} to="/ingest/streams">{n("overview.stream.more", a.streamsMore)} →</Link>}
        {a.targetsMore > 0 && <Link className={more} to="/data?status=attention">{n("overview.target.more", a.targetsMore)} →</Link>}
      </div>
    </section>
  );
}
