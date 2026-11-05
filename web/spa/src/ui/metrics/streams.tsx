import { useT } from "../../i18n";
import { Lines, MinuteBars } from "./charts";
import { Panel, RowRate, Strip } from "./volume";
import { entityOf, useCounts, useStreamMetrics, useStreamsMetrics } from "./hooks";
import { useFormat } from "../../i18n";
import type { Entity } from "./types";

const COLOR = {
  received: "var(--color-state-running)",
  rejected: "var(--color-state-failed)",
  dropped: "var(--color-state-queued)",
  deadLetter: "var(--color-state-retrying)",
  p50: "var(--color-state-success)",
  p95: "var(--color-state-queued)",
};

/** The Streams list's band: events per minute across every stream the sources report. */
export function StreamsStrip() {
  const { t } = useT();
  const { rate, count } = useCounts();
  const q = useStreamsMetrics();
  const e = q.data?.total;
  const r = e?.ratePerMin.received ?? null;
  const values = e?.values.received ?? Array.from({ length: 30 }, () => null);
  const peak = Math.max(0, ...values.map((v) => v ?? 0));
  const hr = e?.lastHour ?? {};
  return (
    <Strip
      family="streams"
      snap={q.data}
      failed={q.isError}
      unit={t("metrics.unit.events")}
      title={t("metrics.streams.title")}
      rateNow={r}
      values={values}
      trendLabel={t("metrics.streams.trend", { rate: r === null ? "—" : rate(r), peak: count(peak) })}
      cells={[
        { label: t("metrics.m.received_hour"), value: hr.received ?? 0 },
        { label: t("metrics.m.delivered_hour"), value: hr.delivered ?? 0 },
        { label: t("metrics.m.rejected_hour"), value: hr.rejected ?? 0, hot: true },
        { label: t("metrics.m.dead_letter_hour"), value: hr.deadLetter ?? 0, hot: true },
      ]}
    />
  );
}

/** A row of the Streams list: the stream is found by its gateway and path, the keys the console knows it by. */
export function StreamRowRate({ gateway, path }: { gateway: string; path: string }) {
  const { t } = useT();
  const q = useStreamsMetrics();
  const e = q.data?.entities.find((x) => x.gateway === gateway && x.path === path);
  return (
    <RowRate
      snap={q.data}
      entity={e}
      unit={t("metrics.unit.events_short")}
      rateNow={e?.ratePerMin.received ?? null}
      values={e?.values.received ?? []}
      label={(rate, peak) => t("metrics.streams.trend", { rate, peak })}
    />
  );
}

/** The stream's page: the last 60 minutes of what was sent to it and what became of it. */
export function StreamPanel({ gateway, path }: { gateway: string; path: string }) {
  const { t } = useT();
  const f = useFormat();
  const { rate, count } = useCounts();
  const q = useStreamMetrics(gateway, path);
  const snap = q.data;
  const e: Entity | undefined = entityOf(snap);
  const hr = e?.lastHour ?? {};
  const lat = e?.latency;
  const sec = (v: number | null | undefined) => (v == null ? "—" : f.duration(v * 1000));
  const r = e?.ratePerMin.received ?? null;
  const val = (k: string) => count(hr[k] ?? 0);
  const g = e?.gauges;
  const latency =
    e?.latencyP50Sec && lat && lat.samples > 0 ? (
      <Lines
        layers={[
          { key: "p50", label: t("metrics.m.p50"), color: COLOR.p50, values: e.latencyP50Sec },
          { key: "p95", label: t("metrics.m.p95"), color: COLOR.p95, values: e.latencyP95Sec ?? [] },
        ]}
        start={snap!.window.start}
        label={t("metrics.latency.chart", { p50: sec(lat.p50Sec), p95: sec(lat.p95Sec) })}
        caption={t("metrics.latency.caption")}
      />
    ) : (
      <p className="mx-quiet">{t("metrics.latency.none")}</p>
    );
  return (
    <Panel
      family="streams"
      title={t("metrics.stream.title")}
      snap={snap}
      failed={q.isError}
      entity={e}
      quiet={!!e && e.observedSec > 0 && Object.values(hr).every((v) => v === 0)}
      figs={[
        { label: t("metrics.m.rate_now"), value: <>{r === null ? "—" : rate(r)} <small>{t("metrics.unit.events")}</small></> },
        { label: t("metrics.m.received_hour"), value: val("received") },
        { label: t("metrics.m.delivered_hour"), value: val("delivered") },
        { label: t("metrics.m.rejected_hour"), value: val("rejected"), hot: (hr.rejected ?? 0) > 0 },
        { label: t("metrics.m.dropped_hour"), value: val("dropped"), hot: (hr.dropped ?? 0) > 0 },
        { label: t("metrics.m.dead_letter_hour"), value: val("deadLetter"), hot: (hr.deadLetter ?? 0) > 0 },
        { label: t("metrics.m.buffered"), value: g ? count(g.buffered ?? 0) : "—" },
        { label: t("metrics.m.queued"), value: g ? count(g.queued ?? 0) : "—" },
        { label: t("metrics.m.delivery_latency"), value: lat && lat.p50Sec !== null ? `${sec(lat.p50Sec)} · ${sec(lat.p95Sec)}` : "—" },
      ]}
      chart={
        e && snap ? (
          <MinuteBars
            layers={[
              { key: "received", label: t("metrics.m.received"), color: COLOR.received, values: e.values.received ?? [] },
              { key: "rejected", label: t("metrics.m.rejected"), color: COLOR.rejected, values: e.values.rejected ?? [] },
              { key: "dropped", label: t("metrics.m.dropped"), color: COLOR.dropped, values: e.values.dropped ?? [] },
              { key: "deadLetter", label: t("metrics.m.dead_letter"), color: COLOR.deadLetter, values: e.values.deadLetter ?? [] },
            ]}
            start={snap.window.start}
            label={t("metrics.stream.chart", { rate: r === null ? "—" : rate(r), hour: count(hr.received ?? 0) })}
            caption={t("metrics.stream.caption")}
          />
        ) : null
      }
      latency={latency}
    />
  );
}

