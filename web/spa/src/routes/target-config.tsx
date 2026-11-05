import { Link } from "react-router";
import type { CfgRow } from "../api/ingest";
import { useMsg, useT, type Cat, type Msg } from "../i18n";
import { stamp, useBrief } from "../ui/format";
import { ConfigNote, CfgRows, DeadLetterPanel, useSectionTitle } from "../ui/cfg";
import { FlowDiagram } from "../ui/flow";
import { linkUnderline } from "../ui/kit";

type Note = { head: string; rest: string };
type Node = { kind: string; main: Cat | null; detail: Cat | null };
type Row = { path: string; label: string; value: string; default: boolean; note: Note | null; before: { label: string; note: Note }[] };
export type WriterConfig = {
  key: string;
  gateway: string;
  stream: string;
  role: string;
  publishedAt: string | null;
  workflow: string;
  node: string;
  described: boolean;
  summary: Cat | null;
  flow: Node[];
  flowDeadLetter: Node | null;
  deadLetter: { where: Cat | null; readable: boolean; commands: string; write: string } | null;
  note: Note | null;
  sections: { key: string; title: Msg | null; note: Note | null; rows: Row[] }[];
  changes: { at: string; runId: string; first: boolean; lines: { path: string; text: Cat }[] }[];
};

const whole = (n: Note) => (n.rest ? `${n.head}\n${n.rest}` : n.head);
// The card's rows are the load page's rows, with the note folded back into one string.
const asRows = (rows: Row[]): CfgRow[] =>
  rows.map((r) => ({ ...r, note: r.note ? whole(r.note) : "", list: [], before: r.before.map((b) => ({ label: b.label, text: whole(b.note) })) }));

function One({ c }: { c: WriterConfig }) {
  const { t } = useT();
  const msg = useMsg();
  const sectionTitle = useSectionTitle();
  const brief = useBrief();
  return (
    <section className="mb-3" data-config={c.key}>
      <div className="mb-2 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h3 className="text-[13px] font-semibold text-ink">{t("config.title")}</h3>
        <span className="text-xs text-muted">
          {c.gateway ? (
            <>
              gateway <span className="font-medium text-ink">{c.gateway}</span> · <span className="font-mono">{c.stream}</span> · {c.role} ·{" "}
              {t("config.published", { at: c.publishedAt ? `${brief(c.publishedAt)} UTC` : "" })}
            </>
          ) : (
            <>
              {t("config.step")}{" "}
              <Link to={`/workflows/${encodeURIComponent(c.workflow)}`} className="font-medium text-ink hover:underline">
                {c.workflow}
              </Link>{" "}
              · {c.node}
            </>
          )}
        </span>
      </div>
      {!c.described ? (
        <p className="rounded-card border border-line bg-surface px-4 py-3 text-xs text-muted shadow-warm-sm sm:px-6">
          {t("config.undescribed")}{" "}
          <Link to={`/workflows/${encodeURIComponent(c.workflow)}`} className={linkUnderline}>
            {t("config.definition")}
          </Link>
          .
        </p>
      ) : (
        <div className="divide-y divide-line-soft rounded-card border border-line bg-surface shadow-warm-sm">
          <p className="px-4 py-3 text-sm text-ink sm:px-6">{msg(c.summary)}</p>
          {c.flow.length > 0 && <FlowDiagram flow={{ nodes: c.flow, deadLetter: c.flowDeadLetter }} />}
          {c.gateway && c.deadLetter && <DeadLetterPanel dl={c.deadLetter} />}
          {c.note && (
            <div className="px-4 py-3 sm:px-6">
              <ConfigNote text={whole(c.note)} />
            </div>
          )}
          {c.sections.map((s) => (
            <div key={s.key} className="px-4 py-3 sm:px-6">
              <h4 className="text-xs font-semibold text-ink">{sectionTitle(s)}</h4>
              {s.note && (
                <div className="mt-1">
                  <ConfigNote text={whole(s.note)} />
                </div>
              )}
              <CfgRows rows={asRows(s.rows)} wide />
            </div>
          ))}
          {c.changes.length > 0 && (
            <div className="px-4 py-3 sm:px-6">
              <h4 className="text-xs font-semibold text-ink">{t("config.changes")}</h4>
              <ol className="mt-2 space-y-2">
                {c.changes.map((ch) => (
                  <li key={ch.at} className="text-xs">
                    <span className="text-muted tabular-nums">{stamp(ch.at)}</span>{" "}
                    {ch.runId && (
                      <Link to={`/runs/${ch.runId}`} className={`ml-2 font-mono ${linkUnderline}`}>
                        {t("config.run", { run: ch.runId.slice(0, 8) })}
                      </Link>
                    )}{" "}
                    {ch.first && c.gateway ? (
                      <span className="ml-2 text-muted">{t("config.first_published")}</span>
                    ) : ch.first ? (
                      <span className="ml-2 text-muted">{t("config.first_recorded")}</span>
                    ) : ch.lines.length === 0 ? (
                      <span className="ml-2 text-muted">{t("config.earlier")}</span>
                    ) : null}{" "}
                    {ch.lines.length > 0 && (
                      <ul className="mt-1 space-y-0.5 pl-4">
                        {ch.lines.map((l) => (
                          <li key={l.path}>
                            <span className="font-mono text-muted">{l.path}</span>{" "}
                            <span className="ml-2 font-mono break-all text-ink">{msg(l.text)}</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </li>
                ))}
              </ol>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

/** How each writer of a destination is configured: its settings by section, the defaults marked. */
export function Configurations({ configs }: { configs: WriterConfig[] }) {
  return (
    <>
      {configs.map((c) => (
        <One key={c.key} c={c} />
      ))}
    </>
  );
}
