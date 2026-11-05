import { Fragment } from "react";
import type { CfgRow } from "../api/ingest";
import { useMsg, useT, type Cat, type Msg } from "../i18n";

/** A comment from the config file, in its author's line breaks: one line whole, a longer one folded to its first line. */
export function ConfigNote({ text }: { text: string }) {
  const [head = "", ...rest] = text.split("\n");
  const more = rest.join("\n").trim();
  if (more === "") return <p className="text-xs whitespace-pre-wrap text-muted">{text}</p>;
  return (
    <details className="text-xs text-muted">
      <summary className="cursor-pointer hover:text-ink">{head}</summary>
      <p className="mt-1 whitespace-pre-wrap">{more}</p>
    </details>
  );
}

/** A list of settings: label, value, default flag and note, with the block comments a row begins. `wide` is the target card's looser grid. */
export function CfgRows({ rows, wide }: { rows: CfgRow[]; wide?: boolean }) {
  const { t } = useT();
  if (rows.length === 0) return null;
  return (
    <div className={wide ? "mt-2" : "px-3.5 py-2"}>
      {rows.map((r) => (
        <Fragment key={r.path}>
          {r.before.map((b) => (
            <div key={b.label} className={wide ? "mt-3 mb-1" : "mt-2 mb-1"}>
              <span className={`font-mono text-ink ${wide ? "text-xs" : "text-[12px]"}`}>{b.label}</span>
              <ConfigNote text={b.text} />
            </div>
          ))}
          <div
            className={`grid grid-cols-1 py-1 ${wide ? "gap-x-6 sm:grid-cols-[16rem_1fr]" : "gap-x-4 sm:grid-cols-[14rem_1fr]"}`}
            data-path={r.path}
          >
            <div className={`font-mono break-all text-muted ${wide ? "text-xs" : "text-[12px]"}`}>{r.label}</div>
            <div className={wide ? "text-xs" : "text-[12px]"}>
              {r.value === "" ? <span className="text-muted">{t("config.empty")}</span> : <span className="font-mono break-all text-ink">{r.value}</span>}{" "}
              {r.default && (
                <span className="ml-2 rounded-card border border-line px-1 py-px text-[0.65rem] text-muted" title={t("config.default.tip")}>
                  {t("config.default")}
                </span>
              )}
              {r.note !== "" && (
                <div className="mt-1">
                  <ConfigNote text={r.note} />
                </div>
              )}
            </div>
          </div>
        </Fragment>
      ))}
    </div>
  );
}

/** A block's heading: the console's words where it has them, else its key made a heading ("dead_letter" -> "Dead letter"). */
export function useSectionTitle() {
  const msg = useMsg();
  return (s: { key: string; title: Msg | null }) => {
    if (s.title) return msg(s.title);
    const words = s.key.replaceAll("_", " ");
    return words.charAt(0).toUpperCase() + words.slice(1);
  };
}

type DeadLetter = { where: Cat | null; commands: string; readable: boolean; write: string };

/** Where a stream's refused batches go and how to get them back; the console runs nothing. */
export function DeadLetterPanel({ dl }: { dl: DeadLetter }) {
  const { t } = useT();
  const msg = useMsg();
  return (
    <div className="px-4 py-3 sm:px-6">
      <h4 className="text-xs font-semibold text-ink">{t("deadletter.commands.title")}</h4>
      <p className="mt-1 text-xs text-muted">{t("deadletter.goes", { where: msg(dl.where) })}</p>
      {dl.readable ? (
        <>
          <p className="mt-2 text-xs text-muted">{t("deadletter.commands.where")}</p>
          <pre className="mt-2 overflow-auto rounded-card border border-line-soft bg-canvas px-3 py-2 font-mono text-xs text-ink">{dl.commands}</pre>
          <p className="mt-2 text-xs text-muted">{t(dl.write === "append" ? "deadletter.resend.append" : dl.write === "merge" ? "deadletter.resend.merge" : "deadletter.resend.other")}</p>
        </>
      ) : (
        <p className="mt-2 text-xs text-muted">{t("deadletter.unreadable")}</p>
      )}
    </div>
  );
}
