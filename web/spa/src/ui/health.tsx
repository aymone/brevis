import type { StreamHealth } from "../api/ingest";
import { useFormat, useT } from "../i18n";
import { useCounts } from "./metrics/hooks";
import { Status } from "./status";

/** A stream's health state as a status kind; a server that predates the field says nothing, which is "unknown". */
export const healthKind = (h?: StreamHealth) => h?.state || "unknown";

/** The order states are listed in: what needs a person first. */
export const HEALTH_ORDER = ["failing", "degraded", "healthy", "quiet", "unknown"];

/** Failing streams first, then degraded, then the rest; each group keeps the order it came in. */
const SEVERITY: Record<string, number> = { failing: 0, degraded: 1 };
export const severity = (h?: StreamHealth) => SEVERITY[healthKind(h)] ?? 2;

/** The sentence for a health reason, with the numbers that justify it. Every key is written out so the i18n scan sees it. */
export function useHealthReason() {
  const { t, n, lang } = useT();
  const { clock } = useFormat();
  const { count } = useCounts();
  // Whole percents: "56%" is the claim; the decimal would be precision the window does not have.
  const pct = new Intl.NumberFormat(lang, { style: "percent", maximumFractionDigits: 0 });
  return (h?: StreamHealth) => {
    const minutes = h?.windowMinutes ?? 5;
    const w = h?.window;
    const p = (r?: number) => pct.format(r ?? 0);
    switch (h?.reason) {
      case "destination_unnamed":
        return t("streams.health.destination_unnamed");
      case "source_not_configured":
        return t("streams.health.source_not_configured");
      case "source_unreachable":
        return t("streams.health.source_unreachable");
      case "waiting":
        return t("streams.health.waiting");
      case "not_in_source":
        return t("streams.health.not_in_source");
      case "no_traffic":
        return t("streams.health.no_traffic", { minutes, at: h?.since ? clock(h.since) : "—" });
      case "no_traffic_seen":
        return n("streams.health.no_traffic_seen", h?.observedMinutes ?? 0, { count: count(h?.observedMinutes ?? 0) });
      case "delivering":
        return t("streams.health.delivering", { minutes });
      case "dead_lettered":
        return (w?.deadLetter ?? 0) > 0
          ? n("streams.health.dead_lettered", w?.deadLetter ?? 0, { count: count(w?.deadLetter ?? 0), minutes })
          : n("streams.health.buried", w?.buried ?? 0, { count: count(w?.buried ?? 0), minutes });
      case "rejected_some":
        return t("streams.health.rejected_some", { percent: p(h?.rejectedRatio), minutes });
      case "saturated":
        return n("streams.health.saturated", w?.saturated ?? 0, { count: count(w?.saturated ?? 0), minutes });
      case "retrying":
        return t("streams.health.retrying", { percent: p(h?.retryRatio), minutes });
      case "slow_delivery":
        return t("streams.health.slow_delivery", { seconds: Math.round(h?.latencyP95Sec ?? 0), minutes });
      case "replicas_partial":
        return t("streams.health.replicas_partial", { reachable: h?.reachable ?? 0, replicas: h?.replicas ?? 0 });
      case "rejected_most":
        return t("streams.health.rejected_most", { percent: p(h?.rejectedRatio), minutes });
      case "dead_letter_most":
        return t("streams.health.dead_letter_most", { percent: p(h?.deadLetterRatio), minutes });
      case "delivery_failing":
        return t("streams.health.delivery_failing", { percent: p(h?.failedBatchRatio), minutes });
      case "saturating":
        return t("streams.health.saturating", { minutes });
      default:
        return t("streams.health.not_reported");
    }
  };
}

/** "since 10:12" for the states that have a beginning; "quiet" says it in its sentence. */
export function useHealthSince() {
  const { t } = useT();
  const { clock } = useFormat();
  return (h?: StreamHealth) => (h?.since && h.state !== "quiet" && h.state !== "unknown" ? t("streams.health.since", { at: clock(h.since) }) : "");
}

/** The mark and word of a stream's health, the reason as its tooltip. */
export function HealthStatus({ health }: { health?: StreamHealth }) {
  const reason = useHealthReason()(health);
  const since = useHealthSince()(health);
  return (
    <span title={since ? `${reason} (${since})` : reason}>
      <Status kind={healthKind(health)} />
    </span>
  );
}
