import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useFormat, useT } from "../i18n";
import type { RunLog } from "../api/run";
import "./bar.css";
import { FilterBar, Segmented, SearchField } from "./filters";
import { Icon } from "./icon";
import { Skeleton } from "./skeleton";

type Props = {
  /** Undefined while the run is on its way: the tools stay, the lines are a skeleton. */
  log: RunLog | undefined;
  /** True while the run moves: the view follows the tail until the reader scrolls up. */
  live: boolean;
  downloadHref: string;
  /** Bump to scroll to the failing line (clearing the search and level first). */
  gotoCause?: number;
};

const LEVELS = ["all", "warn", "err"] as const;
type Level = (typeof LEVELS)[number];

const tone = (level?: string) => (level === "warn" ? "text-warn" : level === "err" ? "bg-state-failed/10 text-state-failed" : "text-ink");

function Marked({ text, q }: { text: string; q: string }) {
  if (!q) return <>{text}</>;
  const lower = text.toLowerCase();
  const out: ReactNode[] = [];
  let at = 0;
  for (let i = lower.indexOf(q); i >= 0; i = lower.indexOf(q, at)) {
    out.push(text.slice(at, i), <mark key={i}>{text.slice(i, i + q.length)}</mark>);
    at = i + q.length;
  }
  out.push(text.slice(at));
  return <>{out}</>;
}

// The lines' area never gets shorter than this, loading, empty or growing: a log that fills while the run
// runs would otherwise start as a sliver and push whatever sits under it, and the skeleton would not match it.
const LINES_MIN = "log-lines-min";

/** The lines' shape while they load: a heading and a few numbered lines. */
function LinesSkeleton() {
  return (
    <div aria-busy="true" className={`${LINES_MIN} grid gap-2 px-3 py-3`} style={{ alignContent: "start" }}>
      {[60, 90, 75, 40, 85, 55].map((w, i) => (
        <Skeleton key={i} w={`${w}%`} h="0.8rem" />
      ))}
    </div>
  );
}

/**
 * The run's log (web/pages logsPanel): attempts under sticky headings, numbered
 * lines, search, a level filter, wrap and copy. While `live` it follows the
 * tail; scrolling up stops that (a button brings it back), as `tail -f` does.
 * The tools are static and always drawn; only the lines wait for data.
 */
