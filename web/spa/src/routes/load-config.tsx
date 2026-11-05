import type { CfgRow, StepConfig } from "../api/ingest";
import { useT } from "../i18n";
import { CfgRows, ConfigNote } from "../ui/cfg";
import { FlowDiagram } from "../ui/flow";
import { HashTabs, type HashTab } from "../ui/hash-tabs";
import { DefinitionList } from "../ui/kit";

// Hidden: the tab names the panel (the server hides it too when the tabs are there).
const h3 = "hidden px-3.5 pt-3 text-[12.5px] font-semibold text-ink";
const th = "px-2.5 py-2 text-[11.5px] font-medium whitespace-nowrap text-faint";

function Stage({ c }: { c: StepConfig["stage"] }) {
  const { t, n } = useT();
  const cell = "min-w-0 bg-surface px-3.5 py-2 text-[12px]";
  return (
    <ol className="grid grid-cols-1 gap-px border-t border-line-soft bg-line-soft min-[760px]:grid-cols-3">
      <li className={cell}>
        <p className="font-medium text-ink">
          {t("config.stage.source")} <span className="font-mono font-normal text-muted">{c.sourceDriver}</span>
        </p>
        <p className="mt-0.5 text-muted">
          {c.sourceCount > 0
            ? [n("config.stage.sources", c.sourceCount), c.workers !== null && n("config.stage.workers", c.workers)].filter(Boolean).join(" · ")
            : t("config.stage.one_source")}
        </p>
      </li>
      <li className={cell}>
        <p className="font-medium text-ink">
          {t("config.stage.transform")} <span className="font-mono font-normal text-muted">{n("config.stage.stages", c.transformCount)}</span>
        </p>
        <p className="mt-0.5 truncate text-muted" title={c.transformNames}>
          {c.transformCount === 0 ? t("config.stage.as_read") : c.transformNames}
        </p>
      </li>
      <li className={cell}>
        <p className="font-medium text-ink">
          {t("config.stage.target")} <span className="font-mono font-normal text-muted">{c.targetDriver}</span>
        </p>
        <p className="mt-0.5 text-muted">{c.dedup !== "" ? `dedup ${c.dedup}` : t("config.stage.appends")}</p>
      </li>
    </ol>
  );
}

function Cell({ r }: { r: CfgRow }) {
  const { t } = useT();
  return (
    <td className="max-w-[28rem] truncate px-3 font-mono text-[12px] text-ink" title={r.note ? `${r.value}\n${r.note}` : r.value}>
      {r.value}
      {r.default && <span className="text-faint"> ({t("config.default")})</span>}
    </td>
  );
}

