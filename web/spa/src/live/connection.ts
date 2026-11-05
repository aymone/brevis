// One EventSource for the whole app, carrying the union of the topics that
// mounted screens subscribed to (GET /api/events?topics=a,b). The server only
// says "this topic changed"; each subscriber decides what to refetch.
// WHY a singleton: a connection per screen would burn the server's per-user
// cap and re-open on every route change.

/**
 * What woke a subscriber: the server said the topic `change`d, or the connection
 * said `ready` (a connect that may have missed something). They are different:
 * a change always means "refetch", a `ready` only means "make sure what you hold
 * is not stale", and what was fetched a moment ago is not.
 */
export type Wake = "ready" | "change";
type Listener = (wake: Wake) => void;
type Watcher = () => void;

const MAX_TOPICS = 20; // the server's cap per connection
const SETTLE_MS = 50; // a route change unsubscribes and subscribes in the same tick: open once
const HIDDEN_MS = 30_000; // a tab hidden this long stops holding a connection
const GRACE_MS = 10_000; // a blip this short is not worth polling for
const RETRY_MS = 15_000; // the browser gives up on 4xx/5xx: this is our own reopen

const subs = new Map<string, Set<Listener>>();
const watchers = new Set<Watcher>(); // useSyncExternalStore subscribers
const supported = typeof EventSource !== "undefined";

let cur: EventSource | null = null; // the connection that has said `ready`
let next: EventSource | null = null; // its replacement, still connecting
let covered = new Set<string>(); // topics `cur` has been delivering without a gap
let wanted = ""; // the key `cur`/`next` was opened for
let up = false;
let settleT: ReturnType<typeof setTimeout> | undefined;
let graceT: ReturnType<typeof setTimeout> | undefined;
let hiddenT: ReturnType<typeof setTimeout> | undefined;
let retryT: ReturnType<typeof setTimeout> | undefined;
const missed = new Set<string>(); // topics that changed while the tab was hidden

// The server's shared topics. They cost it one cheap query a second between all
// clients, so a connection carries ALL of them whenever it exists: moving between
// the screens that use them then never reopens the stream (every reopen is a
// request the browser lists as "(canceled)" and a `ready` to catch up on). Only a
// named topic (run:<id>, workflow:<slug>) changes the set, so only opening a run
// or a workflow page does.
const SHARED = ["catalog", "ingest", "nav", "overview", "runs", "workflows"];

/** The sorted, capped union of subscribed topics (plus the shared ones, once there is any); equal keys mean the same connection can stay. */
export const topicKey = (topics: Iterable<string>) => {
  const want = [...new Set(topics)];
  if (!want.length) return "";
  return [...new Set([...SHARED, ...want])].sort().slice(0, MAX_TOPICS).join(",");
};

const setUp = (v: boolean) => {
  if (up === v) return;
  up = v;
  watchers.forEach((w) => w());
};

const call = (topic: string, wake: Wake = "change") => subs.get(topic)?.forEach((fn) => fn(wake));

function closeAll() {
  cur?.close();
  next?.close();
  cur = next = null;
  covered = new Set();
  wanted = "";
}

function open(key: string) {
  next?.close();
  wanted = key;
  const src = new EventSource(`/api/events?topics=${key.split(",").map(encodeURIComponent).join(",")}`);
  next = src;

  src.addEventListener("ready", () => {
    clearTimeout(graceT);
    graceT = undefined;
    clearTimeout(retryT);
    if (src === next) {
      // The replacement is live: only now drop the old one, so no change falls in the gap.
      cur?.close();
      cur = src;
      next = null;
    }
    if (src !== cur) return;
    setUp(true);
    missed.clear();
    // A topic that was already covered saw every change; everything else (first
    // connection, reconnect, resume from hidden) may have missed some: refetch it.
    const now = new Set(key.split(","));
    for (const t of now) if (!covered.has(t)) call(t, "ready");
    covered = now;
  });

  src.addEventListener("change", (e) => {
    if (src !== cur) return;
    try {
      const topic = (JSON.parse((e as MessageEvent<string>).data) as { topic?: string }).topic;
      if (!topic) return;
      // A hidden tab never refetches in the background: remember and catch up on show.
      if (document.hidden) missed.add(topic);
      else call(topic);
    } catch {
      /* a malformed frame is ignored */
    }
  });

  // EventSource reconnects by itself (honoring `retry:`) after a drop; a 4xx/5xx
  // closes it for good, so then we reopen ourselves. Nothing is logged.
  src.onerror = () => {
    if (src === cur) covered = new Set();
    if (src === next && src.readyState === EventSource.CLOSED) next = null;
    if (src === cur && src.readyState === EventSource.CLOSED) cur = null;
    if (src.readyState === EventSource.CLOSED) {
      setUp(false);
      clearTimeout(retryT);
      retryT = setTimeout(() => {
        wanted = "";
        sync();
      }, RETRY_MS);
    } else if (!graceT && up) {
      graceT = setTimeout(() => {
        graceT = undefined;
        setUp(false);
      }, GRACE_MS);
    }
  };
}

function sync() {
  const key = topicKey(subs.keys());
  if (!key) return closeAll();
  if (!supported) return setUp(false);
  if (document.hidden && !cur && !next) return; // reopened on show
  if (key === wanted && (cur || next)) return;
  open(key);
}

/** Calls `fn` when `topic` changed (and once on every connect that may have missed it). Returns the unsubscribe. */
export function subscribe(topic: string, fn: Listener): () => void {
  let set = subs.get(topic);
  if (!set) subs.set(topic, (set = new Set()));
  set.add(fn);
  clearTimeout(settleT);
  settleT = setTimeout(sync, SETTLE_MS);
  return () => {
    set.delete(fn);
    if (!set.size) subs.delete(topic);
    clearTimeout(settleT);
    settleT = setTimeout(sync, SETTLE_MS);
  };
}

/** useSyncExternalStore pair: is the stream delivering (or only briefly interrupted)? */
export const watchUp = (w: Watcher) => {
  watchers.add(w);
  return () => void watchers.delete(w);
};
export const isUp = () => up;

if (typeof document !== "undefined") {
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      hiddenT = setTimeout(closeAll, HIDDEN_MS);
      return;
    }
    clearTimeout(hiddenT);
    if (!cur && !next) sync(); // its `ready` refetches everything
    else [...missed].forEach((t) => call(t, "change"));
    missed.clear();
  });
}
