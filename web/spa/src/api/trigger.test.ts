import { describe, expect, it } from "vitest";
import { ApiError } from "./client";
import { fieldErrors } from "./trigger";

describe("fieldErrors", () => {
  const fields = { fields: { n: "must be a number" } } as unknown as Record<string, string>;
  it("reads the 422's per-field sentences", () => {
    expect(fieldErrors(new ApiError(422, "invalid", fields))).toEqual({ n: "must be a number" });
  });
  it("is empty for any other status, a 422 without fields, or a non-ApiError", () => {
    expect(fieldErrors(new ApiError(500, "boom", fields))).toEqual({});
    expect(fieldErrors(new ApiError(422, "invalid"))).toEqual({});
    expect(fieldErrors(new Error("x"))).toEqual({});
  });
});
