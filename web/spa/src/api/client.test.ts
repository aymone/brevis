import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, getJSON, postJSON } from "./client";

const reply = (status: number, body?: unknown, extra: Partial<Response> = {}) => {
  const res = { status, ok: status >= 200 && status < 300, redirected: false, url: "http://x/api", json: async () => body, ...extra } as Response;
  // the client reads a 401's body from a clone, to leave the original readable
  (res as { clone: () => Response }).clone = () => res;
  return res;
};
const stub = (res: Response | Promise<never>) => {
  const f = vi.fn(() => Promise.resolve(res));
  vi.stubGlobal("fetch", f);
  return f;
};
const assign = vi.fn();
const replace = vi.fn();
const here = (path: string) => vi.stubGlobal("location", { pathname: path, search: "?q=1", assign, replace });

afterEach(() => {
  vi.unstubAllGlobals();
  assign.mockClear();
  replace.mockClear();
});

describe("getJSON", () => {
  it("returns the parsed body", async () => {
    stub(reply(200, { a: 1 }));
    await expect(getJSON("/api/x")).resolves.toEqual({ a: 1 });
  });

  it("a 204 is `nothing there`: undefined, and the body is never read", async () => {
    const json = vi.fn();
    stub(reply(204, undefined, { json }));
    await expect(getJSON("/api/x")).resolves.toBeUndefined();
    expect(json).not.toHaveBeenCalled();
  });

  it("an error status becomes an ApiError with the status and the server's message", async () => {
    stub(reply(409, { error: "slug taken" }));
    const e = await getJSON("/api/x").catch((x: unknown) => x);
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({ status: 409, message: "slug taken" });
  });

  it("falls back to `path: HTTP n` when the failure has no JSON body", async () => {
    stub(reply(502, undefined, { json: async () => Promise.reject(new Error("html")) }));
    await expect(getJSON("/api/x")).rejects.toMatchObject({ status: 502, message: "/api/x: HTTP 502" });
  });

  it("the Gate's 401 {error: sign_in_required} sends the browser to the login and back to this screen", async () => {
    here("/runs");
    stub(reply(401, { error: "sign_in_required" }));
    await expect(getJSON("/api/x")).rejects.toMatchObject({ status: 401 });
    expect(replace).toHaveBeenCalledWith("/login?next=%2Fruns%3Fq%3D1");
  });

  it("a 401 that is NOT the Gate's is that screen's error, not a sign-out", async () => {
    here("/runs");
    stub(reply(401, { error: "the warehouse refused the token" }));
    await expect(getJSON("/api/x")).rejects.toMatchObject({ status: 401, message: "the warehouse refused the token" });
    expect(replace).not.toHaveBeenCalled();
  });

  it("on the login screen itself there is nowhere to go: the caller handles the 401", async () => {
    here("/login");
    stub(reply(401, { error: "sign_in_required" }));
    await expect(getJSON("/api/session")).rejects.toMatchObject({ status: 401 });
    expect(replace).not.toHaveBeenCalled();
  });

  it("any other redirect is not a login", async () => {
    stub(reply(200, { ok: true }, { redirected: true, url: "http://x/api/other" }));
    await expect(getJSON("/api/x")).resolves.toEqual({ ok: true });
    expect(assign).not.toHaveBeenCalled();
  });

  it("passes no AbortSignal to fetch (a cancellable read re-sends on every StrictMode mount)", async () => {
    const f = stub(reply(200, {}));
    await getJSON("/api/x");
    const init = (f.mock.calls[0] as unknown as [string, RequestInit])[1];
    expect(init).not.toHaveProperty("signal");
  });
});

describe("postJSON", () => {
  it("sends the body as JSON and resolves undefined on 204", async () => {
    const f = stub(reply(204));
    await expect(postJSON("/api/x", { a: 1 })).resolves.toBeUndefined();
    const init = (f.mock.calls[0] as unknown as [string, RequestInit])[1];
    expect(init).toMatchObject({ method: "POST", body: '{"a":1}' });
  });
  it("keeps the parsed error body on the ApiError, for per-field messages", async () => {
    stub(reply(422, { error: "bad", kind: "param", name: "must be a number" }));
    const e = await postJSON("/api/x", {}).catch((x: unknown) => x);
    expect(e).toMatchObject({ status: 422, message: "bad", body: { kind: "param", name: "must be a number" } });
  });
});

// The rule in client.ts is about EVERY query function, and a screen that reads `signal`
// brings the double request back with no test noticing. So scan the sources for it.
describe("queryFns never read React Query's signal", () => {
  const walk = (dir: string): string[] =>
    readdirSync(dir).flatMap((n) => {
      const p = join(dir, n);
      return statSync(p).isDirectory() ? walk(p) : /\.(ts|tsx)$/.test(n) && !/\.test\./.test(n) ? [p] : [];
    });
  const files = walk(join(import.meta.dirname, ".."));

  it("scans the app's sources", () => expect(files.length).toBeGreaterThan(30));
  it("no destructured or member access of `signal` in a queryFn", () => {
    const offenders = files.filter((f) => {
      const src = readFileSync(f, "utf8");
      return /queryFn:\s*(async\s*)?\(\s*\{[^)]*\bsignal\b/.test(src) || /queryFn:\s*(async\s*)?\(\s*\w+\s*\)\s*=>[^\n]*\.signal\b/.test(src);
    });
    expect(offenders).toEqual([]);
  });
});
