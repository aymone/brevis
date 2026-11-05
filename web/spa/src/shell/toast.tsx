import { useSyncExternalStore } from "react";

// What just happened, in the corner, for 4.2s (ui.js's Brevis.toast). Any screen
// calls toast("Saved") or toast("Paused", { action: "Undo", onAction }); the
// <Toasts /> region in the shell draws them. role="status", so they are announced.
export type ToastOpts = { kind?: "ok" | "fail" | "run"; action?: string; onAction?: () => void };
type Item = ToastOpts & { id: number; message: string; leaving: boolean };

const TOAST_MS = 4200;
let items: Item[] = [];
let seq = 0;
const subs = new Set<() => void>();
const set = (next: Item[]) => {
  items = next;
  subs.forEach((s) => s());
};
const remove = (id: number) => set(items.filter((i) => i.id !== id));

export function toast(message: string, opts: ToastOpts = {}) {
  const id = ++seq;
  set([...items, { ...opts, id, message, leaving: false }]);
  setTimeout(() => {
    set(items.map((i) => (i.id === id ? { ...i, leaving: true } : i)));
    setTimeout(() => remove(id), 220);
  }, TOAST_MS);
}

const SHAPE = { fail: "st-diamond text-state-failed", run: "st-spin text-state-running", ok: "st-dot text-state-success" };

export function Toasts() {
  const list = useSyncExternalStore(
    (s) => (subs.add(s), () => void subs.delete(s)),
    () => items,
  );
  return (
    <div id="toasts" role="status" aria-live="polite" className="toasts">
      {list.map((t) => (
        <div key={t.id} className={`toast${t.leaving ? " toast-leaving" : ""}`}>
          <span className={`st st-mark ${SHAPE[t.kind ?? "ok"]}`} aria-hidden="true" />
          <span className="min-w-0 flex-1">{t.message}</span>
          {t.action && t.onAction && (
            <button
              type="button"
              className="toast-action"
              onClick={() => {
                t.onAction?.();
                remove(t.id);
              }}
            >
              {t.action}
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
