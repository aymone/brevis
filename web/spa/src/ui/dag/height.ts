// The graph's height depends on the graph (heightFor in dag.tsx), which arrives
// after the page around it. A box that opens at one height and settles at
// another pushes everything below it, so the height each graph last settled at
// is remembered (in memory, and in sessionStorage across a reload) and the box
// and its lazy-chunk fallback open at that height. The first sight of a graph
// has nothing to remember and opens at the default: the floor heightFor can
// return (200), because a wide, short drawing is what the executor's levels
// make of most workflows (measured: every example workflow settles there).
export const DEFAULT_DAG_HEIGHT = 200;
const memo = new Map<string, number>();
const key = (url: string) => `brevis_dag_h:${url}`;

export function dagHeight(url: string): number {
  const known = memo.get(url);
  if (known) return known;
  try {
    const stored = Number(sessionStorage.getItem(key(url)));
    if (stored > 0) {
      memo.set(url, stored);
      return stored;
    }
  } catch {
    /* storage can be blocked: the default is only a first guess */
  }
  return DEFAULT_DAG_HEIGHT;
}

export function rememberDagHeight(url: string, height: number) {
  if (memo.get(url) === height) return;
  memo.set(url, height);
  try {
    sessionStorage.setItem(key(url), String(height));
  } catch {
    /* kept in memory only */
  }
}
