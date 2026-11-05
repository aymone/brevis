import { describe, expect, it } from "vitest";
import { drawerOf } from "./drawer";

describe("drawerOf", () => {
  it("reads the drawer a hash names", () => {
    expect(drawerOf("#publish")).toBe("publish");
    expect(drawerOf("#test-event")).toBe("test-event");
    expect(drawerOf("")).toBe("");
  });

  it("still opens a link written before the ids were English", () => {
    expect(drawerOf("#publicar")).toBe("publish");
    expect(drawerOf("#novo")).toBe("new");
  });
});
