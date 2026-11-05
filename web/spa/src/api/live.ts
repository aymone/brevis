import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useFallbackInterval, useLive } from "../live/hooks";
import { getJSON } from "./client";

type Options = {
  /**
   * The server topic that says this data changed ("catalog", "ingest"). Without
   * one the screen has no server signal and keeps a slow poll (30 s, paused in a
   * hidden tab), as it always did.
   */
  topic?: string | string[];
  enabled?: boolean;
  /**
   * Keep the old answer on screen while a new key loads. Right for a list
   * whose filter changed (no skeleton flash); wrong for another record's page,
   * which must not show the previous record under the new address.
   */
  keep?: boolean;
};

// useData reads a JSON endpoint for a screen that follows its data. `isPending`
// is true only before the first answer for a key (or always, without `keep`).
export function useData<T>(key: readonly unknown[], path: string, { topic, enabled = true, keep = true }: Options = {}) {
  const topics = topic === undefined ? [] : Array.isArray(topic) ? topic : [topic];
  useLive(enabled ? topics : [], [key]);
  const fallback = useFallbackInterval();
  return useQuery({
    queryKey: key,
    queryFn: () => getJSON<T>(path),
    placeholderData: keep ? keepPreviousData : undefined,
    refetchInterval: topics.length ? fallback : 30_000,
    enabled,
  });
}
