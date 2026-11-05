import { useFill } from "../shell/fill";
import { type PointerEvent as RPointerEvent, type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ApiError, getJSON, postJSON } from "../api/client";
import { useConfig, useSession } from "../api/session";
import { useFormat, useT } from "../i18n";
import { CodeEditor } from "../ui/CodeEditor";
import { DataGrid } from "../ui/DataGrid";
import { buttonClass } from "../ui/buttons";
import { DropdownFilter } from "../ui/filters";
import { Icon } from "../ui/icon";
import { Page } from "../ui/kit";
import { Kbd } from "../ui/kbd";
import { Tree, type TreeItem } from "../ui/Tree";
import { Skeleton } from "../ui/skeleton";
import "../ui/bar.css";
import "./sql.css";
import { useErrorText } from "../api/errors";

type Connection = { name: string; target: string; queryable: boolean };
type TreeData = { schemas: { name: string; tables: string[] }[]; cut: boolean; writable: boolean };
type State = {
  connections: Connection[];
  unreadable: string[];
  target: string;
  statement: string;
  preloaded: boolean;
  tree: TreeData;
};
type Result = {
  columns: string[];
  rows: unknown[][];
  truncated: boolean;
  bytes: number;
  ms: number;
  wrote: boolean;
  changed: number | null;
};
type Price = { state: "priced" | "unpriced" | "refused"; bytes: number; why: string };
type Entry = { text: string; outcome: "ok" | "failed"; cost: string };

// The same keys the server-rendered workbench writes, so History carries over.
const RECENT = "brevis.workbench.recent";
const DRAFT = "brevis.spa.sql.draft";
const KEEP = 20;

function recall(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function remember(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Private mode or a full quota: the screen works, it just forgets.
  }
}
function recent(): Entry[] {
  try {
    const list: unknown = JSON.parse(recall(RECENT) ?? "[]");
    return Array.isArray(list) ? (list as Entry[]).filter((e) => typeof e?.text === "string").slice(0, KEEP) : [];
  } catch {
    return [];
  }
}

