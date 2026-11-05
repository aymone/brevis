type Say = (key: string, vars?: Record<string, string | number>) => string;
export type TestEventRules = { format: string; table: string; allow: string[]; dataset: string; lands: string };

const isObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);

/**
 * What the test-event drawer says about a body (ui.js testEvent): one object
 * for json, a list for array, one per line for ndjson; for an auto_table the
 * table each event names against the accepted prefixes; then where it lands.
 * `kind` picks the color. The words come from the caller's t().
 */
export function checkEvent(raw: string, r: TestEventRules, say: Say): { text: string; kind: "failed" | "queued" | "success" } {
  const body = raw.replace(/\s+$/, "");
  let problem = "";
  let warning = "";
  let events: Record<string, unknown>[] = [];
  try {
    switch (r.format) {
      case "array": {
        const list: unknown = JSON.parse(body);
        if (!Array.isArray(list)) problem = say("testevent.array");
        else if (!list.every(isObject)) problem = say("testevent.items");
        else events = list;
        break;
      }
      case "ndjson":
        body.split("\n").forEach((line, i) => {
          if (problem || line.trim() === "") return;
          try {
            const one: unknown = JSON.parse(line);
            if (!isObject(one)) problem = say("testevent.line", { n: i + 1, e: say("testevent.object") });
            else events.push(one);
          } catch (e) {
            problem = say("testevent.line", { n: i + 1, e: (e as Error).message });
          }
        });
        break;
      default: {
        const event: unknown = JSON.parse(body);
        if (!isObject(event)) problem = say("testevent.object");
        else events = [event];
      }
    }
  } catch (e) {
    problem = say("testevent.invalid", { e: (e as Error).message });
  }

  const lands: string[] = [];
  if (!problem && r.table) {
    const allow = r.allow.filter(Boolean);
    for (const ev of events) {
      if (problem || warning) break;
      const name = ev[r.table];
      if (typeof name !== "string" || name === "") problem = say("testevent.no_table", { field: r.table });
      else if (allow.length && !allow.some((p) => name.startsWith(p))) warning = say("testevent.refused", { name, allow: allow.join(", ") });
      else {
        const to = r.dataset ? `${r.dataset}.${name}` : name;
        if (!lands.includes(to)) lands.push(to);
      }
    }
  } else if (!problem) lands.push(r.lands);

  if (problem) return { text: problem, kind: "failed" };
  if (warning) return { text: warning, kind: "queued" };
  return { text: `${say("testevent.valid", { format: r.format })} ${say("testevent.lands", { to: lands.join(", ") })}`, kind: "success" };
}
