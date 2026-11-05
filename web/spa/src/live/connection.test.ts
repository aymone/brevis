import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The connection is a module-level singleton (one stream for the whole app), so every
// test imports a fresh copy against a fake EventSource and fake timers.
class FakeES {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  static all: FakeES[] = [];
  readyState = 0;
  closed = false;
  onerror: (() => void) | null = null;
  private fns = new Map<string, ((e: unknown) => void)[]>();
  constructor(readonly url: string) {
    FakeES.all.push(this);
  }
  addEventListener(type: string, fn: (e: unknown) => void) {
    this.fns.set(type, [...(this.fns.get(type) ?? []), fn]);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  emit(type: string, data?: string) {
    this.fns.get(type)?.forEach((f) => f({ data }));
  }
  ready() {
    this.readyState = 1;
    this.emit("ready");
  }
  change(topic: string) {
    this.emit("change", JSON.stringify({ topic }));
  }
  drop(closed: boolean) {
    this.readyState = closed ? 2 : 0;
    this.onerror?.();
  }
  get topics() {
    return new URL(this.url, "http://x").searchParams.get("topics")!.split(",");
  }
}

let hidden = false;
const docListeners: [string, EventListener][] = [];
const realAdd = document.addEventListener.bind(document);

async function load() {
  vi.resetModules();
  return import("./connection");
}
const settle = () => vi.advanceTimersByTime(60);
const setHidden = (v: boolean) => {
  hidden = v;
  document.dispatchEvent(new Event("visibilitychange"));
};

beforeEach(() => {
  vi.useFakeTimers();
  FakeES.all = [];
  hidden = false;
  vi.stubGlobal("EventSource", FakeES);
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  // Each fresh import registers its own visibilitychange listener on the shared document:
  // forget the previous copies' or they would open streams of their own.
  vi.spyOn(document, "addEventListener").mockImplementation((t: string, fn: EventListenerOrEventListenerObject, o?: boolean | AddEventListenerOptions) => {
    docListeners.push([t, fn as EventListener]);
    realAdd(t, fn, o);
  });
});
afterEach(() => {
  docListeners.splice(0).forEach(([t, fn]) => document.removeEventListener(t, fn));
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("topicKey", () => {
  it("is empty with nothing subscribed, so no stream is opened", async () => {
    const { topicKey } = await load();
    expect(topicKey([])).toBe("");
  });
  it("always carries the shared topics, sorted, plus the named ones", async () => {
    const { topicKey } = await load();
    expect(topicKey(["run:9", "runs"])).toBe("catalog,ingest,nav,overview,run:9,runs,workflows");
  });
  it("is the same key for any shared topic alone, so moving between screens never reopens", async () => {
    const { topicKey } = await load();
    expect(topicKey(["runs"])).toBe(topicKey(["overview"]));
  });
  it("caps at the server's 20 topics", async () => {
    const { topicKey } = await load();
    const many = Array.from({ length: 40 }, (_, i) => `t${String(i).padStart(2, "0")}`);
    expect(topicKey(many).split(",")).toHaveLength(20);
  });
});

describe("opening and the topic set", () => {
  it("opens nothing until somebody subscribes, then once after the settle delay", async () => {
    const { subscribe } = await load();
    settle();
    expect(FakeES.all).toHaveLength(0);
    subscribe("runs", () => {});
    expect(FakeES.all).toHaveLength(0);
    settle();
    expect(FakeES.all).toHaveLength(1);
    expect(FakeES.all[0]!.topics).toEqual(["catalog", "ingest", "nav", "overview", "runs", "workflows"]);
  });

  it("a route change (unsubscribe + subscribe in one tick) opens once, not twice", async () => {
    const { subscribe } = await load();
    const off = subscribe("runs", () => {});
    settle();
    off();
    subscribe("overview", () => {});
    settle();
    expect(FakeES.all).toHaveLength(1); // same key: the open stream stays
  });

  it("a named topic changes the set: the new stream replaces the old one only once it is ready", async () => {
    const { subscribe } = await load();
    subscribe("runs", () => {});
    settle();
    const first = FakeES.all[0]!;
    first.ready();
    subscribe("run:42", () => {});
    settle();
    const second = FakeES.all[1]!;
    expect(second.topics).toContain("run:42");
    expect(first.closed).toBe(false); // no gap where a change could fall
    second.ready();
    expect(first.closed).toBe(true);
  });

  it("closes the stream when the last subscriber leaves", async () => {
    const { subscribe } = await load();
    const off = subscribe("runs", () => {});
    settle();
    FakeES.all[0]!.ready();
    off();
    settle();
    expect(FakeES.all[0]!.closed).toBe(true);
  });
});

describe("wake kinds", () => {
  it("`ready` wakes a topic once, and a topic already covered is not woken again by a replacement", async () => {
    const { subscribe } = await load();
    const runs = vi.fn();
    const run = vi.fn();
    subscribe("runs", runs);
    settle();
    FakeES.all[0]!.ready();
    expect(runs).toHaveBeenCalledWith("ready");
    subscribe("run:1", run);
    settle();
    FakeES.all[1]!.ready();
    expect(run).toHaveBeenCalledWith("ready");
    expect(runs).toHaveBeenCalledTimes(1); // it saw every change on the first stream
  });

  it("a `change` frame wakes the topic as `change`; a malformed or topic-less frame is ignored", async () => {
    const { subscribe } = await load();
    const fn = vi.fn();
    subscribe("runs", fn);
    settle();
    const es = FakeES.all[0]!;
    es.ready();
    fn.mockClear();
    es.change("runs");
    es.emit("change", "{not json");
    es.emit("change", "{}");
    es.change("nav"); // nobody listens to it
    expect(fn.mock.calls).toEqual([["change"]]);
  });
});

describe("hidden tab", () => {
  it("does not wake in the background and catches up as `change` on show", async () => {
    const { subscribe } = await load();
    const fn = vi.fn();
    subscribe("runs", fn);
    settle();
    const es = FakeES.all[0]!;
    es.ready();
    fn.mockClear();
    setHidden(true);
    es.change("runs");
    expect(fn).not.toHaveBeenCalled();
    setHidden(false);
    expect(fn.mock.calls).toEqual([["change"]]);
  });

  it("closes after 30 s hidden and reconnects on show, refetching through `ready`", async () => {
    const { subscribe } = await load();
    const fn = vi.fn();
    subscribe("runs", fn);
    settle();
    FakeES.all[0]!.ready();
    fn.mockClear();
    setHidden(true);
    vi.advanceTimersByTime(29_000);
    expect(FakeES.all[0]!.closed).toBe(false);
    vi.advanceTimersByTime(1_500);
    expect(FakeES.all[0]!.closed).toBe(true);
    setHidden(false);
    expect(FakeES.all).toHaveLength(2);
    FakeES.all[1]!.ready();
    expect(fn.mock.calls).toEqual([["ready"]]);
  });

  it("coming back before 30 s keeps the stream", async () => {
    const { subscribe } = await load();
    subscribe("runs", () => {});
    settle();
    FakeES.all[0]!.ready();
    setHidden(true);
    vi.advanceTimersByTime(20_000);
    setHidden(false);
    vi.advanceTimersByTime(60_000);
    expect(FakeES.all).toHaveLength(1);
    expect(FakeES.all[0]!.closed).toBe(false);
  });
});

describe("up / down (the fallback polling switch)", () => {
  it("is up after `ready`, and a blip shorter than 10 s does not flip it", async () => {
    const { subscribe, isUp } = await load();
    subscribe("runs", () => {});
    settle();
    const es = FakeES.all[0]!;
    expect(isUp()).toBe(false);
    es.ready();
    expect(isUp()).toBe(true);
    es.drop(false);
    vi.advanceTimersByTime(9_000);
    expect(isUp()).toBe(true);
    es.ready();
    vi.advanceTimersByTime(5_000);
    expect(isUp()).toBe(true);
  });

  it("goes down after 10 s of a dropped stream and tells the watchers", async () => {
    const { subscribe, isUp, watchUp } = await load();
    const seen = vi.fn();
    watchUp(seen);
    subscribe("runs", () => {});
    settle();
    const es = FakeES.all[0]!;
    es.ready();
    seen.mockClear();
    es.drop(false);
    vi.advanceTimersByTime(10_100);
    expect(isUp()).toBe(false);
    expect(seen).toHaveBeenCalledTimes(1);
  });

  it("a closed stream (4xx/5xx) is down at once and reopened by us after 15 s", async () => {
    const { subscribe, isUp } = await load();
    subscribe("runs", () => {});
    settle();
    const es = FakeES.all[0]!;
    es.ready();
    es.drop(true);
    expect(isUp()).toBe(false);
    vi.advanceTimersByTime(15_100);
    expect(FakeES.all).toHaveLength(2);
  });
});
