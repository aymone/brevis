import { act, fireEvent, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { Link, MemoryRouter, useLocation } from "react-router";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { seededClient } from "../../test/harness";
import { useFilter } from "../use-filter";
import { DropdownFilter, optionsOf, pickFrom, selectedOf, type FilterDef } from "./DropdownFilter";
import { Segmented } from "./Segmented";
import { useUrlSearch } from "./use-url-search";

const wrap = (ui: React.ReactElement, at = "/") => (
  <QueryClientProvider client={seededClient()}>
    <MemoryRouter initialEntries={[at]}>{ui}</MemoryRouter>
  </QueryClientProvider>
);

describe("useFilter (the URL is the state)", () => {
  function Probe({ change }: { change: Record<string, string> }) {
    const f = useFilter(["state", "q", "lang"], "/workflows");
    return <output data-any={f.any}>{f.hrefWith(change)}</output>;
  }
  const href = (at: string, change: Record<string, string> = {}) => {
    const { container } = render(wrap(<Probe change={change} />, at));
    return container.querySelector("output")!;
  };
  it("writes parameters in the declared order, so one filter has exactly one URL", () => {
    expect(href("/workflows?lang=go&state=failed", { q: "x" }).textContent).toBe("/workflows?state=failed&q=x&lang=go");
  });
  it("an empty value drops the parameter, and no filter is the bare path", () => {
    expect(href("/workflows?state=failed", { state: "" }).textContent).toBe("/workflows");
  });
  it("`any` says whether a filter is set", () => {
    expect(href("/w").dataset.any).toBe("false");
    expect(href("/w?q=a").dataset.any).toBe("true");
  });
});

describe("useUrlSearch", () => {
  function Search() {
    const [text, setText] = useUrlSearch("q");
    const loc = useLocation();
    return (
      <>
        <input aria-label="s" value={text} onChange={(e) => setText(e.target.value)} />
        <output>{loc.search}</output>
      </>
    );
  }
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("typing stays local; the address moves one beat (250 ms) after the last key", () => {
    render(wrap(<Search />, "/x"));
    const input = screen.getByLabelText("s") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "ab" } });
    act(() => void vi.advanceTimersByTime(200));
    fireEvent.change(input, { target: { value: "abc" } });
    act(() => void vi.advanceTimersByTime(200));
    expect(document.querySelector("output")!.textContent).toBe("");
    act(() => void vi.advanceTimersByTime(60));
    expect(document.querySelector("output")!.textContent).toBe("?q=abc");
  });

  it("starts from the address, and a change of the address (a link, Back, a chip) replaces what is typed", () => {
    render(wrap(<><Search /><Link to="/x?q=other">go</Link></>, "/x?q=hello"));
    const input = screen.getByLabelText("s") as HTMLInputElement;
    expect(input.value).toBe("hello");
    fireEvent.click(screen.getByText("go"));
    expect(input.value).toBe("other");
  });
});

describe("filter helpers", () => {
  const single: FilterDef = { id: "l", label: "L", allLabel: "All", options: [{ key: "go", label: "Go" }], value: [], onChange: () => {} };
  const multi: FilterDef = { id: "t", label: "T", multiple: true, options: [{ key: "a", label: "A" }, { key: "b", label: "B" }], value: ["a"], onChange: () => {} };
  it("a single choice gets an `all` row first, which picks nothing", () => {
    expect(optionsOf(single).map((o) => o.key)).toEqual(["", "go"]);
    expect(pickFrom(single, "")).toEqual([]);
    expect(pickFrom(single, "go")).toEqual(["go"]);
    expect(selectedOf(single)).toEqual([""]);
  });
  it("a multiple choice toggles", () => {
    expect(pickFrom(multi, "b")).toEqual(["a", "b"]);
    expect(pickFrom(multi, "a")).toEqual([]);
  });
});

describe("Segmented keyboard", () => {
  const items = [{ key: "all", label: "All" }, { key: "ok", label: "Ok" }, { key: "bad", label: "Bad" }];
  function Host({ onChange }: { onChange: (k: string) => void }) {
    const [v, setV] = useState("all");
    return <Segmented label="State" items={items} value={v} onChange={(k) => (setV(k), onChange(k))} />;
  }
  const radios = () => screen.getAllByRole("radio");
  const setup = () => {
    const onChange = vi.fn();
    render(wrap(<Host onChange={onChange} />));
    return onChange;
  };

  it("is one tab stop: only the chosen segment is tabbable", () => {
    setup();
    expect(radios().map((r) => r.tabIndex)).toEqual([0, -1, -1]);
  });
  it("arrows move focus AND choose, and wrap around", () => {
    const onChange = setup();
    fireEvent.keyDown(radios()[0]!, { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("ok");
    expect(document.activeElement).toBe(radios()[1]);
    fireEvent.keyDown(radios()[1]!, { key: "ArrowLeft" });
    fireEvent.keyDown(radios()[0]!, { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith("bad");
    expect(radios()[2]!.getAttribute("aria-checked")).toBe("true");
    expect(radios()[2]!.tabIndex).toBe(0);
  });
  it("Home and End jump; other keys do nothing", () => {
    const onChange = setup();
    fireEvent.keyDown(radios()[0]!, { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith("bad");
    fireEvent.keyDown(radios()[2]!, { key: "Home" });
    expect(onChange).toHaveBeenLastCalledWith("all");
    onChange.mockClear();
    fireEvent.keyDown(radios()[0]!, { key: "a" });
    expect(onChange).not.toHaveBeenCalled();
  });
  it("a count ghosts as 0 while pending and shows the number after", () => {
    const { rerender } = render(wrap(<Segmented label="S" items={[{ key: "a", label: "A", count: 1200 }]} value="a" onChange={() => {}} pending />));
    expect(document.querySelector(".flt-n")!.classList.contains("sk-ghost")).toBe(true);
    rerender(wrap(<Segmented label="S" items={[{ key: "a", label: "A", count: 1200 }]} value="a" onChange={() => {}} />));
    expect(document.querySelector(".flt-n")!.textContent).toBe("1,200");
  });
});

describe("DropdownFilter keyboard", () => {
  it("ArrowDown on the trigger opens the listbox; Enter on an option picks it", () => {
    const onChange = vi.fn();
    const def: FilterDef = { id: "l", label: "Language", allLabel: "All", options: [{ key: "go", label: "Go" }, { key: "py", label: "Python" }], value: [], onChange };
    render(wrap(<DropdownFilter def={def} />));
    const trigger = screen.getByRole("button", { name: /Language|\[/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    const opts = screen.getAllByRole("option");
    expect(opts.map((o) => o.textContent)).toEqual(expect.arrayContaining([expect.stringContaining("Go"), expect.stringContaining("Python")]));
    fireEvent.keyDown(opts.find((o) => o.textContent?.includes("Python"))!, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith(["py"]);
  });
});
