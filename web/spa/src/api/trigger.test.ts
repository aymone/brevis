import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import { fieldErrors, fieldMessage, fieldText } from "./trigger";

describe("fieldErrors", () => {
  const body = { error: "invalid_params", fields: { n: { code: "not_integer", value: "x" } } } as unknown as Record<string, string>;
  it("reads the 422's per-field codes", () => {
    expect(fieldErrors(new ApiError(422, "invalid_params", body))).toEqual({ n: { code: "not_integer", value: "x" } });
  });
  it("is empty for any other status, a 422 without fields, or a non-ApiError", () => {
    expect(fieldErrors(new ApiError(500, "boom", body))).toEqual({});
    expect(fieldErrors(new ApiError(422, "invalid"))).toEqual({});
    expect(fieldErrors(new Error("x"))).toEqual({});
  });
});

describe("fieldMessage", () => {
  it("maps a code to its catalog key and variables", () => {
    expect(fieldMessage({ code: "required" }).key).toBe("drawer.check.required");
    expect(fieldMessage({ code: "not_one_of", value: "mars", allowed: ["br", "us"] })).toMatchObject({ key: "drawer.err.not_one_of", vars: { value: "mars", allowed: "br, us" } });
    expect(fieldMessage({ code: "no_match", value: "A", pattern: "^a$" }).vars).toEqual({ value: "A", pattern: "^a$" });
  });
  it("carries a list item's position, except where the code says it already", () => {
    expect(fieldMessage({ code: "not_integer", value: "x", item: 2 }).item).toBe(2);
    expect(fieldMessage({ code: "item_empty", item: 2 })).toMatchObject({ key: "drawer.err.item_empty", vars: { n: 2 } });
    expect(fieldMessage({ code: "item_empty", item: 2 }).item).toBeUndefined();
  });
  it("names a slot in UTC, and the schedule's zone beside the nearest", () => {
    const m = fieldMessage({ code: "slot_not_on_schedule", slot: "2026-10-26T10:00:00Z", cron: "0 6 * * *", nearest: "2026-10-26T09:00:00Z", zone: "America/Sao_Paulo" });
    expect(m.key).toBe("drawer.err.slot_nearest");
    expect(m.vars).toEqual({ slot: "2026-10-26 10:00 UTC", cron: "0 6 * * *", nearest: "2026-10-26 09:00 UTC (2026-10-26 06:00 America/Sao_Paulo)" });
    expect(fieldMessage({ code: "slot_not_on_schedule", slot: "2026-10-26T10:00:00Z", cron: "0 9 * * *", zone: "UTC" }).key).toBe("drawer.err.slot_not_on_schedule");
    expect(fieldMessage({ code: "slot_in_future", slot: "2026-10-26T10:00:00Z" }).vars).toEqual({ slot: "2026-10-26 10:00 UTC" });
  });
  it("reads a code it does not know as a plain refusal", () => {
    expect(fieldMessage({ code: "from_the_future" }).key).toBe("drawer.err.invalid");
    expect(fieldMessage({ code: "" }).key).toBe("drawer.err.invalid");
  });
});

describe("fieldText", () => {
  const say = (key: string, vars?: Record<string, string | number>) => `${key}${vars ? JSON.stringify(vars) : ""}`;
  it("puts the item in front of the sentence", () => {
    expect(fieldText(say, { code: "not_integer", value: "x", item: 2 })).toBe(`drawer.err.item{"n":2,"message":"drawer.err.not_integer{\\"value\\":\\"x\\"}"}`);
    expect(fieldText(say, { code: "not_integer", value: "x" })).toBe(`drawer.err.not_integer{"value":"x"}`);
  });
});
