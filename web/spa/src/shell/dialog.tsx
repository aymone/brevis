import { useEffect, useRef, type ReactNode } from "react";
import { Icon } from "../ui/icon";
import { useT } from "../i18n";
import { openDialog, useOpenDialog, type DialogName } from "./dialogs";

// A native <dialog> driven by the shell's one-at-a-time state: focus
// containment, Escape and the backdrop come with the element. Same markup as
// components.Modal, so the compiled sheet's `dialog.modal` styles apply.
export function Modal({ name, title, foot, children }: { name: DialogName; title: string; foot?: ReactNode; children: ReactNode }) {
  const { t } = useT();
  const ref = useRef<HTMLDialogElement>(null);
  const open = useOpenDialog() === name;

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  return (
    <dialog ref={ref} id={name} className="modal" aria-labelledby={`${name}-title`} onClose={() => open && openDialog(null)}>
      <header className="dialog-head">
        <h2 id={`${name}-title`} className="text-[0.95rem] font-semibold text-ink">
          {title}
        </h2>
        <form method="dialog">
          <button
            type="submit"
            aria-label={t("dialog.close")}
            className="flex h-8 w-8 items-center justify-center rounded-card text-muted transition-colors hover:bg-surface-2 hover:text-ink"
          >
            <Icon name="x" className="h-4 w-4" />
          </button>
        </form>
      </header>
      <div className="dialog-body">{children}</div>
      {foot && <footer className="dialog-foot">{foot}</footer>}
    </dialog>
  );
}
