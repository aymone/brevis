import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Background, Controls, ReactFlow, type Edge, type Node, type NodeChange, type NodeTypes } from "@xyflow/react";
import { useQuery } from "@tanstack/react-query";
import "@xyflow/react/dist/style.css";
import { getJSON } from "../../api/client";
import { useFallbackInterval, useLive } from "../../live/hooks";
import { useFormat, useT } from "../../i18n";
import { Skeleton } from "../skeleton";
import { dagHeight, rememberDagHeight } from "./height";
import { langLabel, NODE_TYPES, stageTone, tone, useSourceWord, useWords, type NodeData } from "./nodes";
import "./dag.css";
import { useErrorText } from "../../api/errors";

type GNode = Node<NodeData> & { parentId?: string };
export type Graph = { slug: string; run_id?: string; status?: string; terminal: boolean; nodes: GNode[]; edges: Edge[] };

type Props = {
  /** The graph's URL: /api/runs/{id}/graph or /api/workflows/{slug}/graph. */
  src?: string;
  /** Told which step is chosen (null when none), for a page that says so under the graph. */
  onPick?: (id: string | null) => void;
  /** Shorthand for a run's graph; it refetches when the server reports the run. */
  runId?: string;
};

// The drawing's room: how much air fitView leaves, and how far in it may zoom.
const FIT_PAD = 0.09;
const MAX_ZOOM = 1.5;
const NODE_W = 230; // nodeWidth in internal/api/graph.go

// The topic that says this graph changed: the run's, or the workflow's own.
const topicOf = (runId: string | undefined, src: string | undefined) => {
  if (runId) return `run:${runId}`;
  const slug = src?.match(/^\/api\/workflows\/([^/]+)\/graph/)?.[1];
  return slug ? `workflow:${decodeURIComponent(slug)}` : "";
};

// Column labels, from where the server put the steps.
function columnLabels(nodes: GNode[], word: string): GNode[] {
  const xs = new Set<number>();
  let top = Infinity;
  for (const n of nodes) {
    if (n.parentId || n.type === "group" || n.type === "stage") continue;
    xs.add(n.position.x);
    top = Math.min(top, n.position.y);
  }
  const cols = [...xs].sort((a, b) => a - b);
  if (cols.length < 2) return [];
  return cols.map((x, i) => ({
    id: `stage-${x}`,
    type: "stage",
    position: { x, y: top - 30 },
    selectable: false,
    draggable: false,
    connectable: false,
    focusable: false,
    style: { width: NODE_W, height: 18 },
    data: { label: `${String(i + 1).padStart(2, "0")} · ${word} ${i + 1}` },
  }));
}

// The height the drawing needs at the zoom that fits its width: wide, short
// graphs would leave an empty band under a fixed height.
function heightFor(nodes: GNode[], width: number): number {
  let x1 = Infinity, y1 = Infinity, x2 = -Infinity, y2 = -Infinity;
  for (const n of nodes) {
    if (n.parentId) continue;
    const w = Number(n.style?.width ?? NODE_W);
    const h = Number(n.style?.height ?? 60);
    x1 = Math.min(x1, n.position.x);
    y1 = Math.min(y1, n.position.y);
    x2 = Math.max(x2, n.position.x + w);
    y2 = Math.max(y2, n.position.y + h);
  }
  if (!isFinite(x1) || !width) return 380;
  const zoom = Math.min(MAX_ZOOM, (width * (1 - 2 * FIT_PAD)) / Math.max(1, x2 - x1));
  return Math.max(200, Math.min(660, Math.round(((y2 - y1) * zoom) / (1 - 2 * FIT_PAD))));
}

function ties(edges: Edge[]) {
  const up = new Map<string, string[]>();
  const down = new Map<string, string[]>();
  for (const e of edges) {
    up.set(e.target, [...(up.get(e.target) ?? []), e.source]);
    down.set(e.source, [...(down.get(e.source) ?? []), e.target]);
  }
  const walk = (id: string, next: Map<string, string[]>) => {
    const seen = new Set<string>();
    const stack = [...(next.get(id) ?? [])];
    for (let n = stack.pop(); n !== undefined; n = stack.pop()) {
      if (seen.has(n)) continue;
      seen.add(n);
      stack.push(...(next.get(n) ?? []));
    }
    return [...seen];
  };
  return { around: (id: string) => new Set([id, ...walk(id, up), ...walk(id, down)]) };
}

