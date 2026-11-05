import { useLayoutEffect, useRef } from "react";
import { Link, useSearchParams } from "react-router";
import { useFormat, useT } from "../i18n";
import { Panel, linkClass } from "../ui/card";
import { SK_REAL, Skeleton } from "../ui/skeleton";
import type { WorkflowDetail } from "../api/workflows";

// web/components Heatmap: a year of days, one square each, colored by the day's
// worst outcome. A day that ran is a link to its runs; the day chosen has a ring.
const CELL = 12, STEP = 15, LEFT = 30, TOP = 16, WEEKS = 53;
const WIDTH = LEFT + WEEKS * STEP - 3;
const HEIGHT = TOP + 7 * STEP - 3;

// "Tue, Mar 3, 2026": a day, in UTC like every slot.
function useDayName() {
  const { lang } = useT();
  const f = new Intl.DateTimeFormat(lang, { weekday: "short", day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
  return (d: Date | string) => f.format(new Date(d));
}

function Plot({ d }: { d: WorkflowDetail }) {
  const { t, n, lang } = useT();
  const dayName = useDayName();
  const [sp] = useSearchParams();
  const selected = sp.get("day");
  const byDay = new Map(d.calendar.days.map((x) => [x.date, x]));
  const today = new Date(d.now);
  today.setUTCHours(0, 0, 0, 0);
  const monday = new Date(today);
  monday.setUTCDate(monday.getUTCDate() - ((today.getUTCDay() + 6) % 7));
  const start = new Date(monday);
  start.setUTCDate(start.getUTCDate() - 7 * (WEEKS - 1));
  const month = new Intl.DateTimeFormat(lang, { month: "short", timeZone: "UTC" });
  const weekday = new Intl.DateTimeFormat(lang, { weekday: "short", timeZone: "UTC" });

  const squares: React.ReactNode[] = [];
  const months: React.ReactNode[] = [];
  const used = new Set<string>();
  for (let i = 0; i < WEEKS * 7; i++) {
    const day = new Date(start);
    day.setUTCDate(day.getUTCDate() + i);
    const col = Math.floor(i / 7);
    const row = i % 7;
    const key = day.toISOString().slice(0, 10);
    const x = LEFT + col * STEP;
    const y = TOP + row * STEP;
    if (row === 0 && day.getUTCDate() <= 7) {
      const m = month.format(day);
      if (!used.has(m)) {
        used.add(m);
        months.push(<text key={`m${i}`} x={x} y={10} className="fill-muted text-[10px]">{m}</text>);
      }
    }
    const info = byDay.get(key);
    const ran = !!info && info.total > 0;
    const when = dayName(day);
    const fill = !ran ? "var(--color-surface-2)" : info.failed > 0 ? "var(--color-state-failed)" : info.open > 0 ? "var(--color-state-queued)" : "var(--color-state-success)";
    const tip = !ran
      ? `${when} · ${t("calendar.no_run")}`
      : `${when} · ${n("calendar.runs", info.total)} · ${info.failed > 0 ? n("calendar.failed", info.failed) : info.open > 0 ? t("calendar.open") : t("calendar.ok")}`;
    const opacity = day > today ? 0.35 : ran ? 1 : 0.8;
    squares.push(
      ran ? (
        <Link key={key} to={`?day=${key}#runs`} data-day={key}>
          <rect x={x} y={y} width={CELL} height={CELL} rx={2} fill={fill} fillOpacity={opacity} stroke={selected === key ? "var(--color-ink)" : undefined} strokeWidth={selected === key ? 2 : undefined} />
          <title>{tip}</title>
        </Link>
      ) : (
        <rect key={key} x={x} y={y} width={CELL} height={CELL} rx={2} fill={fill} fillOpacity={opacity} stroke="var(--color-line)" strokeWidth={0.5}>
          <title>{tip}</title>
        </rect>
      ),
    );
  }
  const weekdays = [0, 2, 4].map((r) => {
    const day = new Date(monday);
    day.setUTCDate(day.getUTCDate() + r);
    return <text key={r} x={0} y={TOP + r * STEP + CELL - 2} className="fill-muted text-[10px]">{weekday.format(day)}</text>;
  });
  return (
    <svg width={WIDTH} height={HEIGHT} viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={t("calendar.label")} data-calendar>
      {months}
      {weekdays}
      {squares}
    </svg>
  );
}

function Summary({ d }: { d: WorkflowDetail }) {
  const { t, n } = useT();
  const { number } = useFormat();
  const c = d.calendar;
  const day = useDayName();
  return (
    <p data-calendar-summary className="mb-3 flex flex-wrap items-baseline gap-x-5 gap-y-1 text-[12.5px] text-muted">
      <span><strong className="font-semibold text-ink tabular-nums">{number(c.ranDays)}</strong> {n("calendar.ran_days", c.ranDays)}</span>
      <span><strong className="font-semibold text-ink tabular-nums">{number(c.failedDays)}</strong> {t("calendar.failed_days")}</span>
      {c.ranDays > 0 && <span><strong className="font-semibold text-ink tabular-nums">{number(c.streak)}</strong> {n("calendar.streak", c.streak)}</span>}
      {c.lastFailure && (
        <span>
          {t("calendar.last_failure")} <strong className="font-medium text-ink">{day(c.lastFailure)}</strong>
          <Link to={`?day=${c.lastFailure.slice(0, 10)}#runs`} className={`ml-1 ${linkClass}`}>{t("calendar.see")}</Link>
        </span>
      )}
      {c.since && <span>{t("calendar.since")} <strong className="font-medium text-ink">{day(c.since)}</strong></span>}
    </p>
  );
}

// A typical year's summary, drawn hidden while the real one loads, so it wraps like the loaded line at any width.
const SUMMARY_PLACEHOLDER = { calendar: { ranDays: 12, failedDays: 3, streak: 2, lastFailure: "", since: "2026-01-01" } } as unknown as WorkflowDetail;

/** The year's panel: its words and legend are the page's; the summary and the squares are data. */
export function YearOfRuns({ d }: { d?: WorkflowDetail }) {
  const { t } = useT();
  // A year is wider than a phone: where it scrolls it opens at this week, as the server page does.
  const box = useRef<HTMLDivElement>(null);
  const placed = useRef(false);
  useLayoutEffect(() => {
    const el = box.current;
    if (!d || !el || placed.current) return;
    placed.current = true;
    if (el.scrollWidth > el.clientWidth) el.scrollLeft = el.scrollWidth;
  }, [d]);
  return (
    <Panel title={t("calendar.title")}>
      <div className="px-3.5 py-3">
        <p className="mb-2 text-[12.5px] text-muted">{t("calendar.lead")}</p>
        {d ? <Summary d={d} /> : <div aria-hidden="true" className={SK_REAL} style={{ marginBottom: 0 }}><Summary d={SUMMARY_PLACEHOLDER} /></div>}
        <div ref={box} className="max-w-full overflow-x-auto">{d ? <Plot d={d} /> : <Skeleton w={WIDTH} h={HEIGHT} />}</div>
        <div className="mt-3 flex flex-wrap items-center gap-4 text-xs text-muted">
          {([["var(--color-state-success)", "calendar.ok"], ["var(--color-state-failed)", "calendar.some_failed"], ["var(--color-state-queued)", "calendar.open"], ["var(--color-surface-2)", "calendar.no_run"]] as const).map(([c, k]) => (
            <span key={k} className="inline-flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full" style={{ background: c }} />
              {t(k)}
            </span>
          ))}
          <span className="ml-auto">{t("calendar.click")}</span>
        </div>
      </div>
    </Panel>
  );
}
