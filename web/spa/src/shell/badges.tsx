import { useQuery } from "@tanstack/react-query";
import { getJSON } from "../api/client";
import { useFallbackInterval, useLive } from "../live/hooks";
import { useFormat, useT } from "../i18n";

type Badges = { failing: number; running: number; attention: number };

// The sidebar's live numbers (JSON sibling of /nav/runs). Refetched when the
// server reports the `nav` topic; the slow fallback only runs while the event
// stream is down. Nothing is drawn until the first answer, and a failed read
// keeps the last numbers: this is decoration.
export const useBadges = () => {
  useLive(["nav"], [["nav"]]);
  return useQuery({
    queryKey: ["nav", "runs"],
    queryFn: () => getJSON<Badges>("/api/nav/runs"),
    refetchInterval: useFallbackInterval(),
    refetchOnWindowFocus: true,
    staleTime: 0,
  }).data;
};

export function NavBadge({ kind, children }: { kind: "fail" | "run"; children: string }) {
  return <span className={`nav-badge nav-badge-${kind}`}>{children}</span>;
}

/** What a section has to say before it is opened: Runs' failing and in-flight counts, Catalog's attention. */
export function SectionBadge({ section }: { section: string }) {
  const { n } = useT();
  const { number } = useFormat();
  const b = useBadges();
  if (!b) return null;
  if (section === "runs") {
    return (
      <span data-nav-runs data-sb-badge className="ml-auto inline-flex items-center gap-1">
        {b.failing > 0 && (
          <>
            <span className="sr-only">{n("nav.runs.failing", b.failing)}</span>
            <span aria-hidden="true">
              <NavBadge kind="fail">{number(b.failing)}</NavBadge>
            </span>
          </>
        )}
        {b.running > 0 && (
          <>
            <span className="sr-only">{n("nav.runs.running", b.running)}</span>
            <span aria-hidden="true">
              <NavBadge kind="run">{number(b.running)}</NavBadge>
            </span>
          </>
        )}
      </span>
    );
  }
  if (section === "catalog" && b.attention > 0) {
    return (
      <>
        <span className="sr-only">{n("nav.catalog.attention", b.attention)}</span>
        <span aria-hidden="true" data-sb-badge className="ml-auto inline-flex">
          <NavBadge kind="fail">{number(b.attention)}</NavBadge>
        </span>
      </>
    );
  }
  return null;
}