function Inspector({ node, stages, close }: { node: GNode; stages: GNode[]; close: () => void }) {
  const { t } = useT();
  const words = useWords();
  const sourceWord = useSourceWord();
  const { duration } = useFormat();
  const d = node.data;
  const ctx = d.context ? Object.entries(d.context as Record<string, unknown>).sort(([a], [b]) => a.localeCompare(b)) : [];
  const avail = d.available ? (Object.entries(d.available) as [string, Record<string, unknown>][]).sort(([a], [b]) => a.localeCompare(b)) : [];
  const rows: [string, string][] = [
    [t("graph.insp.step"), node.id],
    [t("graph.insp.state"), t(`state.${tone(d.status)}`)],
    ...(d.action ? [[t("graph.insp.command"), words(d.action)] as [string, string]] : []),
    ...(d.duration_ms ? [[t("graph.insp.duration"), duration(d.duration_ms)] as [string, string]] : []),
    ...(typeof d.attempt === "number" ? [[t("graph.insp.attempt"), String(d.attempt + 1)] as [string, string]] : []),
    ...(typeof d.exit_code === "number" ? [[t("graph.insp.exit_code"), String(d.exit_code)] as [string, string]] : []),
    ...(d.runtime ? [[t("graph.insp.runtime"), `${langLabel(d.runtime)} (${sourceWord(d.runtime_source)})`] as [string, string]] : []),
    ...(d.tools?.length ? [[t("graph.insp.tools"), (d.tools as string[]).map(langLabel).join(", ")] as [string, string]] : []),
    // Values as they were written: a number stays a number.
    ...ctx.map(([k, v]) => [`context.${k}`, typeof v === "string" ? v : JSON.stringify(v)] as [string, string]),
    // What the steps above published and this one may read.
    ...avail.flatMap(([step, values]) =>
      Object.entries(values ?? {})
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([k, v]) => [t("graph.insp.available", { name: `${step}.${k}` }), `${typeof v === "string" ? v : JSON.stringify(v)}  \u2190 ${step}`] as [string, string]),
    ),
  ];
  return (
    <aside className="dag-insp shadow-warm" aria-label={t("graph.inspector")}>
      <header>
        <h4>{t("graph.details")}</h4>
        <button type="button" onClick={close} aria-label={t("dialog.close")}>
          ×
        </button>
      </header>
      <dl>
        {rows.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
      {stages.length > 0 && (
        <div className="stages">
          <h4>{d.sdk ? t("graph.insp.stages_sdk", { sdk: d.sdk }) : t("graph.insp.stages")}</h4>
          {stages.map((s) => (
            <div key={s.id} className="stage-row" style={{ color: `var(--st-${stageTone(s.data.state)})` }}>
              <b>{words(s.data.label || s.data.name)}</b>
              <span className="muted">{s.data.ms !== null && s.data.ms !== undefined ? duration(s.data.ms) : "—"}</span>
              {Object.entries((s.data.numbers ?? {}) as Record<string, unknown>).map(([k, v]) => (
                <span key={k} className="mono" style={{ color: "var(--fg)" }}>
                  {k}={String(v)}
                </span>
              ))}
            </div>
          ))}
        </div>
      )}
      {d.error && <pre>{d.error}</pre>}
    </aside>
  );
}

/**
 * The workflow's flow: nodes, edges and states from the server's graph JSON
 * (positions are the executor's own levels), drawn with React Flow. Give it a
 * graph URL, or a run id for a run's graph; it refetches when the server reports the run or workflow.
 */
