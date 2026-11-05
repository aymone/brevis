import { useFormat, useT } from "../../i18n";
import { useSince } from "../format";
import { CopyButton } from "../copy";
import type { Health } from "./hooks";
import type { Snapshot } from "./types";

// THE HONEST STATES. A chart of zeros would be a claim, so each thing the screen cannot know says so: no source
// configured (with the variable to set, copyable), nothing scraped yet, the source down (and when it last answered).

/** What to set, per family: the variable and an example value in its own format. */
const SETUP = {
  streams: { variable: "BREVIS_GATEWAY_METRICS_URLS", example: "events_gateway=http://gateway:9090/metrics" },
  endpoints: { variable: "BREVIS_ENDPOINTS_METRICS_URLS", example: "example-api=http://endpoints:9090/metrics" },
} as const;

export type Family = keyof typeof SETUP;

export function Notice({ family, h }: { family: Family; h: Exclude<Health, { kind: "ready" }> }) {
  const { t } = useT();
  const since = useSince();
  if (h.kind === "not_configured") {
    const line = `${SETUP[family].variable}=${SETUP[family].example}`;
    return (
      <div className="mx-notice" role="status">
        <p className="mx-notice-title">{t("metrics.nc.title")}</p>
        <p>{t(`metrics.nc.${family}`)}</p>
        <div className="mx-copy">
          <code>{line}</code>
          <CopyButton text={line} label={t("metrics.copy")} />
        </div>
      </div>
    );
  }
  if (h.kind === "waiting")
    return (
      <div className="mx-notice" role="status">
        <p className="mx-notice-title">{t("metrics.waiting")}</p>
        <p>{t("metrics.waiting.text")}</p>
      </div>
    );
  return (
    <div className="mx-notice" role="status">
      <p className="mx-notice-title">{t("metrics.unreachable")}</p>
      {h.owners.map((o) => (
        <p key={o.name}>
          <b className="font-mono text-[12px]">{o.name}</b>{" "}
          {o.lastSuccess ? t("metrics.unreachable.last", { when: since(o.lastSuccess) }) : t("metrics.unreachable.never")}
          {o.error ? ` · ${t(`metrics.error.${o.error}`)}` : ""}
        </p>
      ))}
    </div>
  );
}

/** The footnote of a working source: since when it is counted, replicas missing, series left out. One line, always there. */
export function Footnote({ snap, h }: { snap: Snapshot<unknown> | undefined; h: Health }) {
  const { t } = useT();
  const f = useFormat();
  if (!snap || h.kind !== "ready") return <p className="mx-note" aria-hidden="true" />;
  const parts = [t("metrics.since", { time: f.dateTime(snap.since) })];
  for (const o of h.partial) parts.push(t("metrics.partial", { name: o.name, ok: o.reachable, total: o.replicas }));
  if (h.dropped > 0) parts.push(t("metrics.dropped", { count: h.dropped }));
  return (
    <p className="mx-note" title={t("metrics.since.title")}>
      {parts.join(" · ")}
    </p>
  );
}
