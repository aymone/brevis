import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { getJSON } from "../api/client";
import { useConfig } from "../api/session";
import { useFormat, useT } from "../i18n";
import { Icon } from "../ui/icon";
import { Modal } from "./dialog";
import { openDialog } from "./dialogs";
import { toast } from "./toast";

export type Release = { tag: string; url: string; notes: string; published: string; newer: boolean };

/** "v0.40.0" however the build stamped it ("0.40.0", "v0.40.0"); "" when unknown. */
export const versionLabel = (v: string) => (!v ? "" : /^\d/.test(v) ? `v${v}` : v.trim());

/** The newest release, or undefined where none is known (a dev build, or no way to reach GitHub). */
export function useRelease(): Release | undefined {
  const { version } = useConfig();
  return useQuery({
    queryKey: ["release"],
    enabled: !!version,
    retry: false,
    staleTime: 5 * 60_000,
    queryFn: async () => {
      return (await getJSON<Release | undefined>("/api/release")) ?? null;
    },
  }).data ?? undefined;
}

// A new Brevis is told once per version: localStorage remembers the one shown.
export function UpdateNotice() {
  const { t } = useT();
  const r = useRelease();
  useEffect(() => {
    if (!r?.newer) return;
    let seen = "";
    try {
      seen = localStorage.getItem("brevis_update_seen") ?? "";
    } catch {
      /* a private window */
    }
    if (seen === r.tag) return;
    try {
      localStorage.setItem("brevis_update_seen", r.tag);
    } catch {
      /* the toast shows again next time */
    }
    toast(t("update.toast", { version: r.tag }), { kind: "run", action: t("update.view"), onAction: () => openDialog("version") });
  }, [r?.newer, r?.tag]); // eslint-disable-line react-hooks/exhaustive-deps
  return null;
}

// The newest release as a small product page. Drawn only when the release is known.
export function VersionModal() {
  const { t } = useT();
  const { date } = useFormat();
  const { version } = useConfig();
  const r = useRelease();
  if (!r) return null;
  const published = r.published && !r.published.startsWith("0001");
  return (
    <Modal
      name="version"
      title={t("update.notes")}
      foot={
        <a
          href={r.url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex shrink-0 items-center justify-center gap-1.5 rounded-card font-medium whitespace-nowrap transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-strong h-8 px-3 text-[0.82rem] bg-accent-strong text-on-accent hover:bg-accent-strong/90"
        >
          <Icon name="download" className="h-4 w-4" />
          {t("update.download")}
        </a>
      }
    >
      <div className="flex items-center gap-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2.5">
            <span className="font-mono text-[1.4rem] leading-none font-semibold tracking-tight text-ink">{r.tag}</span>
            {r.newer ? (
              <span className="rounded-full border border-state-running/40 bg-state-running/10 px-2 py-0.5 text-[0.68rem] font-semibold tracking-wide text-state-running uppercase">
                {t("update.new")}
              </span>
            ) : (
              <span className="rounded-full border border-state-success/40 bg-state-success/10 px-2 py-0.5 text-[0.68rem] font-semibold tracking-wide text-state-success uppercase">
                {t("update.latest")}
              </span>
            )}
          </div>
          <p className="mt-2 text-[0.78rem] leading-none text-muted">
            {published && t("update.released", { date: date(r.published) })}
            {r.newer && (
              <>
                {published && <span className="px-1.5 text-line-strong">·</span>}
                {t("update.running", { version: versionLabel(version) })}
              </>
            )}
          </p>
        </div>
      </div>
      <div className="mt-6 flex items-center gap-3">
        <span className="text-[0.68rem] font-semibold tracking-[0.14em] text-faint uppercase">{t("update.whats_new")}</span>
        <span className="h-px flex-1 bg-line-soft" />
      </div>
      {/* Notes arrive as HTML the server already sanitized (updates.Sanitize). */}
      <div className="release-notes mt-4" dangerouslySetInnerHTML={{ __html: r.notes }} />
    </Modal>
  );
}