export default function Dag({ src, runId, onPick }: Props) {
  const url = src ?? (runId ? `/api/runs/${runId}/graph` : "");
  const topic = topicOf(runId, src);
  useLive(topic ? [topic] : [], [["graph", url]]);
  const fallback = useFallbackInterval();
  const { data: g, error, isPending } = useQuery({
    queryKey: ["graph", url],
    queryFn: () => getJSON<Graph>(url),
    enabled: url !== "",
    staleTime: 0,
    // Only a run in flight has anything to follow while the stream is down.
    refetchInterval: (q) => (!q.state.data || q.state.data.terminal || !q.state.data.run_id ? false : fallback),
  });
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  useEffect(() => onPick?.(selected), [selected, onPick]);
  const [collapsed, setCollapsed] = useState<Record<string, true>>({});

  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    setWidth(el.clientWidth);
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Esc lets go of the chosen step.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setSelected(null);
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const { t } = useT();
  const errText = useErrorText();
  const stageWord = t("graph.stage");
  // React Flow's own screen-reader texts (the zoom buttons, what a focused step says): its English is the default.
  const aria = useMemo(
    () => ({
      "controls.ariaLabel": t("graph.a11y.controls"),
      "controls.zoomIn.ariaLabel": t("graph.a11y.zoom_in"),
      "controls.zoomOut.ariaLabel": t("graph.a11y.zoom_out"),
      "controls.fitView.ariaLabel": t("graph.a11y.fit"),
      "node.a11yDescription.default": t("graph.a11y.node"),
    }),
    [t],
  );
  const toggle = useCallback((id: string) => {
    setCollapsed((m) => {
      const { [id]: was, ...rest } = m;
      return was ? rest : { ...m, [id]: true };
    });
  }, []);

  const view = useMemo(() => {
    if (!g) return null;
    const all = g.nodes ?? [];
    const withStages = new Set(all.flatMap((n) => (n.parentId ? [n.parentId] : [])));
    // A collapsed group swallows its steps (and their phases); edges into it point at the box.
    const hidden = new Map<string, string>();
    for (const n of all) if (n.type === "group" && collapsed[n.id]) hidden.set(String(n.data.label), n.id);
    const swallowed = new Map<string, string>();
    for (const n of all) {
      const grp = n.data?.group as string | undefined;
      if (grp && hidden.has(grp)) swallowed.set(n.id, hidden.get(grp)!);
    }

    let nodes: GNode[] = [];
    for (const n of all) {
      if (n.parentId && collapsed[n.parentId]) continue;
      if (swallowed.has(n.id) || (n.parentId && swallowed.has(n.parentId))) continue;
      if (n.type === "group") {
        const members = all.filter((o) => o.data?.group === n.data.label).length;
        nodes.push({
          ...n,
          data: { ...n.data, members: members, collapsed: !!collapsed[n.id], onToggle: () => toggle(n.id) },
          style: collapsed[n.id] ? { width: NODE_W, height: 46 } : n.style,
        });
      } else if (withStages.has(n.id)) {
        nodes.push({ ...n, data: { ...n.data, hasStages: true, collapsed: !!collapsed[n.id], toggle: () => toggle(n.id) }, style: collapsed[n.id] ? undefined : n.style });
      } else nodes.push(n);
    }

    // The focus: a chosen step, what it waits for and what it feeds stay; the rest steps back.
    const sel = selected ? all.find((n) => n.id === selected) : undefined;
    const path = sel ? ties(g.edges ?? []).around(sel.parentId ?? sel.id) : null;
    if (path) {
      nodes = nodes.map((n) => (n.type === "group" || path.has(n.parentId ?? n.id) ? n : { ...n, style: { ...n.style, opacity: 0.35 } }));
    }
    const labels = columnLabels(all, stageWord);
    const edges = (g.edges ?? [])
      .map((e) => ({ ...e, source: swallowed.get(e.source) ?? e.source, target: swallowed.get(e.target) ?? e.target }))
      .filter((e) => e.source !== e.target)
      .map((e) => ({ ...e, ariaLabel: t("graph.edge", { from: e.source, to: e.target }) }))
      .map((e) => (!path || (path.has(e.source) && path.has(e.target)) ? e : { ...e, style: { ...e.style, opacity: 0.25 } }));
    return { nodes: [...nodes, ...labels], edges, sel, height: heightFor([...all, ...labels], width) };
  }, [g, collapsed, selected, toggle, width, stageWord, t]);

  // Selecting by click or by keyboard (Enter/Space on a focused node) arrives as a change.
  const onNodesChange = useCallback((changes: NodeChange[]) => {
    for (const c of changes) if (c.type === "select" && c.selected) setSelected((s) => (s === c.id ? null : c.id));
  }, []);

  const shape = g?.nodes?.length ?? 0;
  // Only a height measured at a real width is worth keeping (heightFor says 380 while the width is unknown).
  useEffect(() => {
    if (view && width > 0 && url) rememberDagHeight(url, view.height);
  }, [view, width, url]);
  return (
    <div ref={box} className="dag rounded-card border border-line bg-surface shadow-warm-sm" style={{ height: view?.height ?? dagHeight(url) }} aria-busy={isPending}>
      {isPending && <Skeleton h="100%" />}
      {error && <div className="dag-err" role="alert">{t("graph.failed", { error: errText(error) })}</div>}
      {view && (
        <>
          {/* A key on the shape remounts React Flow when the drawing changes: the only reliable re-fit. */}
          <ReactFlow
            key={`shape-${shape}-${view.height}`}
            nodes={view.nodes}
            edges={view.edges}
            nodeTypes={NODE_TYPES as unknown as NodeTypes}
            fitView
            fitViewOptions={{ padding: FIT_PAD, maxZoom: MAX_ZOOM }}
            minZoom={0.4}
            maxZoom={1.6}
            nodesDraggable={false}
            nodesConnectable={false}
            edgesFocusable={false}
            onNodesChange={onNodesChange}
            onPaneClick={() => setSelected(null)}
            defaultEdgeOptions={{
              style: { stroke: "var(--color-line-strong)", strokeWidth: 1.4 },
              labelStyle: { fill: "var(--muted)", fontSize: 11, fontWeight: 500 },
              labelBgStyle: { fill: "var(--card)", fillOpacity: 0.92 },
              labelBgPadding: [6, 3],
              labelBgBorderRadius: 6,
            }}
            aria-label={t("run.tab.steps")}
            ariaLabelConfig={aria}
          >
            <Background gap={22} size={1.4} color="var(--st-pending)" />
            <Controls showInteractive={false} />
          </ReactFlow>
          {view.sel && (
            <Inspector
              node={view.sel.parentId ? (g?.nodes.find((n) => n.id === view.sel?.parentId) ?? view.sel) : view.sel}
              stages={(g?.nodes ?? []).filter((n) => n.parentId === (view.sel?.parentId ?? view.sel?.id))}
              close={() => setSelected(null)}
            />
          )}
        </>
      )}
    </div>
  );
}
