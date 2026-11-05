import { Link } from "react-router";
import { useT } from "../i18n";

/**
 * The rows of the last loads as a small line, oldest on the left. A load that
 * did not count leaves a gap, never a zero: absent is not zero.
 */
export function Sparkline({ values }: { values: (number | null)[] }) {
  const { t } = useT();
  const known = values.filter((v): v is number => v !== null);
  if (known.length < 2) return <span className="text-xs text-muted">—</span>;
  const [w, h, pad] = [120, 24, 2];
  const top = Math.max(...known);
  const step = (w - 2 * pad) / (values.length - 1);
  const y = (v: number) => (top === 0 ? h - pad : h - pad - (v / top) * (h - 2 * pad));
  let d = "";
  let pen = false;
  let last: [number, number] = [0, 0];
  values.forEach((v, i) => {
    if (v === null) {
      pen = false;
      return;
    }
    const x = pad + i * step;
    d += `${pen ? "L" : "M"}${x.toFixed(1)} ${y(v).toFixed(1)} `;
    pen = true;
    last = [x, y(v)];
  });
  return (
    <svg className="text-ink-2" width={w} height={h} viewBox={`0 0 ${w} ${h}`} role="img" aria-label={t("bars.label")}>
      <path d={d.trim()} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
      <circle cx={last[0]} cy={last[1]} r="2" fill="currentColor" />
    </svg>
  );
}

export type Bar = { href: string; rows: number | null; title: string };

/**
 * A destination's loads as bars, oldest first, each a link to its run, then
 * `missed` dashed slots. Outlined = uncounted (present, not zero); zero is red.
 * `size` is the height class: 44px alone, 26px inside a KPI cell.
 */
export function LoadBars({ bars, missed, size = "h-11" }: { bars: Bar[]; missed: number; size?: string }) {
  const { t } = useT();
  const n = bars.length + missed;
  if (n === 0) return <p className="text-xs text-muted">{t("bars.none")}</p>;
  const [height, gap] = [96, 3];
  const width = Math.max(200, n * 14 + (n - 1) * gap);
  const bw = (width - gap * (n - 1)) / n;
  const top = Math.max(0, ...bars.map((b) => b.rows ?? 0));
  return (
    <svg
      className={size}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={t("bars.label")}
      style={{ width: `min(100%, ${Math.round((width * 25) / 17)}px)` }}
    >
      {bars.map((b, i) => {
        const x = i * (bw + gap);
        const h = b.rows === null ? 0 : top > 0 ? Math.max(2, (b.rows / top) * (height - 2)) : 2;
        return (
          <Link key={i} to={b.href}>
            <title>{b.title}</title>
            {b.rows === null ? (
              <rect x={x} y={1} width={bw} height={height - 2} fill="none" stroke="currentColor" strokeWidth="1" className="text-line-strong" />
            ) : (
              <rect x={x} y={height - h} width={bw} height={h} className={b.rows === 0 ? "fill-state-failed" : "fill-bar"} />
            )}
          </Link>
        );
      })}
      {Array.from({ length: missed }, (_, i) => (
        <rect key={`m${i}`} x={(bars.length + i) * (bw + gap)} y={1} width={bw} height={height - 2} fill="none" stroke="currentColor" strokeWidth="1" strokeDasharray="3 2" className="text-state-failed" />
      ))}
    </svg>
  );
}
