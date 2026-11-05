import { describe, expect, it } from "vitest";
import { checkEvent, type TestEventRules } from "./test-event";

// The words are the caller's: echo key and vars so every message is identifiable.
const say = (key: string, vars?: Record<string, string | number>) => (vars ? `${key}(${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(",")})` : key);
const rules = (o: Partial<TestEventRules> = {}): TestEventRules => ({ format: "json", table: "", allow: [], dataset: "", lands: "events", ...o });
const run = (raw: string, o?: Partial<TestEventRules>) => checkEvent(raw, rules(o), say);

describe("json", () => {
  it("accepts one object and says where it lands", () => {
    expect(run('{"a":1}\n')).toEqual({ kind: "success", text: "testevent.valid(format=json) testevent.lands(to=events)" });
  });
  it("refuses a list or a scalar, and invalid JSON with the parser's reason", () => {
    expect(run("[1]")).toMatchObject({ kind: "failed", text: "testevent.object" });
    expect(run("3")).toMatchObject({ kind: "failed", text: "testevent.object" });
    expect(run("{nope")).toMatchObject({ kind: "failed", text: expect.stringContaining("testevent.invalid(e=") as string });
  });
});

describe("array", () => {
  it("accepts a list of objects", () => {
    expect(run('[{"a":1},{"b":2}]', { format: "array" }).kind).toBe("success");
  });
  it("refuses an object, and a list with a non-object item", () => {
    expect(run("{}", { format: "array" }).text).toBe("testevent.array");
    expect(run('[{"a":1}, 2]', { format: "array" }).text).toBe("testevent.items");
  });
});

describe("ndjson", () => {
  it("accepts one object per line and skips blank lines", () => {
    expect(run('{"a":1}\n\n{"b":2}', { format: "ndjson" }).kind).toBe("success");
  });
  it("names the line that is not an object, or not JSON", () => {
    expect(run('{"a":1}\n5', { format: "ndjson" }).text).toBe("testevent.line(n=2,e=testevent.object)");
    expect(run('{"a":1}\n{bad', { format: "ndjson" }).text).toMatch(/^testevent\.line\(n=2,e=/);
  });
});

describe("auto_table", () => {
  const t = { table: "kind", dataset: "raw", allow: ["page_", "click_"] };
  it("lands each event in dataset.table, once per table", () => {
    const r = run('[{"kind":"page_view"},{"kind":"click_cta"},{"kind":"page_view"}]', { ...t, format: "array" });
    expect(r.text).toContain("testevent.lands(to=raw.page_view, raw.click_cta)");
    expect(r.kind).toBe("success");
  });
  it("lands the bare name without a dataset, and any name when nothing is allowed", () => {
    expect(run('{"kind":"x"}', { table: "kind" }).text).toContain("to=x)");
  });
  it("fails when the event has no table field, or an empty one", () => {
    expect(run("{}", t).text).toBe("testevent.no_table(field=kind)");
    expect(run('{"kind":""}', t).kind).toBe("failed");
  });
  it("warns (not fails) about a table the accepted prefixes refuse", () => {
    expect(run('{"kind":"other"}', t)).toEqual({ kind: "queued", text: "testevent.refused(name=other,allow=page_, click_)" });
  });
});
