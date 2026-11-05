import type { Flow } from "../api/ingest";
import { useT } from "../i18n";
import { Icon } from "./icon";

const ICON: Record<string, string> = { source: "plug", buffer: "rows-3", dead_letter: "triangle-alert" };

/**
 * How data reaches a destination, as the configuration declares it: nodes left
 * to right (top to bottom on a phone), the destination marked, and the dead
 * letter as a dashed branch under it. Words arrive finished from the server.
 */
export function FlowDiagram({ flow }: { flow: Flow }) {
  const { t } = useT();
  const title = (kind: string) => t(`flow.${kind}`);
  return (
    <div className="flow-ground px-4 py-4 sm:px-6">
      <ol className="flex flex-col gap-3 min-[700px]:flex-row min-[700px]:items-stretch">
        {flow.nodes.map((n, i) => (
          <li
            key={i}
            data-flow-node={n.kind}
            className={`flow-step min-w-0 flex-1 rounded-card border bg-surface px-3 py-2 ${n.kind === "destination" ? "border-ink-2" : "border-line"}`}
          >
            {i > 0 && <span className="flow-packet" aria-hidden="true" />}
            <p className="flex items-center gap-1.5 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
              <Icon name={ICON[n.kind] ?? "database"} className="h-3.5 w-3.5" />
              {i + 1} · {title(n.kind)}
            </p>
            <p className="mt-1 font-mono text-[0.78rem] break-all text-ink">{n.main}</p>
            {n.detail !== "" && <p className="mt-0.5 text-[0.72rem] text-muted">{n.detail}</p>}
          </li>
        ))}
      </ol>
      {flow.deadLetter && (
        <div className="mt-3 flex min-[700px]:justify-end">
          <div data-flow-node={flow.deadLetter.kind} className="flow-branch w-full min-w-0 rounded-card border border-dashed border-state-failed/60 px-3 py-2 min-[700px]:w-[calc((100%-1.5rem)/3)]">
            <p className="flex items-center gap-1.5 text-[0.68rem] font-medium tracking-[0.08em] text-state-failed uppercase">
              <Icon name="triangle-alert" className="h-3.5 w-3.5" />
              {title(flow.deadLetter.kind)}
            </p>
            <p className="mt-1 font-mono text-[0.78rem] break-all text-ink">{flow.deadLetter.main}</p>
            <p className="mt-0.5 text-[0.72rem] text-state-failed">{flow.deadLetter.detail}</p>
          </div>
        </div>
      )}
    </div>
  );
}
