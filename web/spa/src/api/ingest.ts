// The Ingest endpoints' shapes (internal/api/*_api.go, web/pages/ingest_json.go).
// Lists are always arrays; a missing value is null; times are RFC 3339.

export type Place = { uri: string; kind: string; kindTitle: string; where: string; what: string; note: string };
export type Source = { kind: string; driver: string; label: string; where: string };
export type Choice = { key: string; label: string };

export type LoadRow = {
  workflow: string;
  step: string;
  href: string;
  target: Place;
  legacy: boolean;
  status: string;
  lastLoaded: string | null;
  rows: number | null;
  schedule: string;
  source: Source;
  recent: (number | null)[];
  fixing: string | null;
};

export type Loads = {
  rows: LoadRow[];
  total: number;
  workflows: number;
  summary: Record<string, number>;
  counts: Record<string, number>;
  sources: Choice[];
  hasLegacy: boolean;
  filter: { status: string; source: string; q: string; legacy: boolean };
};

export type CfgRow = {
  path: string;
  label: string;
  value: string;
  default: boolean;
  note: string;
  list: string[];
  before: { label: string; text: string }[];
};

export type FlowNode = { kind: string; main: string; detail: string };
export type Flow = { nodes: FlowNode[]; deadLetter: FlowNode | null };

export type Change = {
  at: string;
  runId: string;
  first: boolean;
  earlier: boolean;
  lines: { path: string; text: string }[];
};

export type StepConfig = {
  summary: string;
  note: string;
  flow: Flow | null;
  stage: {
    sourceDriver: string;
    sourceCount: number;
    workers: number | null;
    transformCount: number;
    transformNames: string;
    targetDriver: string;
    dedup: string;
  };
  other: { title: string; rows: CfgRow[] }[];
  sourceHead: CfgRow[];
  sourceCount: number;
  common: CfgRow[];
  varying: string[];
  sources: { index: number; cells: (CfgRow | null)[] }[];
  transform: { index: string; rows: CfgRow[] }[];
  identity: { kind: string; legacy: boolean; where: string; object: string };
  destination: CfgRow[];
  schema: { name: string; type: string; role: string }[];
  schemaExtra: CfgRow[];
  hasSchema: boolean;
};

export type Reader = { api: string; name: string; path: string; href: string };

export type LoadDetail = {
  status: string;
  reason: string;
  source: Source;
  target: Place;
  hasSchedule: boolean;
  when: string;
  cron: string;
  timezone: string;
  manual: boolean;
  legacy: boolean;
  lastLoaded: string | null;
  lastRows: number | null;
  read: number | null;
  readNothing: boolean;
  fixing: string | null;
  bars: { runId: string; rows: number | null; title: string }[];
  missed: number;
  loads: { loadedAt: string; rows: number | null; bytes: number | null; runId: string }[];
  loadsCount: string;
  described: boolean;
  config: StepConfig | null;
  changes: Change[];
  readBy: Reader[];
  query: string;
  run: "" | "now" | "params";
};

export type Load = {
  workflow: string;
  step: string;
  choices: { uri: string; current: boolean }[];
  choosing: boolean;
  detail: LoadDetail | null;
};

/**
 * What the console can say about a stream's health. `state` is "unknown" until the gateway reports its traffic
 * (the engine holds configuration only); `reason` is a stable key the app translates, never a sentence.
 */
export type StreamHealth = {
  state: string;
  reason: string;
  /** When the state began, as far as the console watched; for "quiet", the last minute with events. */
  since?: string;
  windowMinutes?: number;
  observedMinutes?: number;
  recovering?: boolean;
  /** What the judged window counted: events, except delivered/retried/buried (batches) and saturated (requests). */
  window?: Record<"received" | "rejected" | "dropped" | "deadLetter" | "delivered" | "retried" | "buried" | "saturated", number>;
  rejectedRatio?: number;
  deadLetterRatio?: number;
  failedBatchRatio?: number;
  retryRatio?: number;
  latencyP95Sec?: number;
  replicas?: number;
  reachable?: number;
};

export type StreamRow = {
  gateway: string;
  path: string;
  href: string;
  note: string;
  sink: Place;
  batch: string;
  // The declared values behind `batch` ("2m", 2000): optional so an older server still draws the sentence.
  batchEvery?: string;
  batchRecords?: number | null;
  deadLetter: Place | null;
  // Optional so an older server still draws the row (as "no data").
  health?: StreamHealth;
};

export type Streams = {
  rows: StreamRow[];
  total: number;
  gateways: number;
  withDeadLetter: number;
  healthCounts?: Record<string, number>;
  kinds: Choice[];
  filter: { kind: string; q: string };
};

export type Stream = {
  gateway: string;
  path: string;
  health?: StreamHealth;
  sink: Place;
  deadLetter: Place | null;
  publishedAt: string | null;
  format: string;
  lede: string;
  summary: string;
  flow: Flow | null;
  tables: string[];
  catalogHref: string;
  query: string;
  readBy: Reader[];
  hasConfig: boolean;
  cards: { key: string; title: string; rows: { path: string; label: string; value: string; default: boolean; note: string }[] }[];
  yaml: string;
  changes: { at: string; text: string }[];
  deadLetterPanel: { where: string; commands: string; readable: boolean; write: string } | null;
  buriedMetric: string;
  testEvent: { format: string; body: string; curlHead: string; lands: string; table: string; allow: string[]; dataset: string };
};
