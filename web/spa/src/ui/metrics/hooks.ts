import { useSession } from "../../api/session";
import { useData } from "../../api/live";
import type { Entity, EndpointsMetrics, Owner, Series, StreamsMetrics } from "./types";

// Server topics (internal/events/topics.go). They are not in the connection's shared six: the stream carries
// them only while a screen that draws them is mounted.
export const TOPIC_STREAMS = "metrics:streams";
export const TOPIC_ENDPOINTS = "metrics:endpoints";

// Every row of a list asks for the same key, so the list costs ONE request per scrape however many rows draw it.
export const useStreamsMetrics = () => useData<StreamsMetrics>(["metrics", "streams", 30], "/api/ingest/metrics/streams?minutes=30", { topic: TOPIC_STREAMS });
export const useEndpointsMetrics = () => useData<EndpointsMetrics>(["metrics", "endpoints", 30], "/api/endpoints/metrics?minutes=30", { topic: TOPIC_ENDPOINTS });

const enc = (path: string) => path.split("/").map(encodeURIComponent).join("/");
export const useStreamMetrics = (gateway: string, path: string) =>
  useData<StreamsMetrics>(["metrics", "stream", gateway, path], `/api/ingest/metrics/streams/${encodeURIComponent(gateway)}${path.split("/").map(encodeURIComponent).join("/")}?minutes=60`, {
    topic: TOPIC_STREAMS,
    keep: false,
  });
export const useEndpointMetrics = (api: string, name: string) =>
  useData<EndpointsMetrics>(["metrics", "endpoint", api, name], `/api/endpoints/${encodeURIComponent(api)}/${enc(name)}/metrics?minutes=60`, {
    topic: TOPIC_ENDPOINTS,
    keep: false,
  });

/** The sum of a series' observed minutes; null when none was observed. */
export const total = (s: Series | undefined): number | null => {
  const known = (s ?? []).filter((v): v is number => v !== null);
  return known.length ? known.reduce((a, b) => a + b, 0) : null;
};

/** What a screen may claim about the source, decided once. */
export type Health =
  | { kind: "not_configured" }
  | { kind: "waiting" }
  | { kind: "unreachable"; owners: Owner[] }
  | { kind: "ready"; partial: Owner[]; dropped: number };

export function health(configured: boolean, owners: Owner[]): Health {
  if (!configured) return { kind: "not_configured" };
  const usable = owners.filter((o) => o.state === "ok" || o.state === "partial");
  if (owners.length === 0 || owners.every((o) => o.state === "waiting")) return { kind: "waiting" };
  if (usable.length === 0) return { kind: "unreachable", owners: owners.filter((o) => o.state === "unreachable") };
  return {
    kind: "ready",
    partial: owners.filter((o) => o.state === "partial" || o.state === "unreachable"),
    dropped: owners.reduce((n, o) => n + (o.dropped ?? 0), 0),
  };
}

/** Counts and rates in the reader's language: one decimal under 10, none above. */
export function useCounts() {
  const { lang } = useSession();
  const whole = new Intl.NumberFormat(lang, { maximumFractionDigits: 0 });
  const one = new Intl.NumberFormat(lang, { maximumFractionDigits: 1 });
  const pct = new Intl.NumberFormat(lang, { style: "percent", maximumFractionDigits: 1 });
  return {
    count: (v: number) => whole.format(v),
    rate: (v: number) => (v < 10 ? one.format(v) : whole.format(v)),
    percent: (part: number, whole_: number) => pct.format(whole_ > 0 ? part / whole_ : 0),
  };
}

export const entityOf = <E extends Entity>(snap: { entities: E[] } | undefined): E | undefined => snap?.entities[0];

/** The error rate of an endpoint's counted minutes: 4xx/5xx other than 429, over everything answered. */
export function errorShare(e: Entity): { errors: number; all: number } {
  const h = e.lastHour;
  const errors = (h.clientError ?? 0) + (h.serverError ?? 0);
  return { errors, all: errors + (h.ok ?? 0) + (h.rateLimited ?? 0) };
}

export const REQUEST_KEYS = ["ok", "rateLimited", "clientError", "serverError"] as const;

/** Minute-wise sum of measures; a minute is null only when none of them was observed. */
export function sumSeries(e: Entity | undefined, keys: readonly string[]): Series {
  const parts = keys.map((k) => e?.values[k]).filter((v): v is Series => !!v);
  const n = parts[0]?.length ?? 0;
  return Array.from({ length: n }, (_, i) => (parts.every((p) => p[i] === null) ? null : parts.reduce((a, p) => a + (p[i] ?? 0), 0)));
}

export function sumRate(e: Entity | undefined, keys: readonly string[]): number | null {
  const parts = keys.map((k) => e?.ratePerMin[k] ?? null);
  return parts.every((p) => p === null) ? null : parts.reduce<number>((a, p) => a + (p ?? 0), 0);
}

export const sumHour = (e: Entity | undefined, keys: readonly string[]): number => keys.reduce((a, k) => a + (e?.lastHour[k] ?? 0), 0);
