// The live volume endpoints' shapes (internal/api/metrics_api.go, internal/metrics/view.go).
// A null in a series is a minute nobody observed; a 0 is a minute that was observed and had nothing.

export type Series = (number | null)[];

export type OwnerState = "waiting" | "ok" | "partial" | "unreachable";
export type Owner = {
  name: string;
  state: OwnerState;
  replicas: number;
  reachable: number;
  lastSuccess: string | null;
  error?: "timeout" | "unreachable" | "status" | "unreadable" | "too_large";
  dropped?: number;
};

export type Latency = { p50Sec: number | null; p95Sec: number | null; samples: number; windowMinutes: number };

export type Entity = {
  owner: string;
  name: string;
  present: boolean;
  values: Record<string, Series>;
  covered: number[];
  ratePerMin: Record<string, number | null>;
  lastHour: Record<string, number>;
  observedSec: number;
  gauges?: Record<string, number>;
  latencyP50Sec?: Series;
  latencyP95Sec?: Series;
  latency?: Latency;
};

export type Snapshot<E> = {
  configured: boolean;
  since: string;
  intervalSec: number;
  window: { start: number; minutes: number };
  owners: Owner[];
  total: Entity;
  entities: E[];
};

export type StreamMetric = Entity & { gateway: string; path: string };
export type EndpointMetric = Entity & { api: string };
export type StreamsMetrics = Snapshot<StreamMetric>;
export type EndpointsMetrics = Snapshot<EndpointMetric>;