function csvField(v: unknown): string {
  if (v === null || v === undefined) return "";
  const s = typeof v === "object" ? JSON.stringify(v) : String(v);
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

// Same file as the server-rendered page's: a byte-order mark leads so a
// spreadsheet reads it as UTF-8.
function downloadCSV(res: Result) {
  const lines = [res.columns.map(csvField).join(","), ...res.rows.map((r) => r.map(csvField).join(","))];
  const url = URL.createObjectURL(new Blob(["﻿" + lines.join("\r\n") + "\r\n"], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = `query-${new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19)}.csv`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function useBytes() {
  const { lang } = useSession();
  const f = useFormat();
  return (n: number) => {
    for (const [size, unit] of [[2 ** 40, "TB"], [2 ** 30, "GB"], [2 ** 20, "MB"], [2 ** 10, "kB"]] as const) {
      if (n >= size) return `${new Intl.NumberFormat(lang, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(n / size)} ${unit}`;
    }
    return `${f.number(n)} B`;
  };
}

const schemaIcon = (
  <svg className="h-3 w-3 shrink-0" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.2" aria-hidden="true">
    <ellipse cx="6" cy="3" rx="4" ry="1.5" />
    <path d="M2 3v6c0 .8 1.8 1.5 4 1.5s4-.7 4-1.5V3" />
  </svg>
);
const tableIcon = (
  <svg className="h-3 w-3 shrink-0" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.2" aria-hidden="true">
    <rect x="1.5" y="2" width="9" height="8" rx="1" />
    <path d="M1.5 5h9M5 5v5" />
  </svg>
);

/** A grip between two zones: dragging writes a custom property on the workbench, as the server's island does. Arrow keys move it too. */
function Grip({ axis, prop, min, label, host }: { axis: "x" | "y"; prop: string; min: number; label: string; host: React.RefObject<HTMLDivElement | null> }) {
  const set = (px: number, whole: number) => {
    const el = host.current;
    if (!el) return;
    const v = Math.max(min, px);
    el.style.setProperty(prop, prop === "--editor" ? `${Math.min(90, (v / whole) * 100)}%` : `${v}px`);
  };
  const drag = (e: RPointerEvent<HTMLDivElement>) => {
    const el = host.current;
    if (!el) return;
    e.preventDefault();
    const box = (axis === "x" ? el : (el.querySelector(".workbench-main") as HTMLElement)).getBoundingClientRect();
    const move = (m: PointerEvent) => set(axis === "x" ? m.clientX - box.left : m.clientY - box.top, axis === "x" ? box.width : box.height);
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop);
  };
  const step = (e: React.KeyboardEvent) => {
    const d = { ArrowLeft: -16, ArrowUp: -16, ArrowRight: 16, ArrowDown: 16 }[e.key];
    const el = host.current;
    if (d === undefined || !el) return;
    e.preventDefault();
    const target = axis === "x" ? el.querySelector(".workbench-rail") : el.querySelector(".workbench-editor");
    const cur = (target as HTMLElement | null)?.getBoundingClientRect();
    if (!cur) return;
    const whole = (axis === "x" ? el : (el.querySelector(".workbench-main") as HTMLElement)).getBoundingClientRect();
    set((axis === "x" ? cur.width : cur.height) + d, axis === "x" ? whole.width : whole.height);
  };
  return (
    <div
      className="workbench-grip"
      data-grip={axis === "x" ? "rail" : "split"}
      role="separator"
      aria-orientation={axis === "x" ? "vertical" : "horizontal"}
      aria-label={label}
      tabIndex={0}
      onPointerDown={drag}
      onKeyDown={step}
    />
  );
}

// The rail: the title, the filter, then what the connection holds. The words are static; the tree waits for data.
function Rail({ tree, target, onSelect, collapse }: { tree: TreeData | undefined; target: string; onSelect: (sql: string) => void; collapse: () => void }) {
  const { t } = useT();
  const [filter, setFilter] = useState("");
  const [shut, setShut] = useState<Record<string, boolean>>({});
  const [opened, setOpened] = useState<Record<string, boolean>>({});
  // Which relation's columns were asked for last: they are fetched only when somebody opens one.
  const [open, setOpen] = useState("");
  const [schema, name] = open.split(/\.(.*)/s);
  const cols = useQuery({
    queryKey: ["sql", "columns", target, open],
    enabled: open !== "" && target !== "",
    queryFn: () =>
      getJSON<{ columns: { name: string; type: string }[] }>(
        `/api/sql/describe?target=${encodeURIComponent(target)}&schema=${encodeURIComponent(schema ?? "")}&name=${encodeURIComponent(name ?? "")}`,
      ),
    staleTime: 60_000,
  });

  const items = useMemo(() => {
    const out: TreeItem[] = [];
    const q = filter.trim().toLowerCase();
    for (const s of tree?.schemas ?? []) {
      const tables = q ? s.tables.filter((x) => `${s.name}.${x}`.toLowerCase().includes(q)) : s.tables;
      if (q && tables.length === 0) continue;
      const sid = `s:${s.name}`;
      const sopen = q !== "" || !shut[sid];
      out.push({ id: sid, level: 1, expandable: true, expanded: sopen, label: <>{schemaIcon}<span className="min-w-0 truncate">{s.name}</span></> });
      if (!sopen) continue;
      for (const tb of tables) {
        const rid = `${s.name}.${tb}`;
        const ropen = opened[rid] === true;
        out.push({ id: `t:${rid}`, parent: sid, level: 2, label: <>{tableIcon}<span className="min-w-0 truncate font-mono">{tb}</span></> });
        out.push({ id: `x:${rid}`, parent: `t:${rid}`, level: 3, expandable: true, expanded: ropen, label: <span className="text-[0.7rem]">{ropen ? "−" : "+"} {t("sql.tree.columns")}</span> });
        if (!ropen) continue;
        const list = open === rid ? cols.data?.columns : undefined;
        if (!list) out.push({ id: `c:${rid}:`, parent: `x:${rid}`, level: 4, label: <span className="text-[0.7rem]">…</span> });
        else if (list.length === 0) out.push({ id: `c:${rid}:`, parent: `x:${rid}`, level: 4, label: <span className="text-[0.7rem]">{t("sql.tree.no_columns")}</span> });
        for (const c of list ?? []) {
          out.push({
            id: `c:${rid}:${c.name}`, parent: `x:${rid}`, level: 4,
            label: <span className="flex w-full justify-between gap-2 font-mono text-[0.7rem]"><span className="truncate text-ink">{c.name}</span><span className="shrink-0 text-muted">{c.type}</span></span>,
          });
        }
      }
    }
    return out;
  }, [tree, filter, shut, opened, open, cols.data, t]);

  const toggle = (id: string) => {
    if (id.startsWith("s:")) setShut((m) => ({ ...m, [id]: m[id] !== true }));
    else if (id.startsWith("x:")) {
      const rid = id.slice(2);
      setOpened((m) => ({ ...m, [rid]: !m[rid] }));
      if (!opened[rid]) setOpen(rid);
    }
  };
  // A table is its first look, written in the box and run by nobody.
  const activate = (id: string) => (id.startsWith("t:") ? onSelect(`SELECT *\nFROM ${id.slice(2)}\nLIMIT 100;`) : toggle(id));

  return (
    <aside id="objects" className="workbench-rail border-r border-line-soft pr-3 text-xs">
      <div className="mb-2 flex items-center justify-between gap-2">
        <p className="text-[11.5px] font-medium text-muted">{t("sql.tree.title")}</p>
        <button type="button" aria-expanded="true" aria-controls="objects" aria-label={t("sql.tree.collapse")} onClick={collapse} className="rounded-card px-1 py-0.5 text-muted hover:bg-surface-2 hover:text-ink">
          ⇤
        </button>
      </div>
      <input
        type="search"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        placeholder={t("sql.tree.filter")}
        aria-label={t("sql.tree.filter")}
        className="mb-2 w-full rounded-card border border-line bg-surface px-2 py-1 text-xs text-ink placeholder:text-muted"
      />
      {!tree ? (
        <div aria-busy="true" className="grid gap-2 px-2">
          <Skeleton h="0.9rem" w="60%" />
          <Skeleton h="0.9rem" w="80%" />
          <Skeleton h="0.9rem" w="70%" />
        </div>
      ) : (
        <Tree items={items} label={t("sql.tree.title")} onToggle={toggle} onActivate={activate} />
      )}
      {tree?.cut && <p className="mt-2 px-2 text-[0.7rem] text-muted">{t("sql.tree.cut")}</p>}
    </aside>
  );
}

// The cost line: a dry run priced after a pause. Always drawn (empty until something fills it); it takes the toolbar's spare width.
function CostLine({ target, statement, skip }: { target: string; statement: string; skip: string }) {
  const { t } = useT();
  const bytes = useBytes();
  const priced = useRef(skip);
  const [price, setPrice] = useState<Price | null>(null);

  // Priced after a pause, never twice for the same text, and not on load: what
  // is in the box on arrival is not worth an API call.
  useEffect(() => {
    const text = statement.trim();
    if (text === "" || target === "") {
      priced.current = "";
      setPrice(null);
      return;
    }
    if (text === priced.current) return;
    const ctrl = new AbortController();
    const wait = setTimeout(() => {
      priced.current = text;
      // A service that cannot be reached says nothing about the SQL: the line
      // goes quiet rather than red.
      postJSON<Price>("/api/sql/cost", { target, q: text }, ctrl.signal).then((p) => setPrice(p ?? null), () => setPrice(null));
    }, 600);
    return () => {
      clearTimeout(wait);
      ctrl.abort();
    };
  }, [statement, target]);

  return (
    <p aria-live="polite" className="min-w-0 flex-1 truncate text-[0.7rem] text-muted">
      {price?.state === "priced" && <span>{t("sql.cost.priced")} <strong className="font-medium text-ink">{bytes(price.bytes)}</strong></span>}
      {price?.state === "unpriced" && <span>{t("sql.cost.unpriced")}</span>}
      {price?.state === "refused" && <span className="text-state-failed"><strong className="font-medium">{t("sql.cost.refused")}</strong> {price.why}</span>}
    </p>
  );
}

const resultTab = "px-3 py-1.5 text-xs text-muted hover:text-ink aria-selected:text-ink aria-selected:shadow-[inset_0_-2px_0_var(--color-ink)]";

function Workbench({ state }: { state: State | undefined }) {
  const { t, n } = useT();
  const errText = useErrorText();
  const f = useFormat();
  const bytes = useBytes();
  const host = useRef<HTMLDivElement>(null);
  const [picked, setPicked] = useState<string | null>(null);
  const target = picked ?? state?.target ?? "";
  const [statement, setStatement] = useState(() => recall(DRAFT) ?? "");
  const first = useRef(statement);
  const [tab, setTab] = useState<"results" | "json" | "history">("results");
  const [history, setHistory] = useState<Entry[]>(recent);
  const [collapsed, setCollapsed] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const ctrl = useRef<AbortController | null>(null);

  // The server's own statement (a link into the workbench) wins over the draft, once it is known.
  useEffect(() => {
    if (state?.preloaded) {
      first.current = state.statement;
      setStatement(state.statement);
    }
  }, [state]);

  const tree = useQuery({
    queryKey: ["sql", "tree", target],
    enabled: target !== "",
    queryFn: () => getJSON<TreeData>(`/api/sql/tree?target=${encodeURIComponent(target)}`),
    initialData: state && target === state.target ? state.tree : undefined,
    staleTime: 60_000,
  });
  const writable = tree.data?.writable ?? false;
  const costOf = useCallback((r: Result) => `${t("sql.result.read", { bytes: bytes(r.bytes) })} · ${f.duration(r.ms)}`, [t, bytes, f]);

  const record = (text: string, outcome: Entry["outcome"], cost: string) =>
    setHistory((h) => {
      const next = [{ text, outcome, cost }, ...h].slice(0, KEEP);
      remember(RECENT, JSON.stringify(next));
      return next;
    });

  const run = useMutation<Result, ApiError, string>({
    mutationFn: async (text) => {
      ctrl.current = new AbortController();
      return (await postJSON<Result>("/api/sql/run", { target, q: text }, ctrl.current.signal))!;
    },
    onSuccess: (res, text) => record(text, "ok", costOf(res)),
    onError: (err, text) => err.name !== "AbortError" && record(text, "failed", ""),
  });

  useEffect(() => {
    if (!run.isPending) return;
    const start = Date.now();
    setElapsed(0);
    const id = setInterval(() => setElapsed(Date.now() - start), 100);
    return () => clearInterval(id);
  }, [run.isPending]);

  const edit = (v: string) => {
    setStatement(v);
    remember(DRAFT, v);
  };
  const go = () => {
    const text = statement.trim();
    if (text !== "" && target !== "" && !run.isPending) {
      setTab("results");
      run.mutate(text);
    }
  };

  const res = run.data;
  const json = useMemo(() => (tab === "json" && res ? JSON.stringify({ columns: res.columns, rows: res.rows }, null, 2) : ""), [tab, res]);
  const box = "max-w-3xl rounded-card border px-4 py-3 text-sm";

  let answer: ReactNode = null;
  if (run.isPending) {
    answer = (
      <div role="status" className="mb-3 flex items-center gap-3 rounded-card border border-line bg-surface-2 px-3 py-2 text-[0.8rem] text-ink">
        <span className="h-2 w-2 shrink-0 animate-pulse rounded-full bg-state-running" />
        <span>{t("sql.running")} <span className="text-muted">·</span> <span className="tabular-nums">{(elapsed / 1000).toFixed(1)} s</span></span>
        <button type="button" onClick={() => ctrl.current?.abort()} className="ml-auto inline-flex h-7 items-center rounded-card border border-line bg-surface px-2.5 text-[0.78rem] font-medium text-ink transition-colors hover:bg-surface-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong">
          {t("sql.cancel")}
        </button>
      </div>
    );
  } else if (run.error && run.error.name !== "AbortError") {
    const e = run.error;
    const code = e.code;
    // `detail` is the words of the service downstream (the SQL API's reason, the warehouse's message), shown as it came.
    const detail = typeof e.body?.detail === "string" ? e.body.detail : "";
    if (code === "refused") {
      answer = (
        <div role="alert" className={`${box} border-state-failed/40`}>
          <p className="font-medium text-state-failed">{t("sql.err.refused")}</p>
          <p className="mt-1 text-ink-2">{detail}</p>
          <a href="/workflows" className="mt-2 inline-block text-xs text-muted underline decoration-line-strong underline-offset-2 hover:text-ink">{t("sql.readonly.link")}</a>
        </div>
      );
    } else if (code === "no_connection") {
      answer = <p role="alert" className={`${box} border-line-soft text-muted`}>{t("sql.err.no_connection", { connection: String(e.body?.connection ?? "") })}</p>;
    } else if (code === "unknown_destination") {
      answer = <p role="alert" className={`${box} border-line-soft text-muted`}>{t("sql.err.unknown")}</p>;
    } else {
      answer = (
        <div role="alert" className={`${box} border-line-soft`}>
          <p className="font-medium text-ink">{t("sql.err.failed")}</p>
          <p className="mt-1 text-muted">{detail || errText(e)}</p>
        </div>
      );
    }
  } else if (!res) {
    // Before anything runs, how to: the verb and its key, where the answer will be.
    answer = (
      <div className="flex h-full min-h-32 flex-col items-center justify-center gap-2 text-center">
        <span className="flex h-9 w-9 items-center justify-center rounded-full border border-line text-muted">
          <Icon name="play" className="h-4 w-4" />
        </span>
        <p className="text-sm font-medium text-ink">{t("sql.result.none")}</p>
        <p className="flex max-w-md items-center gap-1.5 text-[0.75rem] text-muted">
          {t("sql.result.none.how")} <Kbd keys="⌘↵" />
        </p>
      </div>
    );
  }

  const changed = res?.wrote ? (res.changed === null ? t("sql.result.uncounted") : n("sql.result.changed", res.changed)) : "";
  const note = res
    ? res.wrote
      ? `${changed} · ${costOf(res)}`
      : `${n("sql.result.rows", res.rows.length)} · ${costOf(res)}${res.truncated ? ` ${t("sql.result.more")}` : ""}`
    : "";
  const bar = "bar-row flex shrink-0 flex-nowrap items-center gap-3 overflow-x-auto";
  const ctl = "h-8 shrink-0";

  return (
    <div ref={host} className="workbench" data-rail-collapsed={collapsed ? "true" : undefined}>
      {!collapsed && <Rail tree={tree.data} target={target} onSelect={edit} collapse={() => setCollapsed(true)} />}
      {collapsed && (
        <button type="button" aria-expanded="false" aria-controls="objects" aria-label={t("sql.tree.show")} onClick={() => setCollapsed(false)} className="workbench-rail-show self-start rounded-card px-1 py-1 text-muted hover:bg-surface-2 hover:text-ink">
          ⇥
        </button>
      )}
      {!collapsed && <Grip axis="x" prop="--rail" min={180} label={t("sql.tree.resize")} host={host} />}
      <div className="workbench-main">
        <section className="workbench-editor">
          {writable && (
            <p className="workbench-writable mb-2 shrink-0 rounded px-2 py-1 text-[0.7rem]">
              {t("sql.writable.before")} <strong>{t("sql.writable.strong")}</strong> {t("sql.writable.after")}
            </p>
          )}
          {/* One row: the verbs, where they run, what the run will cost, and the copy of the text. */}
          <div className={`${bar} pb-2`}>
            <button type="button" onClick={go} disabled={run.isPending || statement.trim() === "" || target === ""} className={`${buttonClass("primary")} ${ctl}`}>
              <Icon name="play" className="h-3.5 w-3.5" />
              {t("sql.run")}
            </button>
            {run.isPending && (
              <button type="button" onClick={() => ctrl.current?.abort()} className={`${buttonClass("danger")} ${ctl}`}>
                {t("sql.cancel")}
              </button>
            )}
            <Kbd keys="⌘↵" />
            <DropdownFilter
              def={{
                id: "connection", label: t("sql.tree.connection"), required: true, mono: true, pending: !state,
                options: (state?.connections ?? []).map((c) => ({ key: c.target, label: c.name, disabled: !c.queryable })),
                value: target ? [target] : [], onChange: (v) => v[0] && setPicked(v[0]),
              }}
            />
            {writable && (
              <span className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-card bg-state-queued/13 px-2 text-xs font-medium text-state-queued">
                <Icon name="triangle-alert" className="h-3.5 w-3.5" />
                {t("sql.writes.allowed")}
              </span>
            )}
            <CostLine target={target} statement={statement} skip={first.current.trim()} />
            <button type="button" onClick={() => void navigator.clipboard?.writeText(statement)} className={`${buttonClass("secondary")} ${ctl}`}>
              <Icon name="copy" className="h-3.5 w-3.5" />
              {t("sql.copy")}
            </button>
          </div>
          <div className="workbench-code flex min-h-0 flex-1 flex-col">
            <CodeEditor id="q" value={statement} onChange={edit} onRun={go} label={t("sql.editor.label")} placeholder="SELECT * FROM dataset.table WHERE day = CURRENT_DATE()" />
          </div>
          {!writable && (
            <p className="mt-2 shrink-0 text-[0.7rem] text-muted">
              {t("sql.readonly")}{" "}
              <a href="/workflows" className="underline decoration-line-strong underline-offset-2 hover:text-ink">{t("sql.readonly.link")}</a>
            </p>
          )}
          <p className="mt-1 shrink-0 text-[0.7rem] text-muted">
            <kbd className="rounded border border-line px-1 py-0.5 font-mono">Ctrl+Space</kbd> {t("sql.editor.complete")}
          </p>
        </section>
        <Grip axis="y" prop="--editor" min={120} label={t("sql.editor.resize")} host={host} />
        <section className="workbench-results">
          {/* One row too: the tabs over the answer, then what the run said and the export. */}
          {/* The CSV button (26px plus its margins) comes with the first result: the row is that tall from the start, or the answer under it drops 6px when the result lands. */}
          <div className={`${bar} gap-1 border-b border-line-soft`} style={{ minHeight: 35 }}>
            <div role="tablist" aria-label={t("sql.result.tabs")} className="flex shrink-0 items-center gap-1">
              {(["results", "json", "history"] as const).map((k) => (
                <button key={k} type="button" role="tab" id={`tab-${k}`} aria-selected={tab === k} aria-controls="wb-panel" className={resultTab} onClick={() => setTab(k)}>
                  {k === "results" ? t("sql.result.tab.results") : k === "json" ? "JSON" : t("sql.result.tab.history")}
                  {k === "history" && <span className="ml-1 text-faint tabular-nums">{history.length}</span>}
                </button>
              ))}
            </div>
            <span className="min-w-0 flex-1" />
            <span role="status" aria-live="polite" className="shrink-0 text-[0.7rem] whitespace-nowrap text-muted">{run.isPending ? "" : note}</span>
            {res && !res.wrote && (
              <button type="button" onClick={() => downloadCSV(res)} className={`${buttonClass("secondary", "small")} my-1`}>
                <Icon name="download" className="h-3.5 w-3.5" />
                CSV
              </button>
            )}
          </div>
          <div id="wb-panel" role="tabpanel" aria-labelledby={`tab-${tab}`} className="workbench-answer flex flex-col pt-2">
            {tab === "history" ? (
              <div className="min-h-0 flex-1 overflow-auto py-2 text-xs">
                {history.length === 0 ? (
                  <p className="px-2 text-[0.75rem] text-muted">{t("sql.history.empty")}</p>
                ) : (
                  <ol className="space-y-0.5">
                    {history.map((e, i) => (
                      <li key={i}>
                        <button type="button" onClick={() => { edit(e.text); setTab("results"); }} className="block w-full rounded-card px-2 py-1 text-left hover:bg-surface-2">
                          <span className="flex items-center gap-1.5">
                            <span className={`shrink-0 text-[0.7rem] ${e.outcome === "ok" ? "text-state-success" : "text-state-failed"}`}>{e.outcome === "ok" ? t("sql.history.ok") : t("sql.history.failed")}</span>
                            <span className="truncate text-[0.7rem] text-muted">{e.cost}</span>
                          </span>
                          <span className="mt-0.5 block truncate font-mono text-[0.7rem] text-ink">{e.text}</span>
                        </button>
                      </li>
                    ))}
                  </ol>
                )}
                <p className="mt-2 px-2 text-[0.7rem] text-faint">{t("sql.history.where")}</p>
              </div>
            ) : answer ? (
              answer
            ) : res && tab === "json" ? (
              <pre className="min-h-0 flex-1 overflow-auto p-2 font-mono text-[0.7rem] whitespace-pre text-ink">{json}</pre>
            ) : res && res.wrote ? (
              <p className="px-3 py-4 text-sm text-ink">{changed}</p>
            ) : res ? (
              <DataGrid columns={res.columns} rows={res.rows} label={t("sql.result.tab.results")} rowLabel="Row" emptyText={n("sql.result.rows", 0)} />
            ) : null}
          </div>
        </section>
      </div>
    </div>
  );
}

function Unavailable() {
  const { t } = useT();
  return <p className="max-w-3xl rounded-card border border-line-soft px-4 py-3 text-sm text-muted">{t("sql.empty.catalog")}</p>;
}

function Screen() {
  const { search } = useLocation();
  // The workbench is drawn at once; the state (connections, the tree's first level) fills it in.
  const { data, isPending } = useQuery({
    queryKey: ["sql", "state", search],
    queryFn: () => getJSON<State>(`/api/sql/state${search}`),
    staleTime: 0,
    gcTime: 0,
    retry: (c, e) => !(e instanceof ApiError && e.status === 404) && c < 1,
  });
  const none = !isPending && data && data.connections.length === 0;
  if (none) return <Unavailable />;
  return <Workbench key={search} state={data} />;
}

export default function SqlRoute() {
  const { sql } = useConfig();
  // The workbench fills the shell: three panes that scroll on their own.
  useFill();
  // main[data-fill] > :last-child is the one flex column; the workbench sits inside the page's gutters, as in the server page.
  // Without a SQL service the server has no such page (a 404); here the path says so in the empty-catalog voice.
  return <Page>{sql ? <Screen /> : <Unavailable />}</Page>;
}
