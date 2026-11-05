import { lazy, Suspense, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { fieldErrors, triggerRun } from "../api/trigger";
import { useT } from "../i18n";
import { toast } from "../shell/toast";

// The drawer is its own chunk: most visits never open one.
const RunDrawer = lazy(() => import("./run-drawer"));

/** A rerun's starting point: the original run's values (and slot, which a rerun does not read). */
export type Rerun = { runId: string; params: Record<string, string>; slot: string | null };
type Target = { slug: string; rerun?: Rerun };

/**
 * Opens the Run drawer for a workflow: from a row, the workflow's page, or a run's Rerun.
 * `drawer` is rendered once by the screen; `open(target, opener)` gives focus back to `opener` on close.
 */
export function useRunDrawer(): { open: (t: Target, opener: HTMLElement) => void; drawer: ReactNode } {
  const [target, setTarget] = useState<Target | null>(null);
  const opener = useRef<HTMLElement | null>(null);
  const close = () => {
    setTarget(null);
    const el = opener.current;
    // After the dialog is gone: a native close() restores focus to a node that may have been re-rendered.
    requestAnimationFrame(() => el?.isConnected && el.focus());
  };
  return {
    open: (t, el) => {
      opener.current = el;
      setTarget(t);
    },
    drawer: target && (
      <Suspense fallback={null}>
        <RunDrawer slug={target.slug} rerun={target.rerun} onClose={close} />
      </Suspense>
    ),
  };
}

/** The refreshes a new run owes every list that shows runs. */
export function useRefreshRuns() {
  const qc = useQueryClient();
  return (slug: string) =>
    Promise.all(["workflows", "runs", "overview"].map((k) => qc.invalidateQueries({ queryKey: [k] })).concat(qc.invalidateQueries({ queryKey: ["workflow", slug] })));
}

/**
 * One click, no questions: the workflow has no parameters to ask for (a rerun of it) or the values are
 * already known (the Overview's Rerun). `stay` keeps the reader on the list, with a toast that follows the run.
 */
export function useQuickRun(stay = false) {
  const { t } = useT();
  const navigate = useNavigate();
  const refresh = useRefreshRuns();
  const [pending, setPending] = useState<string | null>(null);
  const run = async (slug: string, params: Record<string, string>, fromRun: string) => {
    setPending(slug);
    try {
      const r = await triggerRun(slug, { params, from_run: fromRun });
      void refresh(slug);
      const message = t("overview.rerun_created", { workflow: slug });
      if (stay) toast(message, { kind: "run", action: t("overview.follow"), onAction: () => navigate(r.href) });
      else {
        toast(message, { kind: "run" });
        navigate(r.href);
      }
    } catch (e) {
      // Without the drawer there is no field to put a refusal beside: it says which one.
      const [first] = Object.entries(fieldErrors(e));
      toast(first ? `${first[0]}: ${first[1]}` : (e as Error).message, { kind: "fail" });
    } finally {
      setPending(null);
    }
  };
  return { run, pending };
}