/** A step's configuration in tabs: Summary and Destination always, the others when they have something. */
export function StepConfigPanel({ c }: { c: StepConfig }) {
  const { t } = useT();
  const tabs: HashTab[] = [
    {
      key: "summary",
      label: t("config.tab.summary"),
      panel: (
        <>
          <h3 className={h3}>{t("config.tab.summary")}</h3>
          <p className="px-3.5 py-3 text-[12.5px] text-ink">{c.summary}</p>
          <Stage c={c.stage} />
          {c.flow && (
            <div className="border-t border-line-soft">
              <FlowDiagram flow={c.flow} />
            </div>
          )}
          {c.note !== "" && (
            <div className="border-t border-line-soft px-3.5 py-2.5">
              <ConfigNote text={c.note} />
            </div>
          )}
          {c.other.map((s) => (
            <div key={s.title} className="border-t border-line-soft">
              <h4 className="px-3.5 pt-2.5 text-[12px] font-semibold text-ink">{s.title}</h4>
              <CfgRows rows={s.rows} />
            </div>
          ))}
        </>
      ),
    },
  ];
  if (c.sourceHead.length > 0 || c.sourceCount > 0) {
    tabs.push({
      key: "sources",
      label: t("config.tab.sources"),
      count: c.sourceCount,
      panel: (
        <>
          <h3 className={h3}>{t("config.tab.sources")}</h3>
          <CfgRows rows={c.sourceHead} />
          {c.sourceCount > 0 && (
            <>
              <div className="flex flex-wrap items-center gap-1.5 border-t border-line-soft px-3.5 py-2.5 text-[12px]">
                <span className="font-medium text-ink">{t("config.common", { n: c.sourceCount })}</span>
                {c.common.length === 0 && <span className="text-faint">{t("config.common.none")}</span>}
                {c.common.map((r) => (
                  <span key={r.label} title={r.note} className="rounded-[4px] border border-line-strong px-1.5 font-mono text-[11.5px] text-ink-2">
                    <span className="text-faint">{r.label}</span> {r.value}
                    {r.default && <span className="text-faint"> ({t("config.default")})</span>}
                  </span>
                ))}
              </div>
              {c.varying.length > 0 && (
                <div className="overflow-x-auto border-t border-line-soft">
                  <table className="ct w-full text-[12.5px]">
                    <thead>
                      <tr className="text-left">
                        <th className={`${th} w-10 pl-3.5`}>#</th>
                        {c.varying.map((k) => (
                          <th key={k} className={th}>
                            {k}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-line-soft">
                      {c.sources.map((s) => (
                        <tr key={s.index}>
                          <td className="px-3 pl-3.5 font-mono text-faint tabular-nums">{s.index}</td>
                          {s.cells.map((cell, i) =>
                            cell ? (
                              <Cell key={i} r={cell} />
                            ) : (
                              <td key={i} className="px-3 text-faint">
                                —
                              </td>
                            ),
                          )}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </>
      ),
    });
  }
  if (c.transform.length > 0) {
    tabs.push({
      key: "transform",
      label: t("config.tab.transform"),
      count: c.transform.length,
      panel: (
        <>
          <h3 className={h3}>{t("config.tab.transform")}</h3>
          <ol className="divide-y divide-line-soft">
            {c.transform.map((op) => (
              <li key={op.index} className="flex gap-3 px-3.5 py-2 text-[12.5px]">
                <span className="w-5 shrink-0 font-mono text-faint tabular-nums">{op.index}</span>
                <div className="min-w-0 flex-1">
                  {op.rows.map((r) => (
                    <div key={r.path} data-path={r.path} className="flex flex-wrap items-baseline gap-x-2">
                      <span className="font-mono text-[12px] text-muted">{r.label.replace(`${op.index}.`, "")}</span>
                      <span className="font-mono text-[12px] text-ink">{r.list.length > 0 ? r.list.join(" · ") : r.value}</span>
                      {r.note !== "" && <ConfigNote text={r.note} />}
                    </div>
                  ))}
                </div>
              </li>
            ))}
          </ol>
        </>
      ),
    });
  }
  tabs.push({
    key: "destination",
    label: t("config.tab.destination"),
    panel: (
      <>
        <h3 className={h3}>{t("config.tab.destination")}</h3>
        <div className="px-3.5 py-3">
          <DefinitionList
            rows={[
              { term: t("config.identity.kind"), value: c.identity.legacy ? t("config.identity.legacy") : c.identity.kind || "—", mono: true },
              { term: t("config.identity.where"), value: c.identity.where || "—", mono: true },
              { term: t("config.identity.object"), value: c.identity.object, mono: true },
            ]}
          />
        </div>
        <CfgRows rows={c.destination} />
      </>
    ),
  });
  if (c.hasSchema) {
    tabs.push({
      key: "schema",
      label: t("config.tab.schema"),
      count: c.schema.length,
      panel: (
        <>
          <h3 className={h3}>{t("config.tab.schema")}</h3>
          {c.schema.length > 0 && (
            <div className="overflow-x-auto">
              <table className="ct w-full text-[12.5px]">
                <thead>
                  <tr className="text-left">
                    <th className={`${th} pl-3.5`}>{t("config.schema.column")}</th>
                    <th className={th}>{t("config.schema.type")}</th>
                    <th className={`${th} pr-3.5`}>{t("config.schema.role")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line-soft">
                  {c.schema.map((col) => (
                    <tr key={col.name}>
                      <td className="px-3 pl-3.5 font-mono text-ink">{col.name}</td>
                      <td className="px-3 font-mono text-[12px] text-muted">{col.type}</td>
                      <td className="px-3 pr-3.5 text-muted">{col.role}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <CfgRows rows={c.schemaExtra} />
        </>
      ),
    });
  }
  return <HashTabs id="load-config" label={t("config.title")} tabs={tabs} barClass="px-2 pt-1" />;
}
