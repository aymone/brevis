import { describe, expect, it } from "vitest";
import { errorKey } from "./errors";

describe("errorKey", () => {
  it("says each of the server's codes from the catalog", () => {
    expect(errorKey("unavailable")).toBe("loaderror.text");
    expect(errorKey("not_found")).toBe("error.not_found");
    expect(errorKey("run_just_started")).toBe("error.run_just_started");
    expect(errorKey("endpoint_unreachable")).toBe("endpoint.try.err.unreachable");
    expect(errorKey("refused")).toBe("sql.err.refused");
  });

  it("reads a code it does not know, or none, as a plain failure", () => {
    for (const other of ["", "something_new", "Invalid username or password."]) expect(errorKey(other)).toBe("error.generic");
  });
});
