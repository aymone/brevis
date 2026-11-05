import { Link } from "react-router";
import { useSession } from "../api/session";
import { useT } from "../i18n";
import { Empty } from "../ui/card";
import { Skeleton } from "../ui/skeleton";

export type Bucket = { start: string; succeeded: number; failed: number; running: number; queued: number };

// web/components RunsChart, drawn the same: one stacked column per bucket in a
// 980x240 box, four grid lines, at most eight hour labels. The arithmetic is the
// server's (ceiling, bars, hourTicks), so the same runs make the same chart.
const W = 980, H = 240, LEFT = 34, BOTTOM = 26;
const STEPS = [1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000];

function ceiling(max: number): number {
  if (max === 0) return 4;
  for (const s of STEPS) if (s * 4 >= max) return s * 4;
  return Math.ceil(max / 4000) * 4000;
}


const LEGEND = (t: (k: string) => string) => (
  <div className="mt-3 flex flex-wrap gap-4 text-xs text-muted">
    {([["success", "chart.success"], ["failed", "chart.failed"], ["running", "chart.running"]] as const).map(([k, key]) => (
      <span key={k} className="inline-flex items-center gap-1.5">
        <span className="h-2 w-2 rounded-full" style={{ background: `var(--color-state-${k})` }} />
        {t(key)}
      </span>
    ))}
  </div>
);

/** The plot is data and waits as a block of its own height; the legend is not and never does. */
export function RunsChart({ buckets, grain }: { buckets?: Bucket[]; grain: "hour" | "day" }) {
  const { t } = useT();
  const { lang } = useSession();
  const title = grain === "day" ? t("chart.per_day") : t("chart.per_hour");
  if (!buckets)
    return (
      <>
        <Skeleton h={H} />
        {LEGEND(t)}
      </>
    );
  if (buckets.length === 0) return <Empty text={t("chart.empty")} hint="brevis scheduler" />;

  const day = grain === "day";
  const span = day ? 86_400_000 : 3_600_000;
  const util = H - BOTTOM;
  const top = ceiling(Math.max(...buckets.map((b) => b.succeeded + b.failed + b.running + b.queued)));
  const step = (W - LEFT) / buckets.length;
  const width = Math.max(2, Math.floor(step * 0.62));
  // English writes the hour as "22h"; the other languages the way their clock labels one ("22 h", "22時", "22시").
  const hourOf = new Intl.DateTimeFormat(lang, { hour: "2-digit", hourCycle: "h23" });
  const hour = (iso: string) => (lang.startsWith("en") ? `${String(new Date(iso).getHours()).padStart(2, "0")}h` : hourOf.format(new Date(iso)));
  const label = (iso: string) => (day ? new Date(iso).toLocaleDateString(lang, { month: "short", day: "numeric", timeZone: "UTC" }) : hour(iso));
  const tip = (b: Bucket) => {
    const slice = day ? label(b.start) : `${hour(b.start)}–${hour(new Date(new Date(b.start).getTime() + span).toISOString())}`;
    return t("chart.tip", { slice, ok: b.succeeded, failed: b.failed, running: b.running + b.queued });
  };
  const link = (b: Bucket) => {
    const iso = (ms: number) => new Date(ms).toISOString().replace(".000", "");
    const from = new Date(b.start).getTime();
    return `/runs?${new URLSearchParams({ from: iso(from), to: iso(from + span) })}`;
  };
  const h = (n: number) => (n === 0 ? 0 : Math.max(2, Math.floor((n * util) / top)));
  const skip = Math.floor(buckets.length / 8) + 1;

  return (
    <>
      <div className="relative overflow-x-auto">
        <svg viewBox={`0 0 ${W} ${H}`} className="h-[240px] w-full min-w-[560px]" role="img" aria-label={title}>
          {[0, 1, 2, 3, 4].map((i) => {
            const y = util - Math.floor((util * i) / 4);
            return (
              <g key={i}>
                <line x1={LEFT} y1={y} x2={W} y2={y} stroke="var(--color-ink)" strokeOpacity="0.08" />
                <text x="0" y={y + 4} className="fill-muted text-[11px]">{Math.floor((top * i) / 4)}</text>
              </g>
            );
          })}
          {buckets.map((b, i) => {
            const band = LEFT + Math.floor(i * step);
            const x = band + Math.floor((Math.floor(step) - width) / 2);
            const hs = h(b.succeeded), hf = h(b.failed), ho = h(b.running + b.queued);
            const ys = util - hs, yf = ys - hf, yo = yf - ho;
            return (
              <Link key={b.start} to={link(b)} className="chart-target">
                <rect className="chart-band" x={band} y="0" width={Math.floor(step)} height={H - BOTTOM} rx="3" />
                <rect x={x} y={ys} width={width} height={hs} rx="2" fill="var(--color-state-success)" />
                <rect x={x} y={yf} width={width} height={hf} rx="2" fill="var(--color-state-failed)" />
                <rect x={x} y={yo} width={width} height={ho} rx="2" fill="var(--color-state-running)" />
                <title>{tip(b)}</title>
              </Link>
            );
          })}
          {buckets.map((b, i) =>
            i % skip === 0 ? (
              <text key={b.start} x={LEFT + Math.floor(i * step + step / 2)} y={H - 6} textAnchor="middle" className="fill-muted text-[11px]">
                {label(b.start)}
              </text>
            ) : null,
          )}
        </svg>
      </div>
      {LEGEND(t)}
    </>
  );
}
