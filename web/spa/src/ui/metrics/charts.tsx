import { useFormat, useT } from "../../i18n";
import type { Series } from "./types";
import { useCounts } from "./hooks";

// Hand-rolled SVG, tokens only. A bar is one minute: the last is the current minute, still filling, so it is
// drawn lighter; a minute nobody observed is a faint tick on the baseline, never a zero.

export type Layer = { key: string; label: string; color: string; values: Series };

const stackTop = (layers: Layer[], i: number) => layers.reduce((a, l) => a + (l.values[i] ?? 0), 0);

/** The row's small chart: one series, no axes, a fixed box. The caller gives the spoken summary. */
export function Trend({ values, label, color = "var(--color-state-running)", size = [90, 20] }: { values: Series; label: string; color?: string; size?: [number, number] }) {
  const [w, h] = size;
  const n = Math.max(values.length, 1);
  const bw = w / n;
  const top = Math.max(1, ...values.map((v) => v ?? 0));
  return (
    <svg className="mx-trend" width={w} height={h} viewBox={`0 0 ${w} ${h}`} role="img" aria-label={label}>
      {values.map((v, i) => {
        const x = i * bw;
        if (v === null) return <rect key={i} x={x + 0.5} y={h - 1} width={Math.max(bw - 1, 0.5)} height={1} fill="var(--color-line-strong)" />;
        const bh = v === 0 ? 1 : Math.max(2, (v / top) * (h - 1));
        return <rect key={i} x={x + 0.5} y={h - bh} width={Math.max(bw - 1, 0.5)} height={bh} fill={v === 0 ? "var(--color-line-strong)" : color} opacity={i === n - 1 ? 0.55 : 1} />;
      })}
    </svg>
  );
}

/**
 * Per-minute stacked bars with the oldest minute on the left. `start` is the first minute's Unix time in seconds.
 * The chart is an image for the eye; the table after it (visually hidden) is the same numbers for everyone else.
 */
export function MinuteBars({ layers, start, label, caption, height = 132 }: { layers: Layer[]; start: number; label: string; caption: string; height?: number }) {
  const { t } = useT();
  const { count } = useCounts();
  const f = useFormat();
  const n = layers[0]?.values.length ?? 0;
  const [bw, gap] = [10, 2];
  const w = n * (bw + gap);
  const tops = Array.from({ length: n }, (_, i) => stackTop(layers, i));
  const top = Math.max(1, ...tops);
  const clock = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
  const minuteAt = (i: number) => clock.format(new Date((start + i * 60) * 1000));
  const rows = Math.min(10, n);
  return (
    <figure className="mx-chart">
      <div className="mx-plot" style={{ height }}>
        <span className="mx-ymax" aria-hidden="true">{count(top)}</span>
        <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" role="img" aria-label={label}>
          <line x1="0" x2={w} y1="0.5" y2="0.5" className="mx-grid" />
          <line x1="0" x2={w} y1={height / 2} y2={height / 2} className="mx-grid" />
          <line x1="0" x2={w} y1={height - 0.5} y2={height - 0.5} className="mx-axis" />
          {Array.from({ length: n }, (_, i) => {
            const x = i * (bw + gap);
            const observed = layers.some((l) => l.values[i] !== null);
            let y = height;
            const text = [minuteAt(i), ...layers.map((l) => `${l.label}: ${l.values[i] === null ? "—" : count(l.values[i] as number)}`)].join("\n");
            return (
              <g key={i} opacity={i === n - 1 ? 0.6 : 1}>
                <title>{text}</title>
                <rect x={x - gap / 2} y={0} width={bw + gap} height={height} fill="transparent" />
                {!observed && <rect x={x} y={height - 2} width={bw} height={2} fill="var(--color-line-strong)" />}
                {layers.map((l) => {
                  const v = l.values[i];
                  if (!v) return null;
                  const bh = (v / top) * (height - 2);
                  y -= bh;
                  return <rect key={l.key} x={x} y={y} width={bw} height={Math.max(bh, 1)} fill={l.color} />;
                })}
              </g>
            );
          })}
        </svg>
      </div>
      <div className="mx-xaxis" aria-hidden="true">
        <span>{t("metrics.axis.ago", { minutes: n })}</span>
        <span>{t("metrics.axis.now")}</span>
      </div>
      <ul className="mx-legend">
        {layers.map((l) => (
          <li key={l.key}>
            <i style={{ background: l.color }} aria-hidden="true" />
            {l.label}
          </li>
        ))}
      </ul>
      <table className="sr-only">
        <caption>{caption}</caption>
        <thead>
          <tr>
            <th scope="col">{t("metrics.table.minute")}</th>
            {layers.map((l) => (
              <th key={l.key} scope="col">{l.label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }, (_, k) => n - 1 - k).map((i) => (
            <tr key={i}>
              <th scope="row">{f.dateTime(new Date((start + i * 60) * 1000).toISOString())}{i === n - 1 ? ` (${t("metrics.axis.now")})` : ""}</th>
              {layers.map((l) => (
                <td key={l.key}>{l.values[i] === null ? t("metrics.not_observed") : count(l.values[i] as number)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}

/** Latency percentiles per minute as lines; a minute without requests breaks the line. Values are seconds. */
export function Lines({ layers, start, label, caption, height = 84 }: { layers: Layer[]; start: number; label: string; caption: string; height?: number }) {
  const { t } = useT();
  const f = useFormat();
  const n = layers[0]?.values.length ?? 0;
  const w = n * 12;
  const top = Math.max(0.001, ...layers.flatMap((l) => l.values.map((v) => v ?? 0)));
  const ms = (sec: number) => f.duration(sec * 1000);
  const x = (i: number) => i * 12 + 6;
  const y = (v: number) => height - 3 - (v / top) * (height - 8);
  const rows = Math.min(10, n);
  return (
    <figure className="mx-chart">
      <div className="mx-plot" style={{ height }}>
        <span className="mx-ymax" aria-hidden="true">{ms(top)}</span>
        <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" role="img" aria-label={label}>
          <line x1="0" x2={w} y1="0.5" y2="0.5" className="mx-grid" />
          <line x1="0" x2={w} y1={height - 0.5} y2={height - 0.5} className="mx-axis" />
          {layers.map((l) => {
            let d = "";
            let pen = false;
            l.values.forEach((v, i) => {
              if (v === null) {
                pen = false;
                return;
              }
              d += `${pen ? "L" : "M"}${x(i)} ${y(v).toFixed(1)} `;
              pen = true;
            });
            return <path key={l.key} d={d.trim()} fill="none" stroke={l.color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />;
          })}
        </svg>
      </div>
      <div className="mx-xaxis" aria-hidden="true">
        <span>{t("metrics.axis.ago", { minutes: n })}</span>
        <span>{t("metrics.axis.now")}</span>
      </div>
      <ul className="mx-legend">
        {layers.map((l) => (
          <li key={l.key}>
            <i style={{ background: l.color }} aria-hidden="true" />
            {l.label}
          </li>
        ))}
      </ul>
      <table className="sr-only">
        <caption>{caption}</caption>
        <thead>
          <tr>
            <th scope="col">{t("metrics.table.minute")}</th>
            {layers.map((l) => (
              <th key={l.key} scope="col">{l.label}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }, (_, k) => n - 1 - k).map((i) => (
            <tr key={i}>
              <th scope="row">{f.dateTime(new Date((start + i * 60) * 1000).toISOString())}</th>
              {layers.map((l) => (
                <td key={l.key}>{l.values[i] === null ? t("metrics.not_observed") : ms(l.values[i] as number)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
