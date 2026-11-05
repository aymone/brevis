import { useT, useFormat } from "../../i18n";
import { Lines, MinuteBars } from "./charts";
import { Panel, RowRate, Strip } from "./volume";
import { entityOf, errorShare, REQUEST_KEYS, sumHour, sumRate, sumSeries, useCounts, useEndpointMetrics, useEndpointsMetrics } from "./hooks";
import type { Entity } from "./types";

const COLOR = {
  ok: "var(--color-state-success)",
  rateLimited: "var(--color-state-queued)",
  clientError: "var(--color-state-retrying)",
  serverError: "var(--color-state-failed)",
  p50: "var(--color-state-success)",
  p95: "var(--color-state-queued)",
};

/** Hits over hits and misses; null when the endpoint has no cache to hit (no basis, not zero). */
function cacheShare(e: Entity | undefined): { hit: number; all: number } {
  const hit = e?.lastHour.cacheHit ?? 0;
  return { hit, all: hit + (e?.lastHour.cacheMiss ?? 0) };
}

/** The Endpoints list's band: requests per minute across every API the sources report. */
export function EndpointsStrip() {
  const { t } = useT();
  const { rate, count, percent } = useCounts();
  const q = useEndpointsMetrics();
  const e = q.data?.total;
  const r = sumRate(e, REQUEST_KEYS);
  const values = e ? sumSeries(e, REQUEST_KEYS) : Array.from({ length: 30 }, () => null);
  const peak = Math.max(0, ...values.map((v) => v ?? 0));
  const { errors, all } = e ? errorShare(e) : { errors: 0, all: 0 };
  const cache = cacheShare(e);
  return (
    <Strip
      family="endpoints"
      snap={q.data}
      failed={q.isError}
      unit={t("metrics.unit.requests")}
      title={t("metrics.endpoints.title")}
      rateNow={r}
      values={values}
      trendLabel={t("metrics.endpoints.trend", { rate: r === null ? "—" : rate(r), peak: count(peak) })}
      cells={[
        { label: t("metrics.m.requests_hour"), value: sumHour(e, REQUEST_KEYS) },
        { label: t("metrics.m.error_rate"), text: all > 0 ? percent(errors, all) : null, hot: errors > 0, value: errors },
        { label: t("metrics.m.cache_ratio"), text: cache.all > 0 ? percent(cache.hit, cache.all) : null },
        { label: t("metrics.m.hook_failed_hour"), value: e?.lastHour.hookFailed ?? 0, hot: true },
      ]}
    />
  );
}

/** A row of the Endpoints list, found by its API and name. */
export function EndpointRowRate({ api, name }: { api: string; name: string }) {
  const { t } = useT();
  const q = useEndpointsMetrics();
  const e = q.data?.entities.find((x) => x.api === api && x.name === name);
  return (
    <RowRate
      snap={q.data}
      entity={e}
      unit={t("metrics.unit.requests_short")}
      rateNow={sumRate(e, REQUEST_KEYS)}
      values={e ? sumSeries(e, REQUEST_KEYS) : []}
      label={(rate, peak) => t("metrics.endpoints.trend", { rate, peak })}
    />
  );
}

/** The endpoint's page: the last 60 minutes of who called it and what they got. */
export function EndpointPanel({ api, name }: { api: string; name: string }) {
  const { t } = useT();
  const f = useFormat();
  const { rate, count, percent } = useCounts();
  const q = useEndpointMetrics(api, name);
  const snap = q.data;
  const e = entityOf(snap);
  const lat = e?.latency;
  const sec = (v: number | null | undefined) => (v == null ? "—" : f.duration(v * 1000));
  const r = sumRate(e, REQUEST_KEYS);
  const requests = sumHour(e, REQUEST_KEYS);
  const { errors, all } = e ? errorShare(e) : { errors: 0, all: 0 };
  const cache = cacheShare(e);
  // An endpoint with no cache never reports a hit or a miss: the ratio has no basis, and says so.
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
      family="endpoints"
      title={t("metrics.endpoint.title")}
      snap={snap}
      failed={q.isError}
      entity={e}
      quiet={!!e && e.observedSec > 0 && requests === 0}
      figs={[
        { label: t("metrics.m.rate_now"), value: <>{r === null ? "—" : rate(r)} <small>{t("metrics.unit.requests")}</small></> },
        { label: t("metrics.m.requests_hour"), value: count(requests) },
        { label: t("metrics.m.error_rate"), value: all > 0 ? percent(errors, all) : "—", hot: errors > 0 },
        { label: t("metrics.m.cache_ratio"), value: cache.all > 0 ? percent(cache.hit, cache.all) : "—" },
        { label: t("metrics.m.rate_limited_hour"), value: count(e?.lastHour.rateLimited ?? 0), hot: (e?.lastHour.rateLimited ?? 0) > 0 },
        { label: t("metrics.m.hook_failed_hour"), value: count(e?.lastHour.hookFailed ?? 0), hot: (e?.lastHour.hookFailed ?? 0) > 0 },
        { label: t("metrics.m.latency"), value: lat && lat.p50Sec !== null ? `${sec(lat.p50Sec)} · ${sec(lat.p95Sec)}` : "—" },
      ]}
      chart={
        e && snap ? (
          <MinuteBars
            layers={[
              { key: "ok", label: t("metrics.m.ok"), color: COLOR.ok, values: e.values.ok ?? [] },
              { key: "rateLimited", label: t("metrics.m.rate_limited"), color: COLOR.rateLimited, values: e.values.rateLimited ?? [] },
              { key: "clientError", label: t("metrics.m.client_error"), color: COLOR.clientError, values: e.values.clientError ?? [] },
              { key: "serverError", label: t("metrics.m.server_error"), color: COLOR.serverError, values: e.values.serverError ?? [] },
            ]}
            start={snap.window.start}
            label={t("metrics.endpoint.chart", { rate: r === null ? "—" : rate(r), hour: count(requests) })}
            caption={t("metrics.endpoint.caption")}
          />
        ) : null
      }
      latency={latency}
    />
  );
}
