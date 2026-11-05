import { Link, type To } from "react-router";
import { useFormat, useT } from "../i18n";
import { Skeleton } from "./skeleton";

/** The page numbers around the current one, five of them (the server's Window). */
function window5(page: number, pages: number): number[] {
  let from = page - 2;
  let to = page + 2;
  if (from < 1) {
    to += 1 - from;
    from = 1;
  }
  if (to > pages) {
    from -= to - pages;
    to = pages;
  }
  from = Math.max(1, from);
  return Array.from({ length: to - from + 1 }, (_, i) => from + i);
}

type Props = { page: number; perPage: number; total: number; pages: number; to: (page: number) => To };

const box = "rounded-card border border-line px-3 py-1 transition-colors hover:border-line-strong hover:text-ink";
const off = "rounded-card border border-line-soft px-3 py-1 text-line";

/** The list's footer (web/pages pagination): "1-25 of 60" and previous / numbers / next as router links. */
export function Pagination({ page, perPage, total, pages, to }: Props) {
  const { t } = useT();
  const f = useFormat();
  if (total === 0) return null;
  const first = (page - 1) * perPage + 1;
  const last = Math.min(page * perPage, total);
  return (
    <div className="flex flex-wrap items-center justify-between gap-4 border-t border-line px-6 py-4 text-xs text-muted">
      <span>{t("table.range", { first: f.number(first), last: f.number(last), total: f.number(total) })}</span>
      {pages > 1 && (
        <nav className="flex items-center gap-1">
          {page > 1 ? <Link to={to(page - 1)} rel="prev" className={box}>{t("table.previous")}</Link> : <span className={off}>{t("table.previous")}</span>}
          {window5(page, pages).map((n) =>
            n === page ? (
              <span key={n} aria-current="page" className="rounded-card border border-ink bg-ink px-3 py-1 font-medium text-surface">{n}</span>
            ) : (
              <Link key={n} to={to(n)} className={box}>{n}</Link>
            ),
          )}
          {page < pages ? <Link to={to(page + 1)} rel="next" className={box}>{t("table.next")}</Link> : <span className={off}>{t("table.next")}</span>}
        </nav>
      )}
    </div>
  );
}

/** The footer's height, held while the count is on its way. */
export function PaginationSkeleton() {
  return (
    <div className="flex items-center justify-between gap-4 border-t border-line px-6 py-4" aria-hidden="true">
      <Skeleton w="7rem" h="12px" />
    </div>
  );
}
