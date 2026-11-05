import type { CSSProperties } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { useFormat, useMsg, useT, type Msg } from "../../i18n";
import { langLabel } from "../lang";
import { Logo, hasMark } from "../logo";

// The wire keys (name, state, numbers, label, action, error, attempt,
// duration_ms, instances, group, node type `phase`) are internal/api/graph.go's
// payload; they are read here as they arrive.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type NodeData = Record<string, any>;
type P = NodeProps & { data: NodeData };

const known = new Set(["success", "failed", "running", "retrying", "queued", "canceled", "pending", "skipped"]);
/** A step's state as the colour token it is drawn in. */
export const tone = (s: string | undefined) => (s && known.has(s) ? s : "pending");
// A stage's state reuses the steps' palette: `done` is the same green as `success`.
const STAGE_TONE: Record<string, string> = { done: "success", running: "running", failed: "failed", aborted: "canceled" };
/** A name the data owns (a string) or a sentence the app says (the marker, a stage). */
export const useWords = () => {
  const msg = useMsg();
  return (v: string | Msg | null | undefined) => (typeof v === "string" ? v : msg(v));
};
/** Where a step's runtime came from, in words ("declared" by the step, "inferred" from its command); a source this version does not know is shown as written. */
export const useSourceWord = () => {
  const { t } = useT();
  return (src: unknown) => (src === "declared" ? t("graph.source.declared") : src === "inferred" ? t("graph.source.inferred") : String(src ?? "?"));
};
export const stageTone = (s: string | undefined) => STAGE_TONE[s ?? ""] ?? "pending";
// A language's own token; the chip falls back to the muted ink where the sheet has none.
const tint = (id: string) => ({ "--t": `var(--color-lang-${id}, var(--muted))` }) as CSSProperties;
const colour = (t: string) => ({ "--c": `var(--st-${t})` }) as CSSProperties;

export { langLabel };

// The numbers worth showing on a stage's line, in order (dag.js's HIGHLIGHT).
const HIGHLIGHT = ["in", "out", "groups", "rows", "pages", "objects"];

export function StepCard({ data: d }: P) {
  const { t, n } = useT();
  const words = useWords();
  const sourceWord = useSourceWord();
  const { duration, number } = useFormat();
  // A workflow's graph draws no run: its cards are pending and say how the step went last time.
  const last = d.status === "pending" && d.last_status;
  const st = tone(last ? d.last_status : d.status);
  const took = last ? d.last_duration_ms : d.duration_ms;
  const tools: string[] = d.tools ?? [];
  const inferred = d.runtime_source === "inferred";
  const lang: string = d.runtime || tools[0] || "";
  const mark = hasMark(lang) ? lang : "";
  const published = d.context ? Object.keys(d.context).length : 0;
  return (
    <div className="dag-card" style={colour(st)} data-status={d.status} data-tall={d.hasStages ? "" : undefined}>
      <Handle type="target" position={Position.Left} />
      <div className="dag-row">
        <span className="dag-dot" />
        {mark && <Logo k={mark} size={18} />}
        <span className="dag-name" title={d.label}>
          {d.label}
        </span>
        {d.instances ? (
          <span className="dag-inst" title={`${d.instances_ok} / ${d.instances}`}>
            {d.instances_ok === d.instances ? `[${d.instances}]` : `[${d.instances_ok || 0}/${d.instances}]`}
          </span>
        ) : null}
        {d.hasStages && (
          <button
            type="button"
            className="dag-toggle nodrag"
            aria-expanded={!d.collapsed}
            aria-label={t(d.collapsed ? "graph.expand" : "graph.collapse")}
            onClick={(e) => {
              e.stopPropagation();
              d.toggle();
            }}
          >
            {d.collapsed ? "▸" : "▾"}
          </button>
        )}
      </div>
      {d.action && <div className="dag-cmd">{words(d.action)}</div>}
      {(d.runtime || tools.length > 0) && (
        <div className="dag-chips">
          {d.runtime && (
            <span className={`dag-chip strong${inferred ? " inferred" : ""}`} style={tint(d.runtime)} title={`${langLabel(d.runtime)} — ${inferred ? t("graph.inferred") : sourceWord(d.runtime_source)}`}>
              {langLabel(d.runtime)}
            </span>
          )}
          {tools.map((x) => (
            <span key={x} className={`dag-chip${inferred ? " inferred" : ""}`}>
              {langLabel(x)}
            </span>
          ))}
          {d.image && (
            <span className="dag-image" title={d.image}>
              {d.image}
            </span>
          )}
        </div>
      )}
      {published > 0 && <div className="dag-pub">{n("graph.published", published)}</div>}
      <div className="dag-status">
        <span>{t(`state.${st}`)}</span>
        {took ? <span className="took">{duration(took)}</span> : null}
        {d.attempt ? <span className="retry">{t("graph.retry", { n: number(d.attempt) })}</span> : null}
        {d.sdk && <span className="dag-sdk">SDK {d.sdk}</span>}
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

/** A phase inside an SDK step: the source, each stage in the order it runs, the target. */
export function StageBox({ data: d }: P) {
  const { number, duration } = useFormat();
  const words = useWords();
  const nums = HIGHLIGHT.flatMap((k) => {
    const v = d.numbers?.[k];
    return v === null || v === undefined || v === "" ? [] : [{ k, v: typeof v === "number" ? number(v) : String(v) }];
  }).slice(0, 2);
  const detail: string | undefined = d.numbers?.detail;
  return (
    <div className="dag-stage" style={colour(stageTone(d.state))} data-state={d.state} title={detail ?? ""}>
      <span className="d" />
      <b>{words(d.label || d.name)}</b>
      <span className="detail">{detail}</span>
      <span className="nums">
        {nums.map((n) => (
          <span key={n.k}>
            <span>{n.v}</span> {n.k}
          </span>
        ))}
        {d.ms !== null && d.ms !== undefined && <span>{duration(d.ms)}</span>}
      </span>
    </div>
  );
}

/** The box behind a set of steps. A plain node placed first, so it draws behind the rest. */
export function GroupBox({ data: d }: P) {
  return (
    <div className="dag-group">
      <button type="button" className="nodrag" aria-expanded={!d.collapsed} onClick={(e) => { e.stopPropagation(); d.onToggle(); }}>
        <span style={{ fontSize: 9 }}>{d.collapsed ? "▸" : "▾"}</span>
        {d.label}
        {d.members ? <small>· {d.members} {d.members === 1 ? "step" : "steps"}</small> : null}
      </button>
    </div>
  );
}

/** "01 · Stage 1" over each column. A node, so that fitting the view counts it. */
export function ColumnLabel({ data: d }: P) {
  return <div className="dag-col">{d.label}</div>;
}

export const NODE_TYPES = { brevis: StepCard, phase: StageBox, group: GroupBox, stage: ColumnLabel };
