import { useSearchParams } from "react-router";

/**
 * Filters live in the URL, under the old page's parameter names, so a filtered
 * screen is a link somebody can send. `fields` fixes the order they are
 * written in: one filter has exactly one URL.
 */
export function useFilter(fields: readonly string[], path: string) {
  const [params] = useSearchParams();
  const get = (f: string) => params.get(f) ?? "";
  const hrefWith = (change: Record<string, string>, to = path, extra: Record<string, string> = {}) => {
    const q = new URLSearchParams();
    for (const f of fields) {
      const v = f in change ? change[f] : get(f);
      if (v) q.set(f, v);
    }
    for (const [k, v] of Object.entries(extra)) q.set(k, v);
    const s = q.toString();
    return s ? `${to}?${s}` : to;
  };
  return { get, hrefWith, any: fields.some((f) => get(f) !== "") };
}
