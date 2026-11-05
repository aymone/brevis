import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import { isUp, subscribe, watchUp, type Wake } from "./connection";

// On `ready`, data fetched in the last few seconds is as fresh as a refetch would
// make it: the screen fetched on mount and the stream connected right after.
// Refetching it anyway doubled every request of every page load (a run's detail,
// the nav badges). Older data is refetched: it may have missed a change.
const READY_FRESH_MS = 3_000;

/**
 * Refetch `invalidate` when the server says one of `topics` changed, and once
 * on every (re)connect, so a change between the screen's fetch and the
 * connection is never missed. Subscribes only while mounted; no topics, no
 * subscription (a switched-off Live passes []).
 */
export function useLive(topics: string[], invalidate: QueryKey[]) {
  const qc = useQueryClient();
  const tk = topics.join("\n");
  const ik = JSON.stringify(invalidate);
  useEffect(() => {
    // cancelRefetch:false joins a fetch already in flight instead of aborting it
    // (an aborted read shows as a red "(canceled)" in the network panel).
    const run = (wake: Wake) =>
      invalidate.forEach(
        (queryKey) =>
          void qc.invalidateQueries(
            {
              queryKey,
              predicate: wake === "ready" ? (q) => Date.now() - q.state.dataUpdatedAt > READY_FRESH_MS : undefined,
            },
            { cancelRefetch: false },
          ),
      );
    const offs = topics.map((t) => subscribe(t, run));
    return () => offs.forEach((off) => off());
    // tk/ik are the value-stable stand-ins for the two arrays
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qc, tk, ik]);
}

/**
 * For `refetchInterval`: false while the event stream is up (it tells us when
 * to refetch), `ms` while it is not, so a screen still renews itself if the
 * server or the browser cannot stream. React Query pauses it in a hidden tab.
 */
export function useFallbackInterval(ms = 30_000): number | false {
  return useSyncExternalStore(watchUp, isUp) ? false : ms;
}
