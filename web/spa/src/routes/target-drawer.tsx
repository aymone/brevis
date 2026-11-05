import type { UseQueryResult } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { Link, useNavigate } from "react-router";
import { useMsg, useT, type Cat, type Msg } from "../i18n";
import { CopyButton } from "../ui/copy";
import { Icon } from "../ui/icon";
import { ButtonLink } from "../ui/kit";
import { Logo, useLogoTitle } from "../ui/logo";
import { Skeleton } from "../ui/skeleton";
import { LoadBars } from "../ui/sparkline";
import { Freshness, FreshnessMark } from "../ui/status";
import type { CatalogRow } from "./catalog";
import { Configurations, type WriterConfig } from "./target-config";
import { ErrorText } from "../api/errors";

type Writer = {
  name: string;
  gateway: { name: string; stream: string } | null;
  workflow: string;
  node: string;
  status: string;
  cron: Msg | null;
  runInFlight: string | null;
  reason: Cat | null;
  href: string;
};
export type TargetDetail = {
  row: CatalogRow;
  writers: Writer[];
  bars: { href: string; rows: number | null; title: string }[];
  missed: number;
  readBy: { api: string; name: string; path: string }[];
  configs: WriterConfig[];
  sql: boolean;
};

const h3 = "mb-2 text-[12.5px] font-semibold text-ink";

/** The writers, the loads, what reads it, how each writer is configured: what comes from the data, under headings that do not. */
function Body({ d, u }: { d: TargetDetail | undefined; u: string }) {
  const { t } = useT();
  const msg = useMsg();
  return (
    <div className="dialog-body space-y-4">
      {d
        ? d.writers.map((w) => (
            <section key={w.name} className="rounded-card border border-line bg-surface">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-line-soft px-3.5 py-2.5">
                <FreshnessMark status={w.status} />
                <Link to={w.href} className="font-mono text-[12.5px] font-medium text-ink hover:underline">
                  {w.name}
                </Link>
                {w.cron && <span className="text-[12px] text-muted">{msg(w.cron)}</span>}
                {w.runInFlight && (
                  <Link to={`/runs/${w.runInFlight}`} className="ml-auto rounded-card border border-line px-2 py-0.5 text-[12px] text-ink hover:bg-hover">
                    {t("catalog.drawer.queued")} &rarr;
                  </Link>
                )}
              </div>
              {w.reason && <p className="px-3.5 py-2.5 text-[12.5px] text-ink-2">{msg(w.reason)}</p>}
            </section>
          ))
        : [0, 1].map((i) => <Skeleton key={i} h="4.5rem" className="rounded-card" />)}
      <section>
        <h3 className={h3}>{t("catalog.drawer.loads")}</h3>
        {d ? <LoadBars bars={d.bars} missed={d.missed} /> : <Skeleton w="14rem" h="2.75rem" />}
        <p className="mt-1.5 text-[11.5px] text-faint">{t("catalog.drawer.legend")}</p>
      </section>
      {d && d.readBy.length > 0 && (
        <section>
          <h3 className={h3}>{t("catalog.drawer.read_by")}</h3>
          <ul className="space-y-1 text-[12.5px]">
            {d.readBy.map((e) => (
              <li key={e.api + e.name}>
                <Link to={`/data/endpoints/${encodeURIComponent(e.api)}/${encodeURIComponent(e.name)}`} className="font-mono text-ink hover:underline">
                  GET {e.path}
                </Link>{" "}
                <span className="ml-1.5 text-muted">
                  {e.api} · {t("catalog.drawer.cache")}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {d ? <Configurations configs={d.configs} /> : <Skeleton h="9rem" className="rounded-card" />}
      <p className="text-[11.5px] text-faint" data-u={u}>
        {t("catalog.drawer.credentials")}
      </p>
    </div>
  );
}

/**
 * One destination, over the catalog. A native <dialog> opened modally: it
 * traps focus, closes on Escape and gives focus back to the row that opened it.
 */
export default function TargetDrawer({ u, query, closeTo }: { u: string; query: UseQueryResult<TargetDetail>; closeTo: string }) {
  const { t } = useT();
  const logoTitle = useLogoTitle();
  const navigate = useNavigate();
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (el && !el.open) el.showModal();
    return () => el?.close();
  }, []);

  const d = query.data;
  return (
    <dialog
      ref={ref}
      className="drawer"
      aria-labelledby="target-title"
      onCancel={(e) => {
        e.preventDefault();
        navigate(closeTo);
      }}
      onClick={(e) => {
        if (e.target === ref.current) navigate(closeTo);
      }}
    >
      <header className="dialog-head">
        <div className="flex min-w-0 items-start gap-3">
          {d ? <Logo k={d.row.kind || "file"} size={40} /> : <Skeleton w="40px" h="40px" className="shrink-0 rounded-card" />}
          <div className="min-w-0">
            <p className="text-[11px] font-medium tracking-[0.08em] text-muted uppercase">
              {d ? t("catalog.drawer.kicker", { kind: logoTitle(d.row.kind || "file") }) : <Skeleton w="8rem" h="16.5px" />}
            </p>
            <h2 id="target-title" className="font-mono text-[14px] font-semibold break-all text-ink">
              {u}
            </h2>
            <p className="mt-1 flex flex-wrap items-baseline gap-x-2 text-[12px] text-muted">
              {d ? (
                <>
                  <Freshness status={d.row.status} />
                  <span>{t("catalog.drawer.best")}</span>
                </>
              ) : (
                <Skeleton w="6rem" h="18px" />
              )}
            </p>
          </div>
        </div>
        <Link
          to={closeTo}
          aria-label={t("dialog.close")}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink"
        >
          <Icon name="x" className="h-4 w-4" />
        </Link>
      </header>
      {query.isError ? (
        <div className="dialog-body">
          <p role="alert" className="text-sm text-muted">
            <ErrorText error={query.error} />
          </p>
        </div>
      ) : (
        <Body d={d} u={u} />
      )}
      <footer className="dialog-foot">
        {d?.sql && (
          <ButtonLink to={`/sql?target=${encodeURIComponent(u)}`} icon="terminal">
            {t("catalog.drawer.query")}
          </ButtonLink>
        )}
        <CopyButton text={u} label={t("catalog.drawer.copy")} primary />
      </footer>
    </dialog>
  );
}

