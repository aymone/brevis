import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { parseAst } from "rolldown/parseAst";
import { describe, expect, it } from "vitest";

// A SENTENCE IN JSX IS A SENTENCE NOBODY TRANSLATES. A screen written with
// <p>Could not load this.</p> works in English and stays English in the other ten
// languages, and nothing fails: the only way to see it is to read the page in
// German. So the source is read instead: every JSX text and every user-facing
// attribute (title, aria-label, placeholder, alt, label, text) that carries a
// letter must go through t()/n(), or be on the list below with the reason it is
// not a message.
//
// Scope, and why: JSX text and JSX attributes only. The ghost fixtures at the top
// of the screens (GHOST rows with "Source label") are plain object properties
// drawn as gray bars, never read by a person, so properties are not scanned.

const ATTRS = new Set(["title", "aria-label", "placeholder", "alt", "label", "text", "aria-description"]);

// Text that is not a message in any language. Each line says why.
const ALLOWED: Record<string, string> = {
  GET: "HTTP verb",
  POST: "HTTP verb",
  SDK: "glossary term",
  gateway: "glossary term",
  Brevis: "the product's name",
  ID: "universal abbreviation (screen-reader label of a run's id)",
  ms: "unit symbol",
  Esc: "key name",
  esc: "key name",
  "Ctrl+Space": "key chord",
  CSV: "file format",
  catchup: "the schedule field's own name, as written in the workflow file",
  "UTC —": "time zone code and a separator, followed by the change's own text",
  "$BREVIS_AUTO_*": "environment variable prefix",
  auto_table: "a column name from the data",
  "00 steps · 00 stages": "ghost placeholder, drawn as a gray bar",
  Workflows: "glossary term",
  "1–25 of 25": "ghost placeholder under the runs list, drawn as a gray bar of the loaded text's width",
  connection: "ghost placeholder of the connection picker's value, a gray bar",
};
// Commands a person types are code (as is the SQL example); so are HTML entities (arrows) on their own.
const codeLike = (s: string) => /^(brevis|gateway|brevis-sql-api) /.test(s) || /^(&\w+;|\s)+$/.test(s) || /^SELECT /.test(s) || !/[A-Za-z]{2}/.test(s);

function files(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) files(p, out);
    else if (p.endsWith(".tsx") && !p.includes(".test.")) out.push(p);
  }
  return out;
}

type Node = { type: string; [k: string]: unknown };

function literalsIn(n: unknown, out: string[]): void {
  if (!n || typeof n !== "object") return;
  if (Array.isArray(n)) return n.forEach((x) => literalsIn(x, out));
  const node = n as Node;
  if (node.type === "Literal" && typeof node.value === "string") out.push(node.value);
  if (node.type === "TemplateLiteral") for (const q of node.quasis as { value: { raw: string } }[]) out.push(q.value.raw);
  // A comparison's operand ("day" in grain === "day") and an element nested in the value are not text.
  if (node.type === "JSXElement" || (node.type === "BinaryExpression" && /^[!=]==?$/.test(String(node.operator)))) return;
  if (node.type === "ConditionalExpression") {
    literalsIn(node.consequent, out);
    return literalsIn(node.alternate, out);
  }
  // A t("key") argument is a key, not a sentence.
  if (node.type === "CallExpression" && /^[tn]$/.test(String((node.callee as Node).name ?? ""))) return;
  for (const k of Object.keys(node)) if (k !== "type" && k !== "start" && k !== "end") literalsIn(node[k], out);
}

function scan(src: string): string[] {
  const found: string[] = [];
  const walk = (n: unknown): void => {
    if (!n || typeof n !== "object") return;
    if (Array.isArray(n)) return n.forEach(walk);
    const node = n as Node;
    if (node.type === "JSXText") {
      const text = String(node.value).replace(/\s+/g, " ").trim();
      if (text) found.push(text);
    } else if (node.type === "JSXAttribute" && ATTRS.has(String((node.name as Node).name))) {
      literalsIn(node.value, found);
    } else if (node.type === "JSXExpressionContainer" && (node.expression as Node).type === "Literal") {
      literalsIn(node.expression, found);
    }
    for (const k of Object.keys(node)) if (k !== "type" && k !== "start" && k !== "end") walk(node[k]);
  };
  walk(parseAst(src, { lang: "tsx" }).body);
  return found.map((s) => s.trim()).filter((s) => s !== "" && !codeLike(s) && !(s in ALLOWED));
}

describe("the screens' words", () => {
  it("never sit in JSX as English literals: they come from the catalog", () => {
    const bad: string[] = [];
    for (const f of files(join(__dirname))) for (const s of scan(readFileSync(f, "utf8"))) bad.push(`${f.slice(__dirname.length + 1)}: ${JSON.stringify(s)}`);
    expect(bad, "move each sentence into web/i18n/locales (all 11 files) and read it with t()/n(), or add it to ALLOWED with the reason").toEqual([]);
  });

  it("does see a literal (the check itself works)", () => {
    // The scanner is the only thing standing between a screen and an untranslated
    // sentence; one that silently finds nothing would pass forever.
    const sample = `const A = () => <p title="Hello there" aria-label={t("a.b")} className="flex px-2">Could not load <b>{t("x.y")}</b> <i>GET</i> &rarr;</p>;`;
    expect(scan(sample)).toEqual(["Hello there", "Could not load"]);
  });
});
