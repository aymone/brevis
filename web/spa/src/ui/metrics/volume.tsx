import type { ReactNode } from "react";
import { useT } from "../../i18n";
import { Card } from "../kit";
import { Ghost } from "../skeleton";
import { Trend } from "./charts";
import { Footnote, Notice, type Family } from "./notice";
import { health, useCounts, type Health } from "./hooks";
import type { Entity, Series, Snapshot } from "./types";
import "./metrics.css";

/** A figure of the band: a count, or `text` when it is a ratio. null is "no basis" (a dash), never zero. */
export type Cell = { label: string; value?: number | null; text?: string | null; hot?: boolean };

/**
 * The consolidated band above a list: the rate now with the last 30 minutes beside it, and the last hour's totals.
 * Static words are always drawn; only the numbers are ghosts while the first answer is on its way. The box has one
 * fixed minimum height, so the honest states (not configured, waiting, down) take the room the chart takes.
 */
export function Strip({ family, snap, failed, unit, title, rateNow, values, cells, trendLabel }: {
  family: Family;
  snap: Snapshot<unknown> | undefined;
  failed: boolean;
  unit: string;
  title: string;
  rateNow: number | null;
  values: Series;
  cells: Cell[];
  trendLabel: string;
}) {
  const { t } = useT();
  const { rate, count } = useCounts();
  const ghost = !snap && !failed;
  const h: Health = snap ? health(snap.configured, snap.owners) : { kind: "waiting" };
  return (
    <section className="mx-strip" aria-label={title} aria-busy={ghost || undefined}>
      <div className="mx-main">
        <p className="mx-label">
          {title} <span className="text-faint">· {t("metrics.now")}</span>
        </p>
        {failed && !snap ? (
          <p role="alert" className="mx-notice">{t("metrics.fetch_failed")}</p>
        ) : ghost || h.kind === "ready" ? (
          <>
            <p className="mx-big">
              {ghost ? <Ghost>000</Ghost> : <span>{rateNow === null ? "—" : rate(rateNow)}</span>}
              <span className="mx-unit">{unit}</span>
            </p>
            <div className="mx-spark">
              {ghost ? <span className="sk mx-spark-ghost" aria-hidden="true" /> : <Trend values={values} label={trendLabel} size={[240, 36]} />}
              <span className="mx-spark-cap">{t("metrics.last_minutes", { minutes: 30 })}</span>
            </div>
          </>
        ) : (
          <Notice family={family} h={h} />
        )}
      </div>
      <dl className="mx-cells" aria-hidden={!ghost && h.kind !== "ready" ? true : undefined}>
        {cells.map((c) => (
          <div key={c.label}>
            <dt title={c.label}>{c.label}</dt>
            <dd className={c.hot && c.value ? "mx-hot" : undefined}>
              {ghost ? <Ghost>000</Ghost> : h.kind !== "ready" || (c.text ?? c.value ?? null) === null ? "—" : (c.text ?? count(c.value as number))}
            </dd>
          </div>
        ))}
      </dl>
      <Footnote snap={snap} h={h} />
    </section>
  );
}

/**
 * A row's quiet cell: the rate in the last minute and a small chart of the last 30. "—" says the source has
 * nothing for this row (not configured, down, or the name does not match); it is never drawn as zero.
 */
export function RowRate({ snap, entity, unit, rateNow, values, label }: { snap: Snapshot<unknown> | undefined; entity: Entity | undefined; unit: string; rateNow: number | null; values: Series; label: (rate: string, peak: string) => string }) {
  const { t } = useT();
  const { rate, count } = useCounts();
  if (!snap) return <span className="mx-row" aria-hidden="true"><Ghost>00</Ghost></span>;
  const h = health(snap.configured, snap.owners);
  if (h.kind !== "ready" || !entity?.present || values.length === 0) {
    const why = !snap.configured ? t("metrics.nc.title") : h.kind === "ready" ? t("metrics.not_in_source") : t(h.kind === "waiting" ? "metrics.waiting" : "metrics.unreachable");
    return <span className="mx-row mx-off" title={why}>—</span>;
  }
  const r = rateNow;
  const peak = Math.max(0, ...values.map((v) => v ?? 0));
  const said = label(r === null ? "—" : rate(r), count(peak));
  return (
    <span className="mx-row" title={said}>
      <span className="mx-rate">{r === null ? "—" : rate(r)}<small>{unit}</small></span>
      <Trend values={values} label={said} />
    </span>
  );
}

export type Fig = { label: string; value: ReactNode; hot?: boolean };

/**
 * A detail screen's volume card. ONE SHAPE IN EVERY STATE: the figures grid (its labels are static words), a chart
 * box and a latency box of fixed heights, and the footnote. The honest states (not configured, waiting, down, not
 * reported) are drawn INSIDE the chart box, so nothing below the card moves when the answer arrives.
 */
export function Panel({ family, title, snap, failed, entity, figs, chart, latency, quiet }: {
  family: Family;
  title: string;
  snap: Snapshot<unknown> | undefined;
  failed: boolean;
  entity: Entity | undefined;
  figs: Fig[];
  chart: ReactNode;
  latency: ReactNode;
  quiet: boolean;
}) {
  const { t } = useT();
  const ghost = !snap && !failed;
  const h: Health = snap ? health(snap.configured, snap.owners) : { kind: "waiting" };
  const shown = !ghost && h.kind === "ready" && !!entity?.present;
  const box = (body: ReactNode) => <div className="mx-box">{body}</div>;
  let main: ReactNode;
  if (ghost) main = box(<span className="sk mx-fill" aria-hidden="true" />);
  else if (failed && !snap) main = box(<p role="alert" className="mx-notice">{t("metrics.fetch_failed")}</p>);
  else if (h.kind !== "ready") main = box(<Notice family={family} h={h} />);
  else if (!entity?.present)
    main = box(
      <div className="mx-notice" role="status">
        <p className="mx-notice-title">{t("metrics.not_in_source")}</p>
        <p>{t(`metrics.not_in_source.${family}`)}</p>
      </div>,
    );
  else main = box(chart);
  return (
    <Card title={title} count={t("metrics.last_minutes", { minutes: 60 })}>
      <div className="mx-panel" aria-busy={ghost || undefined}>
        <dl className="mx-figs">
          {figs.map((f) => (
            <div key={f.label} className="mx-fig">
              <dt title={f.label}>{f.label}</dt>
              <dd className={shown && f.hot ? "mx-hot" : undefined}>{ghost ? <Ghost>000</Ghost> : shown ? f.value : "—"}</dd>
            </div>
          ))}
        </dl>
        {main}
        <p className="mx-quiet">{shown && quiet ? t("metrics.no_traffic") : ""}</p>
        <div className="mx-box mx-box-lat">{ghost ? <span className="sk mx-fill" aria-hidden="true" /> : shown ? latency : null}</div>
        <Footnote snap={snap} h={h} />
      </div>
    </Card>
  );
}