export function LogView({ log, live, downloadHref, gotoCause = 0 }: Props) {
  const { t, n } = useT();
  const { number } = useFormat();
  const [q, setQ] = useState("");
  const [level, setLevel] = useState<Level>("all");
  const [wrap, setWrap] = useState(false);
  const [copied, setCopied] = useState(false);
  const [following, setFollowing] = useState(true);
  const [flash, setFlash] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  const needle = q.trim().toLowerCase();
  const filtering = needle !== "" || level !== "all";
  const blocks = useMemo(() => log?.blocks ?? [], [log]);
  // How many lines each level holds: the figures the level control shows, so a click is never a guess.
  const levelCount = useMemo(() => {
    const c: Record<Level, number> = { all: 0, warn: 0, err: 0 };
    for (const b of blocks) for (const it of b.items) if (!it.gap) { c.all++; if (it.level === "warn" || it.level === "err") c[it.level]++; }
    return c;
  }, [blocks]);

  // Followed only while the reader is at the bottom: a few pixels of slack.
  const onScroll = () => {
    const el = box.current;
    if (el) setFollowing(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
  };
  useLayoutEffect(() => {
    const el = box.current;
    if (live && following && el) el.scrollTop = el.scrollHeight;
  }, [log?.lines, live, following]);

  useEffect(() => {
    if (!gotoCause) return;
    setQ("");
    setLevel("all");
    setFollowing(false);
    setFlash(gotoCause);
    // After the unfiltered lines are back in the DOM.
    requestAnimationFrame(() => {
      const el = document.getElementById("log-cause");
      const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      el?.scrollIntoView({ block: "center", behavior: still ? "auto" : "smooth" });
    });
  }, [gotoCause]);

  const visible = useMemo(
    () => blocks.map((b) => b.items.filter((it) => it.gap || ((level === "all" || it.level === level) && (!needle || (it.text ?? "").toLowerCase().includes(needle))))),
    [blocks, level, needle],
  );

  const copy = () => {
    const text = blocks.flatMap((_, i) => (visible[i] ?? []).filter((it) => !it.gap).map((it) => it.text ?? "")).join("\n");
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  const bar = (
    <FilterBar
      className="border-b border-line-soft px-3 py-2"
      search={<SearchField value={q} onChange={setQ} label={t("run.log.search")} hotkey={false} />}
      primary={
        <Segmented
          label={t("run.log.level")}
          value={level}
          pending={!log}
          onChange={(l) => setLevel(l as Level)}
          items={LEVELS.map((l) => ({ key: l, label: t(`run.log.level.${l}`), count: levelCount[l] }))}
        />
      }
      actions={
        <>
          <button type="button" aria-pressed={wrap} onClick={() => setWrap(!wrap)} className="flt-ctl flt-btn">
            {t("run.log.wrap")}
          </button>
          <button type="button" onClick={copy} className="flt-ctl flt-btn" title={t("run.log.copy")} aria-label={copied ? t("copy.done") : t("run.log.copy")}>
            <Icon name={copied ? "check" : "copy"} className="h-3.5 w-3.5" />
            <span className="flt-btn-text">{copied ? t("copy.done") : t("run.log.copy")}</span>
          </button>
          <a href={downloadHref} download className="flt-ctl flt-btn" title={t("run.download_log")} aria-label={t("run.download_log")}>
            <Icon name="download" className="h-3.5 w-3.5" />
            <span className="flt-btn-text">{t("run.download_log")}</span>
          </a>
        </>
      }
    />
  );

  return (
    <div data-log data-wrap={wrap ? "" : undefined} className="overflow-hidden rounded-card border border-line bg-surface">
      {bar}
      {!log ? (
        <LinesSkeleton />
      ) : blocks.length === 0 ? (
        <p className={`${LINES_MIN} px-3.5 py-3 text-[12.5px] text-muted`}>{t("run.log.empty")}</p>
      ) : (
        <>
          {/* role=log is a polite live region: only the lines that are added are read. */}
          <div
            ref={box}
            role="log"
            aria-live={live ? "polite" : "off"}
            aria-relevant="additions"
            aria-label={t("run.tab.logs")}
            tabIndex={0}
            onScroll={onScroll}
            className={`${LINES_MIN} max-h-[70vh] overflow-auto pb-1 font-mono text-[12px] leading-[1.65]`}
          >
            {blocks.map((b, bi) => (
              <Fragment key={`${b.step}#${b.attempt}`}>
                <div className="sticky top-0 border-b border-line-soft bg-surface-2 px-3 py-1 text-[11.5px] text-muted">
                  {b.step} · {t("run.log.attempt", { n: b.attempt })}
                  {b.status && <> · {b.statusLabel}</>}
                  {b.exitCode !== null && <> · {t("run.log.exit", { code: b.exitCode })}</>}
                </div>
                {b.running && <div className="px-3 py-1 font-sans text-faint">{t("run.log.running", { step: b.step })}</div>}
                {(visible[bi] ?? []).map((it, i) =>
                  it.gap ? (
                    !filtering && (
                      <div key={`gap${i}`} className="px-3 py-1 font-sans text-[11.5px] text-faint italic">
                        {n("run.log.gap", it.gap)}
                      </div>
                    )
                  ) : (
                    <div
                      key={it.n}
                      id={it.cause ? "log-cause" : undefined}
                      data-level={it.level}
                      data-flash={it.cause && flash ? "" : undefined}
                      className={`grid grid-cols-[3.5rem_minmax(0,1fr)] gap-x-3 px-3 ${tone(it.level)}`}
                    >
                      <span aria-hidden="true" className="text-right text-faint tabular-nums select-none">
                        {it.n}
                      </span>
                      <span data-text className="whitespace-pre">
                        <Marked text={it.text ?? ""} q={needle} />
                      </span>
                    </div>
                  ),
                )}
              </Fragment>
            ))}
          </div>
          {live && !following && (
            <div className="flex justify-center border-t border-line-soft p-1.5">
              <button
                type="button"
                className="flt-ctl flt-btn"
                onClick={() => {
                  setFollowing(true);
                  const el = box.current;
                  if (el) el.scrollTop = el.scrollHeight;
                }}
              >
                {t("run.live.following")} · {number(log?.lines ?? 0)}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
